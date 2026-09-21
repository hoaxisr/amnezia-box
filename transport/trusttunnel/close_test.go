package trusttunnel

import (
	"context"
	"io"
	"net"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"

	"github.com/stretchr/testify/require"
	"golang.org/x/net/http2"
)

func TestConnTrackerCloseClosesAll(t *testing.T) {
	t.Parallel()

	tracker := newConnTracker()
	const count = 5
	remotes := make([]net.Conn, 0, count)
	t.Cleanup(func() {
		for _, remote := range remotes {
			_ = remote.Close()
		}
	})
	for range count {
		local, remote := net.Pipe()
		remotes = append(remotes, remote)
		_ = tracker.track(local)
	}

	require.Equal(t, count, trackerLen(tracker))
	require.NoError(t, tracker.Close())
	require.Equal(t, 0, trackerLen(tracker))

	for i, remote := range remotes {
		_, err := remote.Read(make([]byte, 1))
		require.Error(t, err, "remote %d should see the tracked conn close", i)
	}
}

func TestConnTrackerUntrackOnClose(t *testing.T) {
	t.Parallel()

	tracker := newConnTracker()
	local, remote := net.Pipe()
	defer remote.Close()
	tracked := tracker.track(local)
	require.Equal(t, 1, trackerLen(tracker))

	require.NoError(t, tracked.Close())
	require.Equal(t, 0, trackerLen(tracker))
	require.ErrorIs(t, tracked.Close(), net.ErrClosed)
}

func TestNewTrackedConnWrapsTLSAndPlain(t *testing.T) {
	t.Parallel()

	t.Run("plain", func(t *testing.T) {
		t.Parallel()
		local, remote := net.Pipe()
		defer local.Close()
		defer remote.Close()
		_, ok := newTrackedConn(local, newConnTracker()).(*trackedCommonConn)
		require.True(t, ok)
	})

	t.Run("tls", func(t *testing.T) {
		t.Parallel()
		local, remote := net.Pipe()
		defer local.Close()
		defer remote.Close()
		tracked := newTrackedConn(&fakeTLSConn{Conn: local}, newConnTracker())
		tlsConn, isTLSConn := tracked.(*trackedTLSConn)
		require.True(t, isTLSConn)
		require.Equal(t, http2.NextProtoTLS, tlsConn.ConnectionState().NegotiatedProtocol)
	})
}

func TestTrackedTLSConnDoubleClose(t *testing.T) {
	t.Parallel()

	local, remote := net.Pipe()
	defer remote.Close()
	tracker := newConnTracker()
	tracked := tracker.track(&fakeTLSConn{Conn: local})
	require.NoError(t, tracked.Close())
	require.ErrorIs(t, tracked.Close(), net.ErrClosed)
	require.Equal(t, 0, trackerLen(tracker))
}

func TestClientCloseClosesActiveConnections(t *testing.T) {
	t.Parallel()

	dialer := newTrackingDialer()
	serverStd, clientStd := generateTestTLSPair(t)
	s := newTestSetupWith(t, &testServerTLSConfig{config: serverStd}, &testClientTLSConfig{config: clientStd}, dialer)

	streams := openLiveStreams(t, s.client, 3)
	tcpConns := dialer.snapshot()
	require.NotEmpty(t, tcpConns, "client should have dialed at least one TCP connection")
	require.Equal(t, len(tcpConns), dialer.liveCount())
	require.Positive(t, trackerLen(s.client.connTracker))

	// CloseIdleConnections must not drop an HTTP/2 session that still has open streams.
	s.client.roundTripper.CloseIdleConnections()
	require.Equal(t, len(tcpConns), dialer.liveCount(), "idle close must not drop active connections")
	assertStreamsStillLive(t, streams)

	require.NoError(t, s.client.Close())
	require.Equal(t, 0, dialer.liveCount(), "client.Close must close every underlying TCP connection")
	require.Equal(t, 0, trackerLen(s.client.connTracker))
	assertStreamsClosed(t, streams)
}

func TestClientResetConnectionsClosesActiveConnections(t *testing.T) {
	t.Parallel()

	dialer := newTrackingDialer()
	serverStd, clientStd := generateTestTLSPair(t)
	s := newTestSetupWith(t, &testServerTLSConfig{config: serverStd}, &testClientTLSConfig{config: clientStd}, dialer)

	streams := openLiveStreams(t, s.client, 3)
	oldConns := dialer.snapshot()
	require.NotEmpty(t, oldConns)

	s.client.ResetConnections()
	for i, conn := range oldConns {
		require.True(t, conn.closed.Load(), "reset must close old TCP connection %d", i)
	}
	require.Equal(t, 0, trackerLen(s.client.connTracker))
	assertStreamsClosed(t, streams)

	newStream := openLiveTCP(t, s.client, []byte("after reset"))
	t.Cleanup(func() { _ = newStream.Close() })
	require.Positive(t, dialer.liveCount())
	require.Positive(t, trackerLen(s.client.connTracker))
}

