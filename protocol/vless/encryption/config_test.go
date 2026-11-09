package encryption

import "testing"

const testKey = "yxMvCENUgclMPYjCWKjgaxLlMHPc1cMgbr5nPa8qnB0"

func TestNewClient(t *testing.T) {
	for _, s := range []string{"", "none"} {
		if c, err := NewClient(s); c != nil || err != nil {
			t.Fatalf("%q: want no encryption, got %v %v", s, c, err)
		}
	}
	c, err := NewClient("mlkem768x25519plus.random.0rtt.100-500-900.50-10-20." + testKey)
	if err != nil {
		t.Fatal(err)
	}
	if c.XorMode != 2 || c.Seconds != 1 || len(c.PaddingLens) != 1 || len(c.PaddingGaps) != 1 {
		t.Fatalf("mode/rtt/padding not parsed: %+v", c)
	}
	for _, bad := range []string{
		"aes-128-gcm",
		"mlkem768x25519plus.bogus.0rtt." + testKey,
		"mlkem768x25519plus.native.2rtt." + testKey,
		"mlkem768x25519plus.native.0rtt.c2hvcnRrZXlzaG9ydGtleXNob3J0a2V5",
		"mlkem768x25519plus.native.0rtt.10-5-5." + testKey,
	} {
		if _, err := NewClient(bad); err == nil {
			t.Errorf("%q: must be rejected", bad)
		}
	}
}
