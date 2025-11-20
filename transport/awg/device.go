package awg

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"strings"

	"github.com/amnezia-vpn/amneziawg-go/device"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/common/dialer"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common/exceptions"
	"github.com/sagernet/sing/common/logger"
	"github.com/sagernet/sing/common/metadata"
	"github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/service"
)

type DeviceOpts struct {
	Address     []netip.Prefix
	AllowedIps  []netip.Prefix
	ExcludedIps []netip.Prefix
	MTU         uint32
}

type Device struct {
	wgDevice  *device.Device
	tun       *tun_adapter
	tunDialer network.Dialer
}

func NewDevice(ctx context.Context, logger logger.ContextLogger, dial network.Dialer, opts DeviceOpts) (*Device, error) {
	networkManager := service.FromContext[adapter.NetworkManager](ctx)

	tun, err := newTun(opts.Address, opts.AllowedIps, opts.ExcludedIps, opts.MTU, networkManager, logger)
	if err != nil {
		return nil, exceptions.Cause(err, "create tunnel")
	}

	tunName, err := tun.Name()
	if err != nil {
		return nil, exceptions.Cause(err, "get tunnel name")
	}

	tunDialer, err := dialer.NewDefault(ctx, option.DialerOptions{
		BindInterface: tunName,
	})
	if err != nil {
		return nil, exceptions.Cause(err, "get in-tunnel dialer")
	}

	wgLogger := &device.Logger{
		Verbosef: func(format string, args ...interface{}) {
			logger.Debug(fmt.Sprintf(strings.ToLower(format), args...))
		},
		Errorf: func(format string, args ...interface{}) {
			logger.Error(fmt.Sprintf(strings.ToLower(format), args...))
		},
	}

	return &Device{
		wgDevice:  device.NewDevice(tun, newBind(dial), wgLogger),
		tun:       tun,
		tunDialer: tunDialer,
	}, nil
}

func (d *Device) Start(stage adapter.StartStage) error {
	if stage != adapter.StartStateStart {
		return nil
	}
	return d.tun.Start()
}

func (d *Device) Close() error {
	return d.tun.Close()
}

func (d *Device) SetIpcConfig(s string) error {
	return d.wgDevice.IpcSet(s)
}

func (e *Device) DialContext(ctx context.Context, network string, destination metadata.Socksaddr) (net.Conn, error) {
	return e.tunDialer.DialContext(ctx, network, destination)
}

func (e *Device) ListenPacket(ctx context.Context, destination metadata.Socksaddr) (net.PacketConn, error) {
	return e.tunDialer.ListenPacket(ctx, destination)
}
