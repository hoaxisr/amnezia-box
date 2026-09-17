package awg

import (
	"context"
	"net"
	"net/netip"
	"os"
	"sync"
	"time"

	awgTun "github.com/amnezia-vpn/amneziawg-go/v3/tun"

	"github.com/amnezia-vpn/amneziawg-go/v3/conn"
	C "github.com/sagernet/sing-box/constant"
	tun "github.com/sagernet/sing-tun"
	E "github.com/sagernet/sing/common/exceptions"
	"github.com/sagernet/sing/common/logger"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

// stackTun is the userspace TCP/IP stack of the AWG endpoint: sing-tun's Go
// stack driven by an in-memory tun. Outgoing connections are dialed through it
// (client role), and packets arriving from the tunnel towards arbitrary
// destinations are handed to the Handler (gateway/exit role). It mirrors
// transport/wireguard.stackDevice.
type stackTun struct {
	stack        *tun.Go
	memoryTun    *tun.MemoryTun
	mtu          uint32
	events       chan awgTun.Event
	closeOnce    sync.Once
	inet4Address netip.Addr
	inet6Address netip.Addr
}

func newNonIntegratedTun(
	ctx context.Context,
	address []netip.Prefix,
	mtu uint32,
	handler tun.Handler,
	udpTimeout time.Duration,
	memoryPressure func() tun.MemoryPressure,
	log logger.ContextLogger,
) (tunAdapter, error) {
	memoryTun := tun.NewMemoryTun(tun.MemoryTunOptions{MTU: int(mtu)})
	goStack, err := tun.NewGo(tun.StackOptions{
		Context:     ctx,
		Tun:         memoryTun,
		TunOptions:  tun.Options{MTU: mtu},
		UDPTimeout:  udpTimeout,
		ICMPTimeout: C.ICMPTimeout,
		Handler:     handler,
		Logger:      log,
		// The router has 256 MB of RAM: the stack must shrink its slab pool
		// when the OOM killer reports pressure, exactly like the tun inbound.
		MemoryPressure: memoryPressure,
	})
	if err != nil {
		memoryTun.Close()
		return nil, err
	}
	tunDevice := &stackTun{
		stack:     goStack,
		memoryTun: memoryTun,
		mtu:       mtu,
		events:    make(chan awgTun.Event, 1),
	}
	for _, prefix := range address {
		if prefix.Addr().Is4() {
			tunDevice.inet4Address = prefix.Addr()
		} else {
			tunDevice.inet6Address = prefix.Addr()
		}
	}
	return tunDevice, nil
}

func (t *stackTun) DialContext(ctx context.Context, network string, destination M.Socksaddr) (net.Conn, error) {
	bind, err := t.bindAddress(destination)
	if err != nil {
		return nil, err
	}
	switch N.NetworkName(network) {
	case N.NetworkTCP:
		tcpConn, dialErr := t.stack.DialTCP(ctx, bind, destination.AddrPort())
		if dialErr != nil {
			return nil, dialErr
		}
		tcpConn.SetKeepAliveConfig(net.KeepAliveConfig{Enable: true, Idle: 15 * time.Second, Interval: 15 * time.Second})
		return tcpConn, nil
	case N.NetworkUDP:
		return t.stack.DialUDP(netip.AddrPortFrom(bind, 0), destination.AddrPort())
	default:
		return nil, E.Extend(N.ErrUnknownNetwork, network)
	}
}

func (t *stackTun) ListenPacket(ctx context.Context, destination M.Socksaddr) (net.PacketConn, error) {
	bind, err := t.bindAddress(destination)
	if err != nil {
		return nil, err
	}
	return t.stack.ListenUDP(netip.AddrPortFrom(bind, 0))
}

func (t *stackTun) bindAddress(destination M.Socksaddr) (netip.Addr, error) {
	if destination.IsIPv4() {
		if !t.inet4Address.IsValid() {
			return netip.Addr{}, E.New("missing IPv4 local address")
		}
		return t.inet4Address, nil
	}
	if !t.inet6Address.IsValid() {
		return netip.Addr{}, E.New("missing IPv6 local address")
	}
	return t.inet6Address, nil
}

func (t *stackTun) Start() error {
	err := t.stack.Start()
	if err != nil {
		return err
	}
	t.events <- awgTun.EventUp
	return nil
}

func (t *stackTun) File() *os.File {
	return nil
}

func (t *stackTun) Read(bufs [][]byte, sizes []int, offset int) (int, error) {
	return t.memoryTun.ReadPackets(bufs, sizes, offset)
}

func (t *stackTun) Write(bufs [][]byte, offset int) (int, error) {
	packets := make([][]byte, 0, len(bufs))
	for _, packet := range bufs {
		packets = append(packets, packet[offset:])
	}
	return t.memoryTun.WritePackets(packets)
}

func (t *stackTun) Flush() error {
	return nil
}

func (t *stackTun) MTU() (int, error) {
	return int(t.mtu), nil
}

func (t *stackTun) Name() (string, error) {
	return "sing-box", nil
}

func (t *stackTun) Events() <-chan awgTun.Event {
	return t.events
}

func (t *stackTun) Close() error {
	var err error
	t.closeOnce.Do(func() {
		close(t.events)
		err = E.Errors(t.stack.Close(), t.memoryTun.Close())
	})
	return err
}

func (t *stackTun) BatchSize() int {
	return conn.IdealBatchSize
}
