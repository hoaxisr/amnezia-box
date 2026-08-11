package awg

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"net"
	"net/netip"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/adapter/endpoint"
	"github.com/sagernet/sing-box/common/dialer"
	"github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing-box/transport/awg"
	"github.com/sagernet/sing-tun"
	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/bufio"
	E "github.com/sagernet/sing/common/exceptions"
	"github.com/sagernet/sing/common/format"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/service"

	"go4.org/netipx"
)

var (
	_ adapter.FlowOutbound                = (*Endpoint)(nil)
	_ adapter.OutboundWithPreferredRoutes = (*Endpoint)(nil)
	_ dialer.PacketDialerWithDestination  = (*Endpoint)(nil)
	_ tun.Handler                         = (*Endpoint)(nil)
)

func RegisterEndpoint(registry *endpoint.Registry) {
	endpoint.Register(registry, constant.TypeAwg, NewEndpoint)
}

type Endpoint struct {
	*awg.Device
	endpoint.Adapter
	ctx       context.Context
	address   []netip.Prefix
	router    adapter.Router
	logger    log.ContextLogger
	dnsRouter adapter.DNSRouter
}

func NewEndpoint(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, options option.AwgEndpointOptions) (adapter.Endpoint, error) {
	if options.MTU == 0 {
		options.MTU = 1408
	}

	options.UDPFragmentDefault = true
	// Check if any peer has a domain address
	remoteIsDomain := common.Any(options.Peers, func(peer option.AwgPeerOptions) bool {
		return !M.ParseAddr(peer.Address).IsValid()
	})
	dial, err := dialer.NewWithOptions(dialer.Options{
		Context:          ctx,
		Options:          options.DialerOptions,
		RemoteIsDomain:   remoteIsDomain,
		ResolverOnDetour: true,
		DirectOutbound:   true,
	})
	if err != nil {
		return nil, err
	}

	var allowedPrefixBuilder netipx.IPSetBuilder
	var excludedPrefixBuilder netipx.IPSetBuilder
	for _, peer := range options.Peers {
		for _, prefix := range peer.AllowedIPs {
			allowedPrefixBuilder.AddPrefix(prefix)
		}

		if addr, err := netip.ParseAddr(peer.Address); err == nil {
			excludedPrefixBuilder.Add(addr)
		}
	}
	allowedIps, err := allowedPrefixBuilder.IPSet()
	if err != nil {
		return nil, err
	}
	excludedIps, err := excludedPrefixBuilder.IPSet()
	if err != nil {
		return nil, err
	}

	// Create peer resolver function for domain endpoints
	// Always use system resolver for peer endpoints because:
	// 1. VPN server must be resolved before VPN tunnel is established
	// 2. dnsRouter may not be fully initialized at this stage
	var resolvePeer func(domain string) ([]netip.Addr, error)
	if remoteIsDomain {
		resolvePeer = func(domain string) ([]netip.Addr, error) {
			return net.DefaultResolver.LookupNetIP(ctx, "ip", domain)
		}
	}

	ipc, err := genIpcConfig(options, resolvePeer)
	if err != nil {
		return nil, err
	}

	// The IPC config above pins whatever the domain resolved to right now;
	// these resolvers re-run on every handshake initiation, so a server that
	// moved (DDNS) or answers with several addresses is still reached.
	var domainPeers []awg.DomainPeer
	for _, peer := range options.Peers {
		if peer.Address == "" || peer.Port == 0 || M.ParseAddr(peer.Address).IsValid() {
			continue
		}
		publicKeyBytes, decodeErr := base64.StdEncoding.DecodeString(peer.PublicKey)
		if decodeErr != nil {
			return nil, decodeErr
		}
		domain := peer.Address
		domainPeers = append(domainPeers, awg.DomainPeer{
			Domain:       domain,
			PublicKeyHex: hex.EncodeToString(publicKeyBytes),
			Port:         peer.Port,
			Resolve: func() ([]netip.Addr, error) {
				return resolvePeer(domain)
			},
		})
	}

	logger.Debug("AWG IPC config:\n", ipc)

	// The endpoint is created before the device so it can be passed as the
	// gVisor forwarder Handler: inbound connections from the tunnel to
	// arbitrary destinations are routed via Endpoint.NewConnectionEx /
	// NewPacketConnectionEx (gateway/exit role). Mirrors transport/wireguard.
	ep := &Endpoint{
		Adapter:   endpoint.NewAdapterWithDialerOptions("awg", tag, []string{N.NetworkTCP, N.NetworkUDP}, options.DialerOptions),
		ctx:       ctx,
		address:   options.Address,
		router:    router,
		logger:    logger,
		dnsRouter: service.FromContext[adapter.DNSRouter](ctx),
	}

	dev, err := awg.NewDevice(ctx, logger, dial, ipc, awg.DeviceOpts{
		UseIntegratedTun: options.UseIntegratedTun,
		Address:          options.Address,
		AllowedIps:       allowedIps.Prefixes(),
		ExcludedIps:      excludedIps.Prefixes(),
		DomainPeers:      domainPeers,
		MTU:              options.MTU,
		Handler:          ep,
		UDPTimeout:       constant.UDPTimeout,
	})
	if err != nil {
		return nil, err
	}

	ep.Device = dev
	return ep, nil
}

