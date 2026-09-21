package trusttunnel

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	stdtls "crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"io"
	"math/big"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/sagernet/sing/common/auth"
	"github.com/sagernet/sing/common/buf"
	"github.com/sagernet/sing/common/bufio"
	"github.com/sagernet/sing/common/logger"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/common/tls"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/http2"
)

// testClientTLSConfig wraps *stdtls.Config to satisfy singtls.Config.
type testClientTLSConfig struct{ config *stdtls.Config }

func (c *testClientTLSConfig) ServerName() string                 { return c.config.ServerName }
func (c *testClientTLSConfig) SetServerName(serverName string)    { c.config.ServerName = serverName }
func (c *testClientTLSConfig) NextProtos() []string               { return c.config.NextProtos }
func (c *testClientTLSConfig) SetNextProtos(p []string)           { c.config.NextProtos = p }
func (c *testClientTLSConfig) HandshakeTimeout() time.Duration    { return 0 }
func (c *testClientTLSConfig) SetHandshakeTimeout(time.Duration)  {}
func (c *testClientTLSConfig) STDConfig() (*stdtls.Config, error) { return c.config, nil }
func (c *testClientTLSConfig) Client(conn net.Conn) (tls.Conn, error) {
	return stdtls.Client(conn, c.config), nil
}

func (c *testClientTLSConfig) Clone() tls.Config {
	return &testClientTLSConfig{config: c.config.Clone()}
}

// testServerTLSConfig wraps *stdtls.Config to satisfy singtls.ServerConfig.
type testServerTLSConfig struct{ config *stdtls.Config }

func (s *testServerTLSConfig) ServerName() string                 { return s.config.ServerName }
func (s *testServerTLSConfig) SetServerName(serverName string)    { s.config.ServerName = serverName }
func (s *testServerTLSConfig) NextProtos() []string               { return s.config.NextProtos }
func (s *testServerTLSConfig) SetNextProtos(p []string)           { s.config.NextProtos = p }
func (s *testServerTLSConfig) HandshakeTimeout() time.Duration    { return 0 }
func (s *testServerTLSConfig) SetHandshakeTimeout(time.Duration)  {}
func (s *testServerTLSConfig) STDConfig() (*stdtls.Config, error) { return s.config, nil }
func (s *testServerTLSConfig) Client(conn net.Conn) (tls.Conn, error) {
	return stdtls.Client(conn, s.config), nil
}

func (s *testServerTLSConfig) Clone() tls.Config {
	return &testServerTLSConfig{config: s.config.Clone()}
}
func (s *testServerTLSConfig) Start() error { return nil }
func (s *testServerTLSConfig) Close() error { return nil }
func (s *testServerTLSConfig) Server(conn net.Conn) (tls.Conn, error) {
	return stdtls.Server(conn, s.config), nil
}

type fakeTLSConfig struct {
	serverName string
	nextProtos []string
}

func (c *fakeTLSConfig) Start() error {
	return nil
}

func (c *fakeTLSConfig) Close() error {
	return nil
}

func (c *fakeTLSConfig) ServerName() string {
	return c.serverName
}

func (c *fakeTLSConfig) SetServerName(serverName string) {
	c.serverName = serverName
}

func (c *fakeTLSConfig) NextProtos() []string {
	return c.nextProtos
}

func (c *fakeTLSConfig) SetNextProtos(nextProtos []string) {
	c.nextProtos = nextProtos
}

func (c *fakeTLSConfig) HandshakeTimeout() time.Duration {
	return 0
}

func (c *fakeTLSConfig) SetHandshakeTimeout(time.Duration) {
}

func (c *fakeTLSConfig) STDConfig() (*stdtls.Config, error) {
	return nil, nil
}

func (c *fakeTLSConfig) Client(conn net.Conn) (tls.Conn, error) {
	return &fakeTLSConn{Conn: conn}, nil
}

func (c *fakeTLSConfig) Clone() tls.Config {
	return &fakeTLSConfig{
		serverName: c.serverName,
		nextProtos: append([]string(nil), c.nextProtos...),
	}
}

func (c *fakeTLSConfig) Server(conn net.Conn) (tls.Conn, error) {
	return &fakeTLSConn{Conn: conn}, nil
}

var _ duckTLSConn = (*fakeTLSConn)(nil)

