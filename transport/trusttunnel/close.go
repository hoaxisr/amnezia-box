package trusttunnel

import (
	stdTLS "crypto/tls"
	"net"
	"sync"
	"sync/atomic"

	"github.com/sagernet/sing/common"
	E "github.com/sagernet/sing/common/exceptions"
	N "github.com/sagernet/sing/common/network"
)

func (c *Client) forceCloseAllConnections() {
	roundTripper := c.roundTripper
	roundTripper.CloseIdleConnections()

	if tracker := c.connTracker; tracker != nil {
		_ = tracker.Close()
	}
}

type connTracker struct {
	access sync.Mutex
	conns  map[trackedConn]struct{}
}

func newConnTracker() *connTracker {
	return &connTracker{
		conns: make(map[trackedConn]struct{}),
	}
}

func (c *connTracker) track(conn net.Conn) trackedConn {
	tracked := newTrackedConn(conn, c)
	c.access.Lock()
	defer c.access.Unlock()
	c.conns[tracked] = struct{}{}
	return tracked
}

func (c *connTracker) Close() error {
	c.access.Lock()
	conns := make([]trackedConn, 0, len(c.conns))
	for conn := range c.conns {
		conns = append(conns, conn)
	}
	clear(c.conns)
	c.access.Unlock()
	var errs []error
	for _, conn := range conns {
		err := conn.closeFromTracker()
		if err != nil {
			errs = append(errs, err)
		}
	}
	return E.Errors(errs...)
}

func (c *connTracker) untrack(conn trackedConn) {
	c.access.Lock()
	defer c.access.Unlock()
	delete(c.conns, conn)
}

func newTrackedConn(conn net.Conn, tracker *connTracker) trackedConn {
	if tlsConn, isTLSConn := conn.(duckTLSConn); isTLSConn {
		return &trackedTLSConn{
			duckTLSConn: tlsConn,
			tracker:     tracker,
		}
	}
	return &trackedCommonConn{
		Conn:    conn,
		tracker: tracker,
	}
}

type trackedConn interface {
	net.Conn
	closeFromTracker() error
}

// To expose underlying syscall conn wrapper
var (
	_ trackedConn          = (*trackedCommonConn)(nil)
	_ common.WithUpstream  = (*trackedCommonConn)(nil)
	_ N.ReaderWithUpstream = (*trackedCommonConn)(nil)
	_ N.WriterWithUpstream = (*trackedCommonConn)(nil)
)

type trackedCommonConn struct {
	net.Conn
	closed  atomic.Bool
	tracker *connTracker
}

func (t *trackedCommonConn) Close() error {
	if t.closed.Swap(true) {
		return net.ErrClosed
	}
	t.tracker.untrack(t)
	return t.Conn.Close()
}

func (t *trackedCommonConn) closeFromTracker() error {
	t.closed.Store(true)
	return t.Conn.Close()
}

func (t *trackedCommonConn) WriterReplaceable() bool {
	return true
}

func (t *trackedCommonConn) ReaderReplaceable() bool {
	return true
}

func (t *trackedCommonConn) Upstream() any {
	return t.Conn
}

type duckTLSConn interface {
	net.Conn
	ConnectionState() stdTLS.ConnectionState
}

var (
	_ trackedConn          = (*trackedTLSConn)(nil)
	_ common.WithUpstream  = (*trackedTLSConn)(nil)
	_ N.ReaderWithUpstream = (*trackedTLSConn)(nil)
	_ N.WriterWithUpstream = (*trackedTLSConn)(nil)
)

type trackedTLSConn struct {
	duckTLSConn
	closed  atomic.Bool
	tracker *connTracker
}

func (t *trackedTLSConn) Close() error {
	if t.closed.Swap(true) {
		return net.ErrClosed
	}
	t.tracker.untrack(t)
	return t.duckTLSConn.Close()
}

func (t *trackedTLSConn) closeFromTracker() error {
	t.closed.Store(true)
	return t.duckTLSConn.Close()
}

func (t *trackedTLSConn) WriterReplaceable() bool {
	return true
}

func (t *trackedTLSConn) ReaderReplaceable() bool {
	return true
}

func (t *trackedTLSConn) Upstream() any {
	return t.duckTLSConn
}
