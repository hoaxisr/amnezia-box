package sniff_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"net"
	"testing"
	"time"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/common/sniff"
	C "github.com/sagernet/sing-box/constant"

	"github.com/stretchr/testify/require"
)

// tlsClientHello снимает настоящий ClientHello с заданной группой обмена ключами.
func tlsClientHello(t testing.TB, curve tls.CurveID) []byte {
	t.Helper()
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	go tls.Client(client, &tls.Config{
		ServerName:         "example.com",
		InsecureSkipVerify: true,
		CurvePreferences:   []tls.CurveID{curve},
	}).Handshake()
	require.NoError(t, server.SetReadDeadline(time.Now().Add(5*time.Second)))
	buffer := make([]byte, 4096)
	n, err := server.Read(buffer)
	require.NoError(t, err)
	return buffer[:n]
}

func TestSniffTLSClientHello(t *testing.T) {
	t.Parallel()
	for _, curve := range []tls.CurveID{tls.X25519, tls.X25519MLKEM768} {
		var metadata adapter.InboundContext
		err := sniff.TLSClientHello(context.Background(), &metadata, bytes.NewReader(tlsClientHello(t, curve)))
		require.NoError(t, err)
		require.Equal(t, C.ProtocolTLS, metadata.Protocol)
		require.Equal(t, "example.com", metadata.Domain)
	}
}

func TestSniffTLSClientHelloTruncated(t *testing.T) {
	t.Parallel()
	hello := tlsClientHello(t, tls.X25519)
	var metadata adapter.InboundContext
	err := sniff.TLSClientHello(context.Background(), &metadata, bytes.NewReader(hello[:len(hello)/2]))
	require.ErrorIs(t, err, sniff.ErrNeedMoreData)
}

// Сниффу нужен только SNI. Обмен ключами виден по аллокациям: на Go 1.26 разбор
// ClientHello даёт 39, а с ML-KEM и X25519 их 65.
const maxSniffTLSAllocs = 50

func TestSniffTLSClientHelloSkipsKeyExchange(t *testing.T) {
	hello := tlsClientHello(t, tls.X25519MLKEM768)
	allocs := testing.AllocsPerRun(20, func() {
		var metadata adapter.InboundContext
		_ = sniff.TLSClientHello(context.Background(), &metadata, bytes.NewReader(hello))
	})
	t.Logf("allocs per ClientHello: %.0f", allocs)
	require.Less(t, allocs, float64(maxSniffTLSAllocs))
}

func BenchmarkSniffTLSClientHello(b *testing.B) {
	hello := tlsClientHello(b, tls.X25519MLKEM768)
	b.ReportAllocs()
	for b.Loop() {
		var metadata adapter.InboundContext
		_ = sniff.TLSClientHello(context.Background(), &metadata, bytes.NewReader(hello))
	}
}