func TestClientCloseClosesActiveFakeTLSConnections(t *testing.T) {
	t.Parallel()

	dialer := newTrackingDialer()
	s := newTestSetupWith(t, &fakeTLSConfig{}, &fakeTLSConfig{}, dialer)

	streams := openLiveStreams(t, s.client, 2)
	require.NotEmpty(t, dialer.snapshot())
	require.Equal(t, len(dialer.snapshot()), dialer.liveCount())

	require.NoError(t, s.client.Close())
	require.Equal(t, 0, dialer.liveCount())
	require.Equal(t, 0, trackerLen(s.client.connTracker))
	assertStreamsClosed(t, streams)
}

type trackingDialer struct {
	inner  N.Dialer
	access sync.Mutex
	conns  []*closeTrackingConn
}

func newTrackingDialer() *trackingDialer {
	return &trackingDialer{inner: new(N.DefaultDialer)}
}

func (d *trackingDialer) DialContext(ctx context.Context, network string, destination M.Socksaddr) (net.Conn, error) {
	conn, err := d.inner.DialContext(ctx, network, destination)
	if err != nil {
		return nil, err
	}
	wrapped := &closeTrackingConn{Conn: conn}
	d.access.Lock()
	d.conns = append(d.conns, wrapped)
	d.access.Unlock()
	return wrapped, nil
}

func (d *trackingDialer) ListenPacket(ctx context.Context, destination M.Socksaddr) (net.PacketConn, error) {
	return d.inner.ListenPacket(ctx, destination)
}

func (d *trackingDialer) snapshot() []*closeTrackingConn {
	d.access.Lock()
	defer d.access.Unlock()
	return slices.Clone(d.conns)
}

func (d *trackingDialer) liveCount() int {
	d.access.Lock()
	defer d.access.Unlock()
	n := 0
	for _, conn := range d.conns {
		if !conn.closed.Load() {
			n++
		}
	}
	return n
}

type closeTrackingConn struct {
	net.Conn
	closed atomic.Bool
}

func (c *closeTrackingConn) Close() error {
	c.closed.Store(true)
	return c.Conn.Close()
}

func (c *closeTrackingConn) Upstream() any {
	return c.Conn
}

func (c *closeTrackingConn) ReaderReplaceable() bool {
	return true
}

func (c *closeTrackingConn) WriterReplaceable() bool {
	return true
}

func openLiveStreams(t *testing.T, client *Client, n int) []io.Closer {
	t.Helper()
	streams := make([]io.Closer, 0, n)
	t.Cleanup(func() {
		for _, stream := range streams {
			_ = stream.Close()
		}
	})
	for i := range n {
		payload := []byte{byte('a' + i)}
		streams = append(streams, openLiveTCP(t, client, payload))
	}
	return streams
}

func openLiveTCP(t *testing.T, client *Client, payload []byte) net.Conn {
	t.Helper()
	conn, err := client.Dial(t.Context(), M.ParseSocksaddr("example.com:80"))
	require.NoError(t, err)
	_, err = conn.Write(payload)
	require.NoError(t, err)
	got := make([]byte, len(payload))
	_, err = io.ReadFull(conn, got)
	require.NoError(t, err)
	require.Equal(t, payload, got)
	return conn
}

func assertStreamsStillLive(t *testing.T, streams []io.Closer) {
	t.Helper()
	for i, stream := range streams {
		conn := stream.(net.Conn)
		payload := []byte{byte('A' + i)}
		_, err := conn.Write(payload)
		require.NoError(t, err, "stream %d should still accept writes", i)
		got := make([]byte, len(payload))
		_, err = io.ReadFull(conn, got)
		require.NoError(t, err, "stream %d should still accept reads", i)
		require.Equal(t, payload, got)
	}
}

func assertStreamsClosed(t *testing.T, streams []io.Closer) {
	t.Helper()
	for i, stream := range streams {
		conn := stream.(net.Conn)
		errCh := make(chan error, 1)
		go func() {
			_, err := conn.Read(make([]byte, 1))
			errCh <- err
		}()
		ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
		select {
		case err := <-errCh:
			cancel()
			require.Error(t, err, "stream %d should fail after the client closed its connections", i)
		case <-ctx.Done():
			cancel()
			t.Fatalf("stream %d read still blocked after connections were closed", i)
		}
	}
}

func trackerLen(tracker *connTracker) int {
	if tracker == nil {
		return 0
	}
	tracker.access.Lock()
	defer tracker.access.Unlock()
	return len(tracker.conns)
}
