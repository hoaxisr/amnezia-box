package v2rayhttpupgrade

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/sagernet/sing-box/option"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

// Сервер принял соединение и молчит: DialContext обязан вернуться по сроку
// и закрыть соединение, а не висеть с открытым дескриптором.
func TestDialContextSilentServer(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	closed := make(chan struct{})
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		buffer := make([]byte, 4096)
		for {
			_, err = conn.Read(buffer)
			if err != nil {
				close(closed)
				return
			}
		}
	}()
	client, err := NewClient(context.Background(), N.SystemDialer, M.SocksaddrFromNet(listener.Addr()), option.V2RayHTTPUpgradeOptions{Path: "/"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		conn, err := client.DialContext(ctx)
		if err == nil {
			conn.Close()
		}
		done <- err
	}()
	select {
	case err = <-done:
		if err == nil {
			t.Fatal("expected error from silent server")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("DialContext hangs on silent server")
	}
	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("connection not closed after handshake timeout")
	}
}
