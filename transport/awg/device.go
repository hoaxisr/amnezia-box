package awg

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"reflect"
	"strings"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/amnezia-vpn/amneziawg-go/conn"
	"github.com/amnezia-vpn/amneziawg-go/device"

	"github.com/sagernet/sing-box/adapter"
	tun "github.com/sagernet/sing-tun"
	"github.com/sagernet/sing/common/exceptions"
	E "github.com/sagernet/sing/common/exceptions"
	"github.com/sagernet/sing/common/logger"
	"github.com/sagernet/sing/common/metadata"
	"github.com/sagernet/sing/common/network"
)

type DeviceOpts struct {
	UseIntegratedTun bool
	Address          []netip.Prefix
	AllowedIps       []netip.Prefix
	ExcludedIps      []netip.Prefix
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
	// amneziawg-go keeps the peer allowed-ips trie private; the same
	// reflect+unsafe access as transport/wireguard.Endpoint.Start is used
	// here to back PreferredAddress (Device.Lookup).
	d.allowedIPs = (*device.AllowedIPs)(unsafe.Pointer(reflect.Indirect(reflect.ValueOf(d.awgDevice)).FieldByName("allowedips").UnsafeAddr()))
	if err := d.awgDevice.IpcSet(d.ipcConfig); err != nil {
		return E.Cause(err, "set ipc config")
	}

	if err := d.awgDevice.Up(); err != nil {
		return err
	}
	d.started.Store(true)
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
func (d *Device) IpcGet() (string, error) {
	if d.awgDevice == nil {
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
