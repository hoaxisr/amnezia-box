package option

import (
	"encoding/json"
	"testing"
)

func TestAwgKeepaliveUnmarshal(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want AwgKeepalive
	}{
		{`25`, "25"},         // pre-3.0 configs carry a number
		{`"25"`, "25"},       // and a string is accepted too
		{`"22-30"`, "22-30"}, // AWG 3.0 range
		{`"0-80"`, "0-80"},   // range starting at zero
	} {
		var k AwgKeepalive
		if err := json.Unmarshal([]byte(tc.raw), &k); err != nil {
			t.Fatalf("%s: %v", tc.raw, err)
		}
		if k != tc.want {
			t.Fatalf("%s: got %q, want %q", tc.raw, k, tc.want)
		}
	}

	for _, raw := range []string{`"abc"`, `"22-"`, `"30-22"`, `"-5"`, `{}`} {
		var k AwgKeepalive
		if err := json.Unmarshal([]byte(raw), &k); err == nil {
			t.Fatalf("%s should be rejected, got %q", raw, k)
		}
	}
}

func TestAwgKeepaliveRoundTrip(t *testing.T) {
	var peer AwgPeerOptions
	if err := json.Unmarshal([]byte(`{"persistent_keepalive_interval":25}`), &peer); err != nil {
		t.Fatal(err)
	}
	out, err := json.Marshal(peer)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != `{"persistent_keepalive_interval":"25"}` {
		t.Fatalf("got %s", out)
	}
}
