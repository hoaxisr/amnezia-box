//go:build minimal

package include

import (
	"context"

	box "github.com/sagernet/sing-box"
	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/adapter/endpoint"
	"github.com/sagernet/sing-box/adapter/inbound"
	"github.com/sagernet/sing-box/adapter/outbound"
	"github.com/sagernet/sing-box/adapter/service"
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/dns"
	"github.com/sagernet/sing-box/dns/transport"
	"github.com/sagernet/sing-box/dns/transport/fakeip"
	"github.com/sagernet/sing-box/dns/transport/hosts"
	"github.com/sagernet/sing-box/dns/transport/local"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing-box/protocol/block"
	"github.com/sagernet/sing-box/protocol/direct"
	protocolDNS "github.com/sagernet/sing-box/protocol/dns"
	"github.com/sagernet/sing-box/protocol/group"
	"github.com/sagernet/sing-box/protocol/mixed"
	"github.com/sagernet/sing-box/protocol/tun"
	"github.com/sagernet/sing-box/protocol/vless"
	"github.com/sagernet/sing-box/service/resolved"
	E "github.com/sagernet/sing/common/exceptions"
)

// Context creates a minimal context for router deployment
// Includes only: tun, mixed (inbound), vless, direct, block, dns (outbound), awg (endpoint)
func Context(ctx context.Context) context.Context {
	return box.Context(ctx, InboundRegistry(), OutboundRegistry(), EndpointRegistry(), DNSTransportRegistry(), ServiceRegistry())
}

func InboundRegistry() *inbound.Registry {
	registry := inbound.NewRegistry()

	// Core inbounds for router
	tun.RegisterInbound(registry)
	mixed.RegisterInbound(registry)

	// Register stubs for excluded inbounds
	registerMinimalInboundStubs(registry)

	return registry
}

func OutboundRegistry() *outbound.Registry {
	registry := outbound.NewRegistry()

	// Essential outbounds
	direct.RegisterOutbound(registry)
	block.RegisterOutbound(registry)
	protocolDNS.RegisterOutbound(registry)

	// Groups for server selection
	group.RegisterSelector(registry)
	group.RegisterURLTest(registry)

	// VLESS - main proxy protocol
	vless.RegisterOutbound(registry)

	// Register stubs for excluded outbounds
	registerMinimalOutboundStubs(registry)

	return registry
}

func EndpointRegistry() *endpoint.Registry {
	registry := endpoint.NewRegistry()

	// AWG endpoint (via with_awg tag)
	registerAwgEndpoint(registry)

	// Register stubs for excluded endpoints
	registerMinimalEndpointStubs(registry)

	return registry
}

func DNSTransportRegistry() *dns.TransportRegistry {
	registry := dns.NewTransportRegistry()

	// Core DNS transports
	transport.RegisterTCP(registry)
	transport.RegisterUDP(registry)
	transport.RegisterTLS(registry)
	transport.RegisterHTTPS(registry)
	hosts.RegisterTransport(registry)
	local.RegisterTransport(registry)
	fakeip.RegisterTransport(registry)
	resolved.RegisterTransport(registry)

	return registry
}

func ServiceRegistry() *service.Registry {
	registry := service.NewRegistry()

	resolved.RegisterService(registry)

	return registry
}