type fakeTLSConn struct {
	net.Conn
}

func (c *fakeTLSConn) NetConn() net.Conn                      { return c.Conn }
func (c *fakeTLSConn) HandshakeContext(context.Context) error { return nil }
func (c *fakeTLSConn) ConnectionState() stdtls.ConnectionState {
	return stdtls.ConnectionState{
		NegotiatedProtocol: http2.NextProtoTLS,
	}
}

// echoHandler echoes all TCP streams and UDP packets back to the sender.
type echoHandler struct{}

func (h *echoHandler) NewConnectionEx(ctx context.Context, conn net.Conn, _, _ M.Socksaddr, onClose N.CloseHandlerFunc) {
	go func() {
		defer onClose(nil)
		defer conn.Close()
		_, _ = bufio.Copy(conn, conn)
	}()
}

func (h *echoHandler) NewPacketConnectionEx(ctx context.Context, conn N.PacketConn, _, _ M.Socksaddr, onClose N.CloseHandlerFunc) {
	go func() {
		defer onClose(nil)
		defer conn.Close()
		for {
			buffer := buf.NewPacket()
			destination, err := conn.ReadPacket(buffer)
			if err != nil {
				buffer.Release()
				return
			}
			err = conn.WritePacket(buffer, destination)
			if err != nil {
				return
			}
		}
	}()
}

func generateTestTLSPair(t *testing.T) (serverStd, clientStd *stdtls.Config) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test"},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(certDER)
	require.NoError(t, err)

	pool := x509.NewCertPool()
	pool.AddCert(cert)

	tlsCert := stdtls.Certificate{Certificate: [][]byte{certDER}, PrivateKey: key}
	serverStd = &stdtls.Config{
		Certificates: []stdtls.Certificate{tlsCert},
		NextProtos:   []string{http2.NextProtoTLS},
	}
	clientStd = &stdtls.Config{
		RootCAs:    pool,
		ServerName: "localhost",
	}
	return
}

type testSetup struct {
	service *Service
	client  *Client
}

func newTestSetup(t *testing.T) *testSetup {
	t.Helper()

	serverStd, clientStd := generateTestTLSPair(t)
	return newTestSetupWith(t, &testServerTLSConfig{config: serverStd}, &testClientTLSConfig{config: clientStd}, new(N.DefaultDialer))
}

func newTestSetupWithTLS(t *testing.T, serverTLS tls.ServerConfig, clientTLS tls.Config) *testSetup {
	t.Helper()
	return newTestSetupWith(t, serverTLS, clientTLS, new(N.DefaultDialer))
}

func newTestSetupWith(t *testing.T, serverTLS tls.ServerConfig, clientTLS tls.Config, detour N.Dialer) *testSetup {
	t.Helper()

	listener, err := net.Listen(N.NetworkTCP, "127.0.0.1:0")
	require.NoError(t, err)

	service := NewService(ServiceOptions{
		Ctx:     t.Context(),
		Logger:  logger.NOP(),
		Handler: &echoHandler{},
	})
	service.UpdateUsers([]auth.User{{Username: "test", Password: "test"}})
	require.NoError(t, service.Start(listener, nil, serverTLS))

	addr := listener.Addr().String()
	client, err := NewClient(ClientOptions{
		Ctx:       t.Context(),
		Detour:    detour,
		Server:    M.ParseSocksaddr(addr),
		Auth:      auth.User{Username: "test", Password: "test"},
		TLSConfig: clientTLS,
	})
	require.NoError(t, err)
	require.NoError(t, client.Start())

	t.Cleanup(func() {
		client.Close()
		service.Close()
	})

	return &testSetup{service: service, client: client}
}

func TestRoundtripHealthCheck(t *testing.T) {
	t.Parallel()
	s := newTestSetup(t)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	require.NoError(t, s.client.HealthCheck(ctx))
}

