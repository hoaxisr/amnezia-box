package tf

import (
	"context"
	"crypto/tls"
	"net"
	"testing"
)

type recordWriter struct {
	net.Conn
	data []byte
}

func (w *recordWriter) Write(p []byte) (int, error) {
	w.data = append(w.data, p...)
	return len(p), nil
}

// Каждая метка до публичного суффикса режется своей записью: у
// www.example.co.uk две метки (www, example) — три записи.
func TestRecordFragmentSplitsEveryLabel(t *testing.T) {
	for serverName, want := range map[string]int{
		"www.example.com":   3,
		"a.b.example.co.uk": 4,
		"example.com":       2,
		"github.io":         3, // весь домен — публичный суффикс: режем обе метки
	} {
		client, server := net.Pipe()
		go func() {
			buf := make([]byte, 4096)
			server.Read(buf)
			server.Close()
		}()
		hello := captureClientHello(t, serverName)
		w := &recordWriter{Conn: client}
		conn := NewConn(w, context.Background(), false, true, 0)
		if _, err := conn.Write(hello); err != nil {
			t.Fatal(err)
		}
		records := 0
		for b := w.data; len(b) >= 5; records++ {
			b = b[5+(int(b[3])<<8|int(b[4])):]
		}
		if records != want {
			t.Errorf("%s: %d records, want %d", serverName, records, want)
		}
		client.Close()
	}
}

func captureClientHello(t *testing.T, serverName string) []byte {
	t.Helper()
	client, server := net.Pipe()
	defer client.Close()
	go tls.Client(client, &tls.Config{ServerName: serverName}).Handshake()
	buf := make([]byte, 4096)
	n, err := server.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	server.Close()
	return buf[:n]
}
