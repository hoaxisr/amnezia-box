package trusttunnel

// awgm: Dial/ListenPacket обязаны дождаться ответа на CONNECT — 407 и ошибка
// транспорта возвращаются вызывающему, а не всплывают на первом Read.

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/sagernet/sing/common/auth"
	"github.com/sagernet/sing/common/logger"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"

	"github.com/stretchr/testify/require"
)

func newBadAuthClient(t *testing.T) *Client {
	t.Helper()
	serverStd, clientStd := generateTestTLSPair(t)
	listener, err := net.Listen(N.NetworkTCP, "127.0.0.1:0")
	require.NoError(t, err)
	service := NewService(ServiceOptions{Ctx: t.Context(), Logger: logger.NOP(), Handler: &echoHandler{}})
	service.UpdateUsers([]auth.User{{Username: "test", Password: "test"}})
	require.NoError(t, service.Start(listener, nil, &testServerTLSConfig{config: serverStd}))
	client, err := NewClient(ClientOptions{
		Ctx:       t.Context(),
		Detour:    new(N.DefaultDialer),
		Server:    M.ParseSocksaddr(listener.Addr().String()),
		Auth:      auth.User{Username: "test", Password: "wrong"},
		TLSConfig: &testClientTLSConfig{config: clientStd},
	})
	require.NoError(t, err)
	require.NoError(t, client.Start())
	t.Cleanup(func() { client.Close(); service.Close() })
	return client
}

func TestDialReturnsAuthErrorBeforeRead(t *testing.T) {
	t.Parallel()
	client := newBadAuthClient(t)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	conn, err := client.Dial(ctx, M.ParseSocksaddr("example.com:80"))
	require.Error(t, err, "407 must surface from Dial, not from the first Read")
	require.Nil(t, conn)
	require.ErrorContains(t, err, "unexpected status code") // тестовый сервис отвечает 403, официальный endpoint — 407
}

func TestListenPacketReturnsAuthErrorEarly(t *testing.T) {
	t.Parallel()
	client := newBadAuthClient(t)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	conn, err := client.ListenPacket(ctx)
	require.Error(t, err)
	require.Nil(t, conn)
}

func TestDialHonorsContextWhileWaiting(t *testing.T) {
	t.Parallel()
	// Сервер, который принимает TCP и молчит: RoundTrip не завершится, Dial обязан
	// вернуться по ctx, а не висеть.
	listener, err := net.Listen(N.NetworkTCP, "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { listener.Close() })
	go func() {
		for {
			c, err := listener.Accept()
			if err != nil {
				return
			}
			defer c.Close()
		}
	}()
	_, clientStd := generateTestTLSPair(t)
	client, err := NewClient(ClientOptions{
		Ctx:       t.Context(),
		Detour:    new(N.DefaultDialer),
		Server:    M.ParseSocksaddr(listener.Addr().String()),
		Auth:      auth.User{Username: "test", Password: "test"},
		TLSConfig: &testClientTLSConfig{config: clientStd},
	})
	require.NoError(t, err)
	require.NoError(t, client.Start())
	t.Cleanup(func() { client.Close() })

	ctx, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	conn, err := client.Dial(ctx, M.ParseSocksaddr("example.com:80"))
	require.Error(t, err)
	require.Nil(t, conn)
	require.Less(t, time.Since(start), 3*time.Second)
}
