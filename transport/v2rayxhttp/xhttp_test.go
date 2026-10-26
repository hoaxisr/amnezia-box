package xhttp

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"sync"
	"testing"

	"github.com/sagernet/sing-box/common/xray/buf"
	"github.com/sagernet/sing-box/common/xray/signal/done"
	"github.com/sagernet/sing-box/option"
)

// packet-up на HTTP/2: после GOAWAY транспорт переигрывает запрос и берёт тело
// из GetBody. Без него повтор уходил с пустым телом, а сервер получал дыру в
// последовательности (Xray #6632).
func TestFillPacketRequestBodyIsReplayable(t *testing.T) {
	payload := buf.MergeBytes(nil, []byte("payload"))
	req, err := http.NewRequest("POST", "https://example.com/", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = FillPacketRequest(req, "session", "0", payload, &option.V2RayXHTTPBaseOptions{}); err != nil {
		t.Fatal(err)
	}
	first, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != "payload" {
		t.Fatalf("тело запроса: got %q, want %q", first, "payload")
	}
	if req.GetBody == nil {
		t.Fatal("GetBody не задан: повтор запроса уйдёт с пустым телом")
	}
	replay, err := req.GetBody()
	if err != nil {
		t.Fatal(err)
	}
	second, err := io.ReadAll(replay)
	if err != nil {
		t.Fatal(err)
	}
	if string(second) != "payload" {
		t.Fatalf("повтор тела: got %q, want %q", second, "payload")
	}
}

type countingCloser struct {
	io.Reader
	closed int
}

func (c *countingCloser) Close() error {
	c.closed++
	return nil
}

// Close может прийти раньше, чем горутина запроса позовёт Set. Тело, приехавшее
// после закрытия, обязано быть закрыто, иначе соединение течёт.
func TestWaitReadCloserSetAfterCloseClosesBody(t *testing.T) {
	w := &WaitReadCloser{wait: done.New()}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	body := &countingCloser{Reader: nil}
	w.Set(body)
	if body.closed != 1 {
		t.Fatalf("тело закрыто %d раз, want 1", body.closed)
	}
	if _, err := w.Read(make([]byte, 1)); err != io.ErrClosedPipe {
		t.Fatalf("Read после Close: got %v, want %v", err, io.ErrClosedPipe)
	}
}

// Упавший запрос не рвёт соединения клиента: на нём могут жить соседние
// сессии xmux. Клиент только перестаёт выдаваться (IsClosed), а соединения
// освобождает Close при вытеснении (F730, как Xray closed.Store(true)).
func TestFailedRequestMarksClientWithoutClosingIt(t *testing.T) {
	c := &DefaultDialerClient{
		options: &option.V2RayXHTTPBaseOptions{},
		client: &http.Client{Transport: &http.Transport{
			DialContext: func(context.Context, string, string) (net.Conn, error) {
				return nil, errors.New("dial refused")
			},
		}},
		httpVersion:   "2",
		uploadRawPool: &sync.Pool{},
	}
	if err := c.PostPacket(context.Background(), "http://example.com/", "session", "0", buf.MergeBytes(nil, []byte("x"))); err == nil {
		t.Fatal("PostPacket: want error")
	}
	if !c.IsClosed() {
		t.Fatal("после упавшего запроса клиент всё ещё выдаётся xmux")
	}
	if c.closed {
		t.Fatal("упавший запрос закрыл соединения клиента — соседние сессии оборваны")
	}
	c.Close()
	if !c.closed {
		t.Fatal("Close после упавшего запроса не освободил соединения")
	}
}