func TestRoundtripFakeTLS(t *testing.T) {
	t.Parallel()

	s := newTestSetupWithTLS(t, &fakeTLSConfig{}, &fakeTLSConfig{})

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	require.NoError(t, s.client.HealthCheck(ctx))

	tcpConn, err := s.client.Dial(ctx, M.ParseSocksaddr("example.com:80"))
	require.NoError(t, err)
	defer tcpConn.Close()
	tcpPayload := []byte("hello fake tls tcp")
	_, err = tcpConn.Write(tcpPayload)
	require.NoError(t, err)
	tcpResponse := make([]byte, len(tcpPayload))
	_, err = io.ReadFull(tcpConn, tcpResponse)
	require.NoError(t, err)
	require.Equal(t, tcpPayload, tcpResponse)

	packetConn, err := s.client.ListenPacket(ctx)
	require.NoError(t, err)
	defer packetConn.Close()
	udpPayload := []byte("hello fake tls udp")
	_, err = packetConn.WriteTo(udpPayload, &net.UDPAddr{IP: net.ParseIP("1.2.3.4"), Port: 53})
	require.NoError(t, err)
	udpResponse := make([]byte, len(udpPayload))
	n, source, err := packetConn.ReadFrom(udpResponse)
	require.NoError(t, err)
	require.Equal(t, udpPayload, udpResponse[:n])
	require.Equal(t, "1.2.3.4:53", source.String())
}

func TestRoundtripTCP(t *testing.T) {
	t.Parallel()
	s := newTestSetup(t)

	conn, err := s.client.Dial(t.Context(), M.ParseSocksaddr("example.com:80"))
	require.NoError(t, err)
	defer conn.Close()

	msg := []byte("hello trusttunnel tcp")
	_, err = conn.Write(msg)
	require.NoError(t, err)

	got := make([]byte, len(msg))
	_, err = io.ReadFull(conn, got)
	require.NoError(t, err)
	require.Equal(t, msg, got)
}

func TestRoundtripUDP(t *testing.T) {
	t.Parallel()
	s := newTestSetup(t)

	conn, err := s.client.ListenPacket(t.Context())
	require.NoError(t, err)
	defer conn.Close()

	dest := &net.UDPAddr{IP: net.ParseIP("1.2.3.4"), Port: 53}
	payload := []byte("hello trusttunnel udp")

	_, err = conn.WriteTo(payload, dest)
	require.NoError(t, err)

	got := buf.Get(1500)
	defer buf.Put(got)
	n, src, err := conn.ReadFrom(got)
	require.NoError(t, err)
	require.Equal(t, payload, got[:n])
	require.Equal(t, "1.2.3.4:53", src.String())
}

// TestRoundtripTCPConcurrent opens many TCP streams simultaneously to catch data races.
// Run with -race to enable the race detector.
func TestRoundtripTCPConcurrent(t *testing.T) {
	t.Parallel()
	s := newTestSetup(t)

	const numStreams = 20
	var waitGroup sync.WaitGroup
	for range numStreams {
		waitGroup.Go(func() {
			conn, err := s.client.Dial(t.Context(), M.ParseSocksaddr("example.com:80"))
			if !assert.NoError(t, err) {
				return
			}
			defer conn.Close()

			msg := []byte("concurrent tcp echo")
			if _, err = conn.Write(msg); !assert.NoError(t, err) {
				return
			}
			got := buf.Get(len(msg))
			defer buf.Put(got)
			if _, err = io.ReadFull(conn, got); !assert.NoError(t, err) {
				return
			}
			assert.Equal(t, msg, got)
		})
	}
	waitGroup.Wait()
}

// TestRoundtripUDPConcurrent opens many UDP packet conns simultaneously to catch data races.
// Run with -race to enable the race detector.
func TestRoundtripUDPConcurrent(t *testing.T) {
	t.Parallel()
	s := newTestSetup(t)

	const numConns = 20
	dest := &net.UDPAddr{IP: net.ParseIP("1.2.3.4"), Port: 53}
	payload := []byte("concurrent udp echo")

	var waitGroup sync.WaitGroup
	for range numConns {
		waitGroup.Go(func() {
			pktConn, err := s.client.ListenPacket(t.Context())
			if !assert.NoError(t, err) {
				return
			}
			defer pktConn.Close()

			if _, err = pktConn.WriteTo(payload, dest); !assert.NoError(t, err) {
				return
			}
			got := buf.Get(1500)
			defer buf.Put(got)
			n, _, err := pktConn.ReadFrom(got)
			if !assert.NoError(t, err) {
				return
			}
			assert.Equal(t, payload, got[:n])
		})
	}
	waitGroup.Wait()
}
