//go:build with_quic

package trusttunnel

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestClientCloseClosesActiveQUICConnections(t *testing.T) {
	t.Parallel()

	dialer := newTrackingDialer()
	s := newQUICTestSetup(t, dialer)

	streams := openLiveStreams(t, s.client, 3)
	udpConns := dialer.snapshot()
	require.NotEmpty(t, udpConns, "client should have dialed at least one UDP connection")
	require.Equal(t, len(udpConns), dialer.liveCount())
	require.Positive(t, trackerLen(s.client.connTracker))
	assertStreamsStillLive(t, streams)

	require.NoError(t, s.client.Close())
	require.Equal(t, 0, dialer.liveCount(), "client.Close must close every underlying UDP connection")
	require.Equal(t, 0, trackerLen(s.client.connTracker))
	assertStreamsClosed(t, streams)
}

func TestClientResetConnectionsClosesActiveQUICConnections(t *testing.T) {
	t.Parallel()

	dialer := newTrackingDialer()
	s := newQUICTestSetup(t, dialer)

	streams := openLiveStreams(t, s.client, 3)
	oldConns := dialer.snapshot()
	require.NotEmpty(t, oldConns)

	s.client.ResetConnections()
	for i, conn := range oldConns {
		require.True(t, conn.closed.Load(), "reset must close old UDP connection %d", i)
	}
	require.Equal(t, 0, trackerLen(s.client.connTracker))
	assertStreamsClosed(t, streams)

	// The HTTP/3 transport must stay usable after a reset.
	newStream := openLiveTCP(t, s.client, []byte("after reset"))
	t.Cleanup(func() { _ = newStream.Close() })
	require.Positive(t, dialer.liveCount())
	require.Positive(t, trackerLen(s.client.connTracker))
}

func TestQUICIdleCloseReleasesTrackedConnections(t *testing.T) {
	t.Parallel()

	dialer := newTrackingDialer()
	s := newQUICTestSetup(t, dialer)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	require.NoError(t, s.client.HealthCheck(ctx))
	require.Positive(t, trackerLen(s.client.connTracker))

	// quic-go never closes a net.Conn it did not create, so an idle close must not
	// leak the UDP socket or its tracker entry.
	s.client.roundTripper.CloseIdleConnections()
	require.Eventually(t, func() bool {
		return dialer.liveCount() == 0 && trackerLen(s.client.connTracker) == 0
	}, 5*time.Second, 10*time.Millisecond, "idle QUIC connections must be untracked and closed")
}
