package finalmask

import (
	"bytes"
	"crypto/rand"
	"net"
	"testing"

	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common/json"
)

type recordConn struct {
	net.Conn
	writes [][]byte
}

func (c *recordConn) Write(p []byte) (int, error) {
	c.writes = append(c.writes, append([]byte(nil), p...))
	return len(p), nil
}

func mask(t *testing.T, settings string) *fragmentConn {
	t.Helper()
	var options option.FinalMaskOptions
	err := json.Unmarshal([]byte(`{"tcp":[{"type":"fragment","settings":`+settings+`}]}`), &options)
	if err != nil {
		t.Fatal(err)
	}
	config, err := newFragmentConfig(options.TCP[0].Fragment)
	if err != nil {
		t.Fatal(err)
	}
	return &fragmentConn{Conn: &recordConn{}, config: config}
}

// helloRecord — handshake-запись TLS 1.0 с телом n байт, плюс хвост.
func helloRecord(n int, tail int) []byte {
	body := make([]byte, n+tail)
	rand.Read(body)
	return append([]byte{22, 3, 1, byte(n >> 8), byte(n)}, body...)
}

// records разбирает поток на TLS-записи и возвращает длины тел и склеенные тела.
func records(t *testing.T, stream []byte) ([]int, []byte) {
	t.Helper()
	var lengths []int
	var payload []byte
	for len(stream) > 0 {
		if len(stream) < 5 || stream[0] != 22 || stream[1] != 3 || stream[2] != 1 {
			t.Fatalf("bad record header: % x", stream[:min(5, len(stream))])
		}
		l := int(stream[3])<<8 | int(stream[4])
		lengths = append(lengths, l)
		payload = append(payload, stream[5:5+l]...)
		stream = stream[5+l:]
	}
	return lengths, payload
}

func TestTLSHelloMergedWithoutDelay(t *testing.T) {
	c := mask(t, `{"packets":"tlshello","length":"100-200"}`)
	hello := helloRecord(1700, 0)
	n, err := c.Write(hello)
	if err != nil || n != len(hello) {
		t.Fatal(n, err)
	}
	writes := c.Conn.(*recordConn).writes
	if len(writes) != 1 {
		t.Fatalf("delay=0 must merge records into one write, got %d", len(writes))
	}
	lengths, payload := records(t, writes[0])
	if !bytes.Equal(payload, hello[5:]) {
		t.Fatal("payload changed")
	}
	if len(lengths) < 9 {
		t.Fatalf("1700 bytes by 100-200 must give ≥9 records, got %d", len(lengths))
	}
	for _, l := range lengths[:len(lengths)-1] {
		if l < 100 || l >= 200 {
			t.Fatalf("record length %d out of 100-200", l)
		}
	}
}

// Маска из #1168: каждая запись — отдельный Write, кусков не больше maxSplit.
func TestTLSHelloIssue1168Mask(t *testing.T) {
	c := mask(t, `{"delay":"1-3","length":"100-200","packets":"tlshello","maxSplit":"3-5"}`)
	hello := helloRecord(1700, 0)
	if _, err := c.Write(hello); err != nil {
		t.Fatal(err)
	}
	writes := c.Conn.(*recordConn).writes
	if len(writes) < 3 || len(writes) > 4 {
		t.Fatalf("maxSplit 3-5 must give 3..4 writes, got %d", len(writes))
	}
	var stream []byte
	for _, w := range writes {
		lengths, _ := records(t, w)
		if len(lengths) != 1 {
			t.Fatalf("each write must be one record, got %d", len(lengths))
		}
		stream = append(stream, w...)
	}
	if _, payload := records(t, stream); !bytes.Equal(payload, hello[5:]) {
		t.Fatal("payload changed")
	}
}

func TestTLSHelloTailAndLaterWritesUntouched(t *testing.T) {
	c := mask(t, `{"packets":"tlshello","length":"50-60"}`)
	hello := helloRecord(300, 40)
	if n, err := c.Write(hello); err != nil || n != len(hello) {
		t.Fatal(n, err)
	}
	second := []byte("application data")
	c.Write(second)
	writes := c.Conn.(*recordConn).writes
	if len(writes) != 3 {
		t.Fatalf("want merged hello + tail + second write, got %d", len(writes))
	}
	if !bytes.Equal(writes[1], hello[305:]) || !bytes.Equal(writes[2], second) {
		t.Fatal("tail or later write changed")
	}
}

func TestTLSHelloSkipsNonHandshake(t *testing.T) {
	c := mask(t, `{"packets":"tlshello","length":"10"}`)
	data := []byte{23, 3, 3, 0, 4, 1, 2, 3, 4}
	c.Write(data)
	if writes := c.Conn.(*recordConn).writes; len(writes) != 1 || !bytes.Equal(writes[0], data) {
		t.Fatal("non-handshake first write must pass through")
	}
}

func TestPacketsRangeSplitsRawWrites(t *testing.T) {
	c := mask(t, `{"packets":"2-2","length":"10"}`)
	first, second, third := make([]byte, 25), make([]byte, 25), make([]byte, 25)
	c.Write(first)
	c.Write(second)
	c.Write(third)
	var sizes []int
	for _, w := range c.Conn.(*recordConn).writes {
		sizes = append(sizes, len(w))
	}
	want := []int{25, 10, 10, 5, 25}
	if len(sizes) != len(want) {
		t.Fatalf("sizes %v, want %v", sizes, want)
	}
	for i := range want {
		if sizes[i] != want[i] {
			t.Fatalf("sizes %v, want %v", sizes, want)
		}
	}
}

func TestOptionsRejectUnsupported(t *testing.T) {
	for name, raw := range map[string]string{
		"udp mask":      `{"udp":[{"type":"salamander","settings":{"password":"x"}}]}`,
		"unknown tcp":   `{"tcp":[{"type":"sudoku","settings":{"password":"x"}}]}`,
		"zero length":   `{"tcp":[{"type":"fragment","settings":{"packets":"tlshello"}}]}`,
		"zero packets":  `{"tcp":[{"type":"fragment","settings":{"packets":"0-3","length":"10"}}]}`,
		"unknown field": `{"tcp":[],"bogus":1}`,
	} {
		var options option.FinalMaskOptions
		if err := json.Unmarshal([]byte(raw), &options); err == nil {
			t.Errorf("%s: must be rejected", name)
		}
	}
	var options option.FinalMaskOptions
	err := json.Unmarshal([]byte(`{"tcp":[{"type":"Fragment","settings":{"packets":"tlshello","length":"100-200"}}],"quicParams":{"congestion":"bbr"}}`), &options)
	if err != nil {
		t.Fatal(err)
	}
}
