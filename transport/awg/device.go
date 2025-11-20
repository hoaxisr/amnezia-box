package awg

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"strings"

	"github.com/amnezia-vpn/amneziawg-go/device"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing/common/logger"
	"github.com/sagernet/sing/common/metadata"
	"github.com/sagernet/sing/common/network"
)

type DeviceOpts struct {
	Address     []netip.Prefix
	AllowedIps  []netip.Prefix
	ExcludedIps []netip.Prefix
	MTU         uint32
}

type Device struct {
	awgDevice *device.Device
	tun       tunAdapter
}

func NewDevice(ctx context.Context, logger logger.ContextLogger, dial network.Dialer, opts DeviceOpts) (*Device, error) {
	// tun, err := newSystemTun(ctx, opts.Address, opts.AllowedIps, opts.ExcludedIps, opts.MTU, logger)
	// if err != nil {
	// 	return nil, exceptions.Cause(err, "create tunnel")
	// }

	tun, err := newNetworkTun(opts.Address, opts.MTU)
	if err != nil {
		return nil, err
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
		awgDevice: device.NewDevice(tun, newBind(dial), wgLogger),
		tun:       tun,
	}, nil
}

func (d *Device) Start(stage adapter.StartStage) error {
	if stage != adapter.StartStateStart {
		return nil
	}
	return d.tun.Start()
}

func (d *Device) Close() error {
	return d.awgDevice.Down()
}

func (d *Device) SetIpcConfig(s string) error {
	return d.awgDevice.IpcSet(s)
}

func (d *Device) DialContext(ctx context.Context, network string, destination metadata.Socksaddr) (net.Conn, error) {
	return d.tun.DialContext(ctx, network, destination)
}

func (d *Device) ListenPacket(ctx context.Context, destination metadata.Socksaddr) (net.PacketConn, error) {
	return d.tun.ListenPacket(ctx, destination)
}
