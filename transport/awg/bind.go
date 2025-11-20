package awg

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"sync"
	"syscall"

	"github.com/amnezia-vpn/amneziawg-go/conn"
	"github.com/sagernet/sing/common/exceptions"
	"github.com/sagernet/sing/common/metadata"
	"github.com/sagernet/sing/common/network"
)

var _ conn.Bind = (*bind_adapter)(nil)

type bind_adapter struct {
	conn4  *net.UDPConn
	conn6  *net.UDPConn
	dialer network.Dialer
	ctx    context.Context
	mutex  sync.Mutex
}

func newBind(dial network.Dialer) conn.Bind {
	return &bind_adapter{
		dialer: dial,
	}
}

func (b *bind_adapter) connect(addr netip.Addr, port uint16) (*net.UDPConn, error) {
	conn, err := b.dialer.ListenPacket(b.ctx, metadata.Socksaddr{Addr: addr, Port: port})
	if err != nil {
		return nil, err
	}

	udpConn, ok := conn.(*net.UDPConn)
	if !ok {
		conn.Close()
		return nil, errors.ErrUnsupported
	}

	return udpConn, nil
}

func (*bind_adapter) receive(c *net.UDPConn) conn.ReceiveFunc {
	return func(packets [][]byte, sizes []int, eps []conn.Endpoint) (n int, err error) {
		n, peerAp, err := c.ReadFromUDPAddrPort(packets[0])
		if err != nil {
			return 0, err
		}
		sizes[0] = n
		eps[0] = &bind_endpoint{AddrPort: peerAp}
		return 1, nil
	}
}

func (b *bind_adapter) Open(port uint16) (fns []conn.ReceiveFunc, actualPort uint16, err error) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	if b.conn4 != nil || b.conn6 != nil {
		return nil, 0, conn.ErrBindAlreadyOpen
	}

	conn4, err := b.connect(netip.IPv4Unspecified(), port)
	if err != nil && !errors.Is(err, syscall.EAFNOSUPPORT) {
		return nil, 0, exceptions.Cause(err, "create ipv4 connection")
	}
	if conn4 != nil {
		fns = append(fns, b.receive(conn4))
	}

	conn6, err := b.connect(netip.IPv6Unspecified(), port)
	if err != nil && !errors.Is(err, syscall.EAFNOSUPPORT) {
		return nil, 0, exceptions.Cause(err, "create ipv6 connection")
	}
	if conn6 != nil {
		fns = append(fns, b.receive(conn6))
	}

	b.conn4 = conn4
	b.conn6 = conn6

	return fns, port, nil
}

func (b *bind_adapter) Close() error {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	var err4, err6 error

	if b.conn4 != nil {
		err4 = b.conn4.Close()
		b.conn4 = nil
	}

	if b.conn6 != nil {
		err6 = b.conn6.Close()
		b.conn6 = nil
	}

	return errors.Join(err4, err6)
}

func (b *bind_adapter) SetMark(mark uint32) error {
	return nil
}

func (b *bind_adapter) Send(bufs [][]byte, ep conn.Endpoint) error {
	var conn *net.UDPConn
	if ep.DstIP().Is6() {
		conn = b.conn6
	} else {
		conn = b.conn4
	}

	udpEp, ok := ep.(*bind_endpoint)
	if !ok {
		return errors.ErrUnsupported
	}

	for _, buf := range bufs {
		if _, err := conn.WriteToUDPAddrPort(buf, udpEp.AddrPort); err != nil {
			return err
		}
	}

	return nil
}

func (b *bind_adapter) ParseEndpoint(s string) (conn.Endpoint, error) {
	ap, err := netip.ParseAddrPort(s)
	if err != nil {
		return nil, err
	}
	return &bind_endpoint{AddrPort: ap}, nil
}

func (b *bind_adapter) BatchSize() int {
	return 1
}

var _ conn.Endpoint = (*bind_endpoint)(nil)

type bind_endpoint struct {
	AddrPort netip.AddrPort
}

func (e bind_endpoint) ClearSrc() {
}

func (e bind_endpoint) SrcToString() string {
	return ""
}

func (e bind_endpoint) DstToString() string {
	return e.AddrPort.String()
}

func (e bind_endpoint) DstToBytes() []byte {
	b, _ := e.AddrPort.MarshalBinary()
	return b
}

func (e bind_endpoint) DstIP() netip.Addr {
	return e.AddrPort.Addr()
}

func (e bind_endpoint) SrcIP() netip.Addr {
	return netip.Addr{}
}
