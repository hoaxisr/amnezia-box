package awg

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"strings"
	"sync/atomic"
	"time"

	"github.com/amnezia-vpn/amneziawg-go/v3/conn"
	"github.com/amnezia-vpn/amneziawg-go/v3/device"

	"github.com/sagernet/sing-box/adapter"
	tun "github.com/sagernet/sing-tun"
	"github.com/sagernet/sing/common/exceptions"
	E "github.com/sagernet/sing/common/exceptions"
	"github.com/sagernet/sing/common/logger"
	"github.com/sagernet/sing/common/metadata"
	"github.com/sagernet/sing/common/network"
)

// DomainPeer is a peer whose endpoint is configured as a domain: Resolve is
// invoked on every handshake initiation, and the initiation is sent to every
// address it returns, so a moved or multi-address server is followed without
// restarting the endpoint.
type DomainPeer struct {
	Domain       string
	PublicKeyHex string
	Port         uint16
	Resolve      func() ([]netip.Addr, error)
}

type DeviceOpts struct {
	UseIntegratedTun bool
	Address          []netip.Prefix
	AllowedIps       []netip.Prefix
	ExcludedIps      []netip.Prefix
	DomainPeers      []DomainPeer
	MTU              uint32
	// Handler receives inbound connections from the tunnel to arbitrary
	// destinations (gateway/exit role). Only honored by the gVisor
	// non-integrated tun; nil keeps client-only behavior.
	Handler    tun.Handler
	UDPTimeout time.Duration
}

type Device struct {
	awgDevice    *device.Device
	tun          tunAdapter
	returnDevice *returnDeviceWrapper
	bind         conn.Bind
	logger       *device.Logger
	ipcConfig    string
	address      []netip.Prefix
	domainPeers  []DomainPeer
	mtu          uint32
	started      atomic.Bool
	allowedIPs   *device.AllowedIPs
}

func NewDevice(ctx context.Context, logger logger.ContextLogger, dial network.Dialer, ipcConfig string, opts DeviceOpts) (*Device, error) {
	var (
		tun tunAdapter
		err error
	)

	if opts.UseIntegratedTun {
		tun, err = newSystemTun(ctx, opts.Address, opts.AllowedIps, opts.ExcludedIps, opts.MTU, logger)
		if err != nil {
			return nil, exceptions.Cause(err, "create tunnel")
		}
	} else {
		tun, err = newNonIntegratedTun(ctx, opts.Address, opts.MTU, opts.Handler, opts.UDPTimeout, logger)
		if err != nil {
			return nil, err
		}
	}

	awgLogger := &device.Logger{
		Verbosef: func(format string, args ...interface{}) {
			logger.Debug(fmt.Sprintf(strings.ToLower(format), args...))
		},
		Errorf: func(format string, args ...interface{}) {
			logger.Error(fmt.Sprintf(strings.ToLower(format), args...))
		},
	}

	return &Device{
		tun:          tun,
		returnDevice: newReturnDevice(tun),
		bind:         newBind(ctx, dial),
		logger:       awgLogger,
		ipcConfig:    ipcConfig,
		address:      opts.Address,
		domainPeers:  opts.DomainPeers,
		mtu:          opts.MTU,
	}, nil
}

func (d *Device) Start(stage adapter.StartStage) error {
	if stage != adapter.StartStateStart {
		return nil
	}

	// Start the underlying tun before creating the AWG device: device.NewDevice
	// immediately launches RoutineReadFromTUN, which reads from returnDevice
	// (the real tun). This mirrors transport/wireguard.Endpoint.Start, which
	// also starts the tun before device.NewDevice.
	if err := d.tun.Start(); err != nil {
		return E.Cause(err, "tun start")
	}

	d.awgDevice = device.NewDevice(d.returnDevice, d.bind, d.logger)
	d.allowedIPs = d.awgDevice.AllowedIPs()
	if err := d.awgDevice.IpcSet(d.ipcConfig); err != nil {
		return E.Cause(err, "set ipc config")
	}
	if err := d.attachPeerResolvers(); err != nil {
		return err
	}

	if err := d.awgDevice.Up(); err != nil {
		return err
	}
	d.started.Store(true)
	return nil
}

// attachPeerResolvers must run after IpcSet: the peers it looks up are created
// by the IPC config.
func (d *Device) attachPeerResolvers() error {
	for _, peer := range d.domainPeers {
		var publicKey device.NoisePublicKey
		if err := publicKey.FromHex(peer.PublicKeyHex); err != nil {
			return E.Cause(err, "parse public key of peer ", peer.Domain)
		}
		awgPeer := d.awgDevice.LookupPeer(publicKey)
		if awgPeer == nil {
			return E.New("missing configured peer ", peer.Domain)
		}
		resolve, port := peer.Resolve, peer.Port
		awgPeer.SetEndpointResolver(func() ([]conn.Endpoint, error) {
			addresses, err := resolve()
			if err != nil {
				return nil, err
			}
			endpoints := make([]conn.Endpoint, 0, len(addresses))
			for _, address := range addresses {
				endpoint, err := d.bind.ParseEndpoint(netip.AddrPortFrom(address, port).String())
				if err != nil {
					return nil, err
				}
				endpoints = append(endpoints, endpoint)
			}
			return endpoints, nil
		})
	}
	return nil
}

func (d *Device) Close() error {
	d.started.Store(false)
	// awgDevice.Close closes the underlying tun exactly once; do not close it
	// again here (double close panics on networkTun's channel).
	if d.awgDevice != nil {
		d.awgDevice.Close()
	}
	return nil
}

func (d *Device) Started() bool {
	return d.started.Load()
}

func (d *Device) Lookup(address netip.Addr) *device.Peer {
	if d.allowedIPs == nil {
		return nil
	}
	return d.allowedIPs.Lookup(address.AsSlice())
}

// IpcGet returns the underlying amneziawg-go device's UAPI "get" response —
// the same per-peer public_key/last_handshake_time_sec/tx_bytes/rx_bytes text
// wg-quick's "wg show" parses. There is no other way to read this endpoint's
// handshake state: it never runs as a kernel interface, and sing-box's Clash
// API connection tracker does not see traffic through it (endpoint, not
// inbound). Callers (see experimental/clashapi/awg.go) parse this text.
// Gated on started rather than a nil awgDevice: Close leaves the pointer set,
// so a nil check would report a stopped device's stale (empty) peer list as a
// live one. The atomic also orders the read — Start stores it after assigning
// awgDevice, so a true here means that write is visible to this goroutine.
func (d *Device) IpcGet() (string, error) {
	if !d.started.Load() {
		return "", E.New("device not started")
	}
	return d.awgDevice.IpcGet()
}

func (d *Device) DialContext(ctx context.Context, network string, destination metadata.Socksaddr) (net.Conn, error) {
	return d.tun.DialContext(ctx, network, destination)
}

func (d *Device) ListenPacket(ctx context.Context, destination metadata.Socksaddr) (net.PacketConn, error) {
	return d.tun.ListenPacket(ctx, destination)
}
