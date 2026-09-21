package trusttunnel

import (
	"net/netip"
	"testing"

	"github.com/sagernet/sing/common/auth"
	sHttp "github.com/sagernet/sing/protocol/http"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse16BytesIP(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name     string
		buf      [16]byte
		wantAddr netip.Addr
		wantIs4  bool
	}{
		{
			name:     "IPv4",
			buf:      [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 2, 3, 4},
			wantAddr: netip.MustParseAddr("1.2.3.4"),
			wantIs4:  true,
		},
		{
			name:     "IPv6",
			buf:      netip.MustParseAddr("2001:db8::1").As16(),
			wantAddr: netip.MustParseAddr("2001:db8::1"),
			wantIs4:  false,
		},
		{
			// ::1 shares the same 16-byte layout as 0.0.0.1; must not be misread as IPv4
			name:     "loopback ::1",
			buf:      [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1},
			wantAddr: netip.MustParseAddr("::1"),
			wantIs4:  false,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			addr := parse16BytesIP(tc.buf)
			assert.Equal(t, tc.wantAddr, addr)
			assert.Equal(t, tc.wantIs4, addr.Is4())
		})
	}
}

func TestBuildPaddingIP(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name string
		addr netip.Addr
	}{
		{name: "IPv4", addr: netip.MustParseAddr("192.168.1.100")},
		{name: "IPv6", addr: netip.MustParseAddr("2001:db8::1")},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			buf := buildPaddingIP(tc.addr)
			if tc.addr.Is4() {
				for i := range 12 {
					assert.Zero(t, buf[i], "byte %d should be zero for IPv4 padding", i)
				}
				ipv4 := tc.addr.As4()
				assert.Equal(t, ipv4[:], buf[12:])
			} else {
				assert.Equal(t, tc.addr.As16(), buf)
			}
		})
	}
}

func TestBuildAuth(t *testing.T) {
	t.Parallel()
	user := auth.User{Username: "alice", Password: "s3cr3t"}
	username, password, ok := sHttp.ParseBasicAuth(buildAuth(user))
	require.True(t, ok)
	assert.Equal(t, user.Username, username)
	assert.Equal(t, user.Password, password)
}
