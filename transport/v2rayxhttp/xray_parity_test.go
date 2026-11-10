package xhttp

import (
	"strings"
	"testing"

	"github.com/sagernet/sing-box/option"
)

func TestResolveMode(t *testing.T) {
	for _, tc := range []struct {
		mode              string
		reality, download bool
		want              string
	}{
		{"auto", false, false, "packet-up"},
		{"auto", false, true, "packet-up"},
		{"auto", true, false, "stream-one"},
		{"auto", true, true, "stream-up"},
		{"stream-up", true, false, "stream-up"},
	} {
		if got := resolveMode(tc.mode, tc.reality, tc.download); got != tc.want {
			t.Errorf("%+v: got %s", tc, got)
		}
	}
}

func TestGenerateSessionID(t *testing.T) {
	options := &option.V2RayXHTTPBaseOptions{SessionIDTable: "0123456789"}
	options.SessionIDLength.From, options.SessionIDLength.To = 12, 14
	for range 50 {
		id := generateSessionID(options)
		if len(id) < 12 || len(id) > 14 || strings.Trim(id, "0123456789") != "" {
			t.Fatalf("id %q outside table/length", id)
		}
	}
	if id := generateSessionID(&option.V2RayXHTTPBaseOptions{}); len(id) != 36 {
		t.Fatalf("default must be UUID, got %q", id)
	}
}