// Stubs for excluded inbounds - provide clear error messages
func registerMinimalInboundStubs(registry *inbound.Registry) {
	inbound.Register[option.DirectInboundOptions](registry, C.TypeDirect, func(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, options option.DirectInboundOptions) (adapter.Inbound, error) {
		return nil, E.New("direct inbound is not included in minimal build")
	})
	inbound.Register[option.SocksInboundOptions](registry, C.TypeSOCKS, func(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, options option.SocksInboundOptions) (adapter.Inbound, error) {
		return nil, E.New("socks inbound is not included in minimal build, use mixed instead")
	})
	inbound.Register[option.HTTPMixedInboundOptions](registry, C.TypeHTTP, func(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, options option.HTTPMixedInboundOptions) (adapter.Inbound, error) {
		return nil, E.New("http inbound is not included in minimal build, use mixed instead")
	})
	inbound.Register[option.ShadowsocksInboundOptions](registry, C.TypeShadowsocks, func(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, options option.ShadowsocksInboundOptions) (adapter.Inbound, error) {
		return nil, E.New("shadowsocks is not included in minimal build")
	})
	inbound.Register[option.VMessInboundOptions](registry, C.TypeVMess, func(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, options option.VMessInboundOptions) (adapter.Inbound, error) {
		return nil, E.New("vmess is not included in minimal build")
	})
	inbound.Register[option.TrojanInboundOptions](registry, C.TypeTrojan, func(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, options option.TrojanInboundOptions) (adapter.Inbound, error) {
		return nil, E.New("trojan is not included in minimal build")
	})
	inbound.Register[option.VLESSInboundOptions](registry, C.TypeVLESS, func(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, options option.VLESSInboundOptions) (adapter.Inbound, error) {
		return nil, E.New("vless inbound is not included in minimal build (only outbound supported)")
	})
	inbound.Register[option.RedirectInboundOptions](registry, C.TypeRedirect, func(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, options option.RedirectInboundOptions) (adapter.Inbound, error) {
		return nil, E.New("redirect is not included in minimal build")
	})
	inbound.Register[option.TProxyInboundOptions](registry, C.TypeTProxy, func(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, options option.TProxyInboundOptions) (adapter.Inbound, error) {
		return nil, E.New("tproxy is not included in minimal build")
	})
	inbound.Register[option.ShadowsocksInboundOptions](registry, C.TypeShadowsocksR, func(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, options option.ShadowsocksInboundOptions) (adapter.Inbound, error) {
		return nil, E.New("ShadowsocksR is deprecated and removed")
	})
}

// Stubs for excluded outbounds
func registerMinimalOutboundStubs(registry *outbound.Registry) {
	outbound.Register[option.SOCKSOutboundOptions](registry, C.TypeSOCKS, func(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, options option.SOCKSOutboundOptions) (adapter.Outbound, error) {
		return nil, E.New("socks outbound is not included in minimal build")
	})
	outbound.Register[option.HTTPOutboundOptions](registry, C.TypeHTTP, func(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, options option.HTTPOutboundOptions) (adapter.Outbound, error) {
		return nil, E.New("http outbound is not included in minimal build")
	})
	outbound.Register[option.ShadowsocksOutboundOptions](registry, C.TypeShadowsocks, func(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, options option.ShadowsocksOutboundOptions) (adapter.Outbound, error) {
		return nil, E.New("shadowsocks is not included in minimal build")
	})
	outbound.Register[option.VMessOutboundOptions](registry, C.TypeVMess, func(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, options option.VMessOutboundOptions) (adapter.Outbound, error) {
		return nil, E.New("vmess is not included in minimal build")
	})
	outbound.Register[option.TrojanOutboundOptions](registry, C.TypeTrojan, func(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, options option.TrojanOutboundOptions) (adapter.Outbound, error) {
		return nil, E.New("trojan is not included in minimal build")
	})
	outbound.Register[option.SSHOutboundOptions](registry, C.TypeSSH, func(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, options option.SSHOutboundOptions) (adapter.Outbound, error) {
		return nil, E.New("ssh is not included in minimal build")
	})
	outbound.Register[option.TorOutboundOptions](registry, C.TypeTor, func(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, options option.TorOutboundOptions) (adapter.Outbound, error) {
		return nil, E.New("tor is not included in minimal build")
	})
	outbound.Register[option.ShadowsocksROutboundOptions](registry, C.TypeShadowsocksR, func(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, options option.ShadowsocksROutboundOptions) (adapter.Outbound, error) {
		return nil, E.New("ShadowsocksR is deprecated and removed")
	})
}

// Stubs for excluded endpoints
func registerMinimalEndpointStubs(registry *endpoint.Registry) {
	endpoint.Register[option.WireGuardEndpointOptions](registry, C.TypeWireGuard, func(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, options option.WireGuardEndpointOptions) (adapter.Endpoint, error) {
		return nil, E.New("WireGuard is not included in minimal build, use AWG instead")
	})
	endpoint.Register[option.TailscaleEndpointOptions](registry, C.TypeTailscale, func(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, options option.TailscaleEndpointOptions) (adapter.Endpoint, error) {
		return nil, E.New("Tailscale is not included in minimal build")
	})
}