func genIpcConfig(opts option.AwgEndpointOptions, resolvePeer func(domain string) ([]netip.Addr, error)) (string, error) {
	if opts.PrivateKey == "" {
		return "", E.New("missing private key")
	}
	privateKeyBytes, err := base64.StdEncoding.DecodeString(opts.PrivateKey)
	if err != nil {
		return "", err
	}
	s := "private_key=" + hex.EncodeToString(privateKeyBytes)
	if opts.ListenPort != 0 {
		s += "\nlisten_port=" + format.ToString(opts.ListenPort)
	}
	if opts.Jc != 0 {
		s += "\njc=" + format.ToString(opts.Jc)
	}
	if opts.Jmin != 0 {
		s += "\njmin=" + format.ToString(opts.Jmin)
	}
	if opts.Jmax != 0 {
		s += "\njmax=" + format.ToString(opts.Jmax)
	}
	if opts.S1 != 0 {
		s += "\ns1=" + format.ToString(opts.S1)
	}
	if opts.S2 != 0 {
		s += "\ns2=" + format.ToString(opts.S2)
	}
	if opts.S3 != 0 {
		s += "\ns3=" + format.ToString(opts.S3)
	}
	if opts.S4 != 0 {
		s += "\ns4=" + format.ToString(opts.S4)
	}
	if opts.H1 != "" {
		s += "\nh1=" + opts.H1
	}
	if opts.H2 != "" {
		s += "\nh2=" + opts.H2
	}
	if opts.H3 != "" {
		s += "\nh3=" + opts.H3
	}
	if opts.H4 != "" {
		s += "\nh4=" + opts.H4
	}
	if opts.I1 != "" {
		s += "\ni1=" + opts.I1
	}
	if opts.I2 != "" {
		s += "\ni2=" + opts.I2
	}
	if opts.I3 != "" {
		s += "\ni3=" + opts.I3
	}
	if opts.I4 != "" {
		s += "\ni4=" + opts.I4
	}
	if opts.I5 != "" {
		s += "\ni5=" + opts.I5
	}

	if opts.HeaderProtectionKey != "" {
		for i, padding := range []int{opts.S1, opts.S2, opts.S3, opts.S4} {
			if padding < 12 {
				return "", E.New("s", i+1, " must be at least 12 when header_protection_key is set")
			}
		}
		headerProtectionKeyBytes, err := base64.StdEncoding.DecodeString(opts.HeaderProtectionKey)
		if err != nil {
			return "", err
		}
		s += "\nheader_protection_key=" + hex.EncodeToString(headerProtectionKeyBytes)
	}
	if opts.ContentPaddingAddition != "" {
		s += "\ncontent_padding_addition=" + opts.ContentPaddingAddition
	}
	if opts.RekeyAfterTime != "" {
		s += "\nrekey_after_time=" + opts.RekeyAfterTime
	}
	if opts.RekeyTimeout != "" {
		s += "\nrekey_timeout=" + opts.RekeyTimeout
	}
	if opts.RejectAfterTime != "" {
		s += "\nreject_after_time=" + opts.RejectAfterTime
	}
	if opts.KeepaliveTimeout != "" {
		s += "\nkeepalive_timeout=" + opts.KeepaliveTimeout
	}
	if opts.MaxHandshakeAttempts != "" {
		s += "\nmax_handshake_attempts=" + opts.MaxHandshakeAttempts
	}

	for _, peer := range opts.Peers {
		publicKeyBytes, err := base64.StdEncoding.DecodeString(peer.PublicKey)
		if err != nil {
			return "", err
		}
		s += "\npublic_key=" + hex.EncodeToString(publicKeyBytes)
		if peer.PresharedKey != "" {
			presharedKeyBytes, err := base64.StdEncoding.DecodeString(peer.PresharedKey)
			if err != nil {
				return "", err
			}
			s += "\npreshared_key=" + hex.EncodeToString(presharedKeyBytes)
		}
		if peer.Address != "" && peer.Port != 0 {
			// Resolve domain to IP if necessary
			endpointAddr := peer.Address
			if addr := M.ParseAddr(peer.Address); !addr.IsValid() {
				// It's a domain, resolve it
				if resolvePeer == nil {
					return "", E.New("peer address is a domain but no resolver provided: ", peer.Address)
				}
				resolvedAddrs, resolveErr := resolvePeer(peer.Address)
				if resolveErr != nil {
					return "", E.Cause(resolveErr, "resolve peer endpoint ", peer.Address)
				}
				endpointAddr = resolvedAddrs[0].String()
			}
			// net.JoinHostPort оборачивает IPv6-литерал в скобки ([::1]:port);
			// без этого endpoint=::1:51820 — невалидный UAPI-адрес для wireguard-go.
			s += "\nendpoint=" + net.JoinHostPort(endpointAddr, format.ToString(peer.Port))
		}
		if peer.PersistentKeepaliveInterval != "" && peer.PersistentKeepaliveInterval != "0" {
			s += "\npersistent_keepalive_interval=" + string(peer.PersistentKeepaliveInterval)
		}
		for _, allowedIp := range peer.AllowedIPs {
			s += "\nallowed_ip=" + allowedIp.String()
		}
	}
	return s, nil
}

