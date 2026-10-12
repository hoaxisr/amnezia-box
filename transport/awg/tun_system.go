package awg

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"os"
	"sync"
	"sync/atomic"

	"github.com/amnezia-vpn/amneziawg-go/v3/device"
	awgTun "github.com/amnezia-vpn/amneziawg-go/v3/tun"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/common/dialer"
	"github.com/sagernet/sing-box/common/kernelports"
	"github.com/sagernet/sing-box/option"
	tun "github.com/sagernet/sing-tun"
	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/exceptions"
	"github.com/sagernet/sing/common/logger"
	"github.com/sagernet/sing/common/metadata"
	"github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/service"
)

type systemTun struct {
	mtu       uint32
	singtun   tun.Tun
	batchTun  tun.LinuxTUN
	events    chan awgTun.Event
	name      string
	dialer    network.Dialer
	logger    logger.Logger
	ports     atomic.Pointer[kernelports.Pool]
	closeOnce sync.Once
}

func newSystemTun(ctx context.Context, address []netip.Prefix, allowedIps []netip.Prefix, excludedIps []netip.Prefix, mtu uint32, logger logger.Logger) (tunAdapter, error) {
	networkManager := service.FromContext[adapter.NetworkManager](ctx)
	name := tun.CalculateInterfaceName("")
	// Buffered so Start can push EventUp before the AWG device's event reader
	// exists: the device is now created after the tun is started (see
	// Device.Start), so an unbuffered send here would deadlock.
	events := make(chan awgTun.Event, 1)

	dial, err := dialer.NewDefault(ctx, option.DialerOptions{
		AbstractDialerOptions: option.AbstractDialerOptions{
			BindInterface: name,
		},
	})
	if err != nil {
		return nil, exceptions.Cause(err, "get in-tunnel dialer")
	}

	singtun, err := tun.New(tun.Options{
		Name: name,
		GSO:  true,
		MTU:  uint32(mtu),
		// This is a transport interface, not the system tun, so it must not
		// touch DNS whatever the default is (mirrors transport/wireguard).
		DNSMode: tun.DNSModeDisabled,
		Inet4Address: common.Filter(address, func(it netip.Prefix) bool {
			return it.Addr().Is4()
		}),
		Inet6Address: common.Filter(address, func(it netip.Prefix) bool {
			return it.Addr().Is6()
		}),
		InterfaceMonitor: networkManager.InterfaceMonitor(),
		InterfaceFinder:  networkManager.InterfaceFinder(),
		Inet4RouteAddress: common.Filter(allowedIps, func(it netip.Prefix) bool {
			return it.Addr().Is4()
		}),
		Inet6RouteAddress: common.Filter(allowedIps, func(it netip.Prefix) bool {
			return it.Addr().Is6()
		}),
		Inet4RouteExcludeAddress: common.Filter(excludedIps, func(it netip.Prefix) bool {
			return it.Addr().Is4()
		}),
		Inet6RouteExcludeAddress: common.Filter(excludedIps, func(it netip.Prefix) bool {
			return it.Addr().Is6()
		}),
		Logger: logger,
	})
	if err != nil {
		return nil, exceptions.Cause(err, "create tunnel")
	}

	return &systemTun{
		mtu:     mtu,
		events:  events,
		singtun: singtun,
		name:    name,
		dialer:  dial,
		logger:  logger,
	}, nil
}

func (t *systemTun) SetDevice(device *device.Device, peers []*device.Peer) {
}

func (t *systemTun) Start() error {
	if err := t.singtun.Start(); err != nil {
		// Device.Close only reaches the tun through awgDevice, which does
		// not exist yet when Start fails, so release the interface here.
		t.singtun.Close()
		return exceptions.Cause(err, "start tunnel")
	}

	if batchTun, ok := t.singtun.(tun.LinuxTUN); ok && batchTun.BatchSize() > 1 {
		t.batchTun = batchTun
	}
	// Forward NAT picks selector ports that kernel sockets of this endpoint
	// must not collide with (mirrors transport/wireguard systemDevice).
	ports, err := kernelports.New()
	if err != nil {
		t.logger.Warn(exceptions.Cause(err, "reserve selector ports"))
	} else {
		t.ports.Store(ports)
	}

	t.events <- awgTun.EventUp
	return nil
}

func (t *systemTun) File() *os.File {
	return nil
}

func (t *systemTun) Read(bufs [][]byte, sizes []int, offset int) (int, error) {
	var (
		n   int
		err error
	)
	if t.batchTun != nil {
		n, err = t.batchTun.BatchRead(bufs, offset-tun.PacketOffset, sizes)
	} else {
		sizes[0], err = t.singtun.Read(bufs[0][offset-tun.PacketOffset:])
		if err == nil {
			n = 1
		}
	}
	// amneziawg-go drops the batch on its own ErrTooManySegments and closes
	// the device on any other read error.
	if errors.Is(err, tun.ErrTooManySegments) {
		err = awgTun.ErrTooManySegments
	}
	return n, err
}

func (t *systemTun) Write(bufs [][]byte, offset int) (int, error) {
	if t.batchTun != nil {
		return t.batchTun.BatchWrite(bufs, offset)
	}
	for _, buf := range bufs {
		common.ClearArray(buf[offset-tun.PacketOffset : offset])
		tun.PacketFillHeader(buf[offset-tun.PacketOffset:], tun.PacketIPVersion(buf[offset:]))

		if _, err := t.singtun.Write(buf[offset-tun.PacketOffset:]); err != nil {
			return 0, err
		}
	}
	return len(bufs), nil
}

func (t *systemTun) MTU() (int, error) {
	return int(t.mtu), nil
}

func (t *systemTun) Name() (string, error) {
	return t.name, nil
}

func (t *systemTun) Events() <-chan awgTun.Event {
	return t.events
}

func (t *systemTun) Close() error {
	var err error
	t.closeOnce.Do(func() {
		close(t.events)
		err = t.singtun.Close()
		if ports := t.ports.Load(); ports != nil {
			err = exceptions.Errors(err, ports.Close())
		}
	})
	return err
}

func (t *systemTun) UpstreamPort() any {
	if ports := t.ports.Load(); ports != nil {
		return ports
	}
	return nil
}

func (t *systemTun) BatchSize() int {
	if t.batchTun != nil {
		return t.batchTun.BatchSize()
	}
	return 1
}

func (t *systemTun) DialContext(ctx context.Context, network string, destination metadata.Socksaddr) (net.Conn, error) {
	return t.dialer.DialContext(ctx, network, destination)
}

func (t *systemTun) ListenPacket(ctx context.Context, destination metadata.Socksaddr) (net.PacketConn, error) {
	return t.dialer.ListenPacket(ctx, destination)
}
