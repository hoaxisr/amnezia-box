//go:build with_quic

package trusttunnel

import (
	"context"
	"io"
	"net/netip"
	"testing"
	"time"

	"github.com/sagernet/quic-go/http3"
	"github.com/sagernet/sing/common/auth"
	"github.com/sagernet/sing/common/logger"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"

	"github.com/stretchr/testify/require"
)

func newQUICTestSetup(t *testing.T, detour N.Dialer) *testSetup {
	t.Helper()

	serverTLS, clientTLS := generateTestTLSPair(t)
	serverTLS.NextProtos = []string{http3.NextProtoH3}
	clientTLS.NextProtos = []string{http3.NextProtoH3}

	packetConn, err := N.SystemDialer.ListenPacket(t.Context(), M.SocksaddrFrom(netip.AddrFrom4([4]byte{127, 0, 0, 1}), 0))
	require.NoError(t, err)

	service := NewService(ServiceOptions{
		Ctx:     t.Context(),
		Logger:  logger.NOP(),
		Handler: &echoHandler{},
	})
	service.UpdateUsers([]auth.User{{Username: "test", Password: "test"}})
	require.NoError(t, service.Start(nil, packetConn, &testServerTLSConfig{config: serverTLS}))

	client, err := NewClient(ClientOptions{
		Ctx:       t.Context(),
		Detour:    detour,
		Server:    M.ParseSocksaddr(packetConn.LocalAddr().String()),
		Auth:      auth.User{Username: "test", Password: "test"},
		TLSConfig: &testClientTLSConfig{config: clientTLS},
		QUIC:      true,
	})
	require.NoError(t, err)
	require.NoError(t, client.Start())
	t.Cleanup(func() {
		_ = client.Close()
		_ = service.Close()
	})

	return &testSetup{service: service, client: client}
}

func TestRoundtripQUIC(t *testing.T) {
	t.Parallel()

	s := newQUICTestSetup(t, new(N.DefaultDialer))

	dialCtx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	conn, err := s.client.Dial(dialCtx, M.ParseSocksaddr("example.com:80"))
	require.NoError(t, err)
	defer conn.Close()

	payload := []byte("hello trusttunnel quic")
	_, err = conn.Write(payload)
	require.NoError(t, err)
	got := make([]byte, len(payload))
	_, err = io.ReadFull(conn, got)
	require.NoError(t, err)
	require.Equal(t, payload, got)
}