func (e *Endpoint) NewPacketConnectionEx(ctx context.Context, conn N.PacketConn, source M.Socksaddr, destination M.Socksaddr, onClose N.CloseHandlerFunc) {
	var metadata adapter.InboundContext
	metadata.Inbound = e.Tag()
	metadata.InboundType = e.Type()
	metadata.Source = source
	metadata.Destination = destination
	for _, addr := range e.address {
		if addr.Contains(destination.Addr) {
			metadata.OriginDestination = destination
			if destination.Addr.Is4() {
				metadata.Destination.Addr = netip.AddrFrom4([4]uint8{127, 0, 0, 1})
			} else {
				metadata.Destination.Addr = netip.IPv6Loopback()
			}
			conn = bufio.NewNATPacketConn(bufio.NewNetPacketConn(conn), metadata.OriginDestination, metadata.Destination)
		}
	}
	e.logger.InfoContext(ctx, "inbound packet connection from ", source)
	e.logger.InfoContext(ctx, "inbound packet connection to ", destination)
	e.router.RoutePacketConnectionEx(ctx, conn, metadata, onClose)
}

func (e *Endpoint) DialContext(ctx context.Context, network string, destination M.Socksaddr) (net.Conn, error) {
	switch network {
	case N.NetworkTCP:
		e.logger.InfoContext(ctx, "outbound connection to ", destination)
	case N.NetworkUDP:
		e.logger.InfoContext(ctx, "outbound packet connection to ", destination)
	}
	if destination.IsFqdn() {
		destinationAddresses, err := e.dnsRouter.Lookup(ctx, destination.Fqdn, adapter.DNSQueryOptions{})
		if err != nil {
			return nil, err
		}
		return N.DialSerial(ctx, e.Device, network, destination, destinationAddresses)
	} else if !destination.Addr.IsValid() {
		return nil, E.New("invalid destination: ", destination)
	}
	return e.Device.DialContext(ctx, network, destination)
}

