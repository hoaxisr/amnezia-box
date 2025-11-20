package awg

import (
	"net/netip"
	"os"

	wgTun "github.com/amnezia-vpn/amneziawg-go/tun"
	"github.com/sagernet/sing-box/adapter"
	tun "github.com/sagernet/sing-tun"
	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/exceptions"
	"github.com/sagernet/sing/common/logger"
)

var _ wgTun.Device = (*tun_adapter)(nil)
var _ adapter.SimpleLifecycle = (*tun_adapter)(nil)

type tun_adapter struct {
	mtu     uint32
	singtun tun.Tun
	events  chan wgTun.Event
	name    string
}

func newTun(address []netip.Prefix, allowedIps []netip.Prefix, excludedIps []netip.Prefix, mtu uint32, networkManager adapter.NetworkManager, logger logger.Logger) (*tun_adapter, error) {
	events := make(chan wgTun.Event)
	name := tun.CalculateInterfaceName("")

	singtun, err := tun.New(tun.Options{
		Name: name,
		GSO:  true,
		MTU:  uint32(mtu),
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

	return &tun_adapter{
		mtu:     mtu,
		events:  events,
		singtun: singtun,
		name:    name,
	}, nil
}

func (t *tun_adapter) Start() error {
	if err := t.singtun.Start(); err != nil {
		return exceptions.Cause(err, "start tunnel")
	}

	t.events <- wgTun.EventUp
	return nil
}

func (t *tun_adapter) File() *os.File {
	return nil
}

func (t *tun_adapter) Read(bufs [][]byte, sizes []int, offset int) (int, error) {
	n, err := t.singtun.Read(bufs[0][offset-tun.PacketOffset:])
	if err != nil {
		return 0, err
	}
	sizes[0] = n
	return 1, nil
}

func (t *tun_adapter) Write(bufs [][]byte, offset int) (int, error) {
	for _, buf := range bufs {
		common.ClearArray(buf[offset-tun.PacketOffset : offset])
		tun.PacketFillHeader(buf[offset-tun.PacketOffset:], tun.PacketIPVersion(buf[offset:]))

		if _, err := t.singtun.Write(buf[offset-tun.PacketOffset:]); err != nil {
			return 0, err
		}
	}
	return len(bufs), nil
}

func (t *tun_adapter) MTU() (int, error) {
	return int(t.mtu), nil
}

func (t *tun_adapter) Name() (string, error) {
	return t.name, nil
}

func (t *tun_adapter) Events() <-chan wgTun.Event {
	return t.events
}

func (t *tun_adapter) Close() error {
	close(t.events)
	return nil
}

func (t *tun_adapter) BatchSize() int {
	return 1
}