func (e *Endpoint) ListenPacket(ctx context.Context, destination M.Socksaddr) (net.PacketConn, error) {
	e.logger.InfoContext(ctx, "outbound packet connection to ", destination)
	if destination.IsFqdn() {
		destinationAddresses, err := e.dnsRouter.Lookup(ctx, destination.Fqdn, adapter.DNSQueryOptions{})
		if err != nil {
			return nil, err
		}
		packetConn, _, err := N.ListenSerial(ctx, e.Device, destination, destinationAddresses)
		if err != nil {
			return nil, err
		}
		return packetConn, nil
	}
	return e.Device.ListenPacket(ctx, destination)
}

func (w *Endpoint) NewConnectionEx(ctx context.Context, conn net.Conn, source M.Socksaddr, destination M.Socksaddr, onClose N.CloseHandlerFunc) {
	var metadata adapter.InboundContext
	metadata.Inbound = w.Tag()
	metadata.InboundType = w.Type()
	metadata.Source = source
	for _, addr := range w.address {
		if addr.Contains(destination.Addr) {
			metadata.OriginDestination = destination
			if destination.Addr.Is4() {
				destination.Addr = netip.AddrFrom4([4]uint8{127, 0, 0, 1})
			} else {
				destination.Addr = netip.IPv6Loopback()
			}
			break
		}
	}
	metadata.Destination = destination
	w.logger.InfoContext(ctx, "inbound connection from ", source)
	w.logger.InfoContext(ctx, "inbound connection to ", metadata.Destination)
	w.router.RouteConnectionEx(ctx, conn, metadata, onClose)
}

func (e *Endpoint) PreMatchFlow(network string, destination netip.Addr) adapter.PreMatchAction {
	return adapter.PreMatchFlow
}

func (e *Endpoint) JudgeFlow(network uint8, source netip.AddrPort, destination netip.AddrPort, firstPacket []byte) tun.FlowVerdict {
	for _, localPrefix := range e.address {
		if localPrefix.Contains(destination.Addr()) {
			return tun.FlowVerdict{Action: tun.ActionAccept}
		}
	}
	return adapter.JudgeFlow(e.router, e.Tag(), e.Type(), network, source, destination, firstPacket)
}

func (e *Endpoint) NewDNSPacket(payload []byte, source M.Socksaddr, destination M.Socksaddr, writer N.PacketWriter) {
	ctx := log.ContextWithNewID(e.ctx)
	var metadata adapter.InboundContext
	metadata.Inbound = e.Tag()
	metadata.InboundType = e.Type()
	metadata.Network = N.NetworkUDP
	metadata.Source = source
	metadata.Destination = destination
	metadata.Protocol = constant.ProtocolDNS
	e.logger.InfoContext(ctx, "inbound DNS packet from ", source)
	e.router.HijackDNSPacket(ctx, payload, writer, metadata)
}

func (e *Endpoint) PreferredDomain(metadata *adapter.InboundContext, domain string) bool {
	return false
}

func (e *Endpoint) PreferredAddress(metadata *adapter.InboundContext, address netip.Addr) bool {
	if !e.Device.Started() {
		return false
	}
	return e.Device.Lookup(address) != nil
}

func (e *Endpoint) ListenPacketWithDestination(ctx context.Context, destination M.Socksaddr) (net.PacketConn, netip.Addr, error) {
	e.logger.InfoContext(ctx, "outbound packet connection to ", destination)
	if destination.IsFqdn() {
		destinationAddresses, err := e.dnsRouter.Lookup(ctx, destination.Fqdn, adapter.DNSQueryOptions{})
		if err != nil {
			return nil, netip.Addr{}, err
		}
		return N.ListenSerial(ctx, e.Device, destination, destinationAddresses)
	}
	packetConn, err := e.Device.ListenPacket(ctx, destination)
	if err != nil {
		return nil, netip.Addr{}, err
	}
	if destination.IsIP() {
		return packetConn, destination.Addr, nil
	}
	return packetConn, netip.Addr{}, nil
}
