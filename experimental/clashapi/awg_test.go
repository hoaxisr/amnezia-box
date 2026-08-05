package clashapi

import (
	"encoding/json"
	"reflect"
	"testing"
)

const (
	peerAHex = "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20"
	peerAB64 = "AQIDBAUGBwgJCgsMDQ4PEBESExQVFhcYGRobHB0eHyA="
	peerBHex = "2122232425262728292a2b2c2d2e2f303132333435363738393a3b3c3d3e3f40"
	peerBB64 = "ISIjJCUmJygpKissLS4vMDEyMzQ1Njc4OTo7PD0+P0A="
)

// TestParseAwgUAPIPeers pins the shape of the WireGuard configuration-protocol
// "get" response this route depends on: peer fields belong to the preceding
// public_key line, device-level lines (including the private key) are never
// reported, and a device with no peers still renders as [].
func TestParseAwgUAPIPeers(t *testing.T) {
	testCases := []struct {
		name string
		uapi string
		want []awgPeerObject
	}{
		{
			name: "one peer among device lines",
			uapi: "private_key=" + peerAHex + "\n" +
				"listen_port=51820\n" +
				"public_key=" + peerBHex + "\n" +
				"preshared_key=" + peerAHex + "\n" +
				"protocol_version=1\n" +
				"endpoint=192.0.2.1:51820\n" +
				"last_handshake_time_sec=1700000000\n" +
				"last_handshake_time_nsec=123\n" +
				"tx_bytes=10\n" +
				"rx_bytes=20\n" +
				"allowed_ip=0.0.0.0/0\n" +
				"errno=0\n",
			want: []awgPeerObject{{PublicKey: peerBB64, LastHandshake: 1700000000, TxBytes: 10, RxBytes: 20}},
		},
		{
			name: "fields attach to their own peer",
			uapi: "public_key=" + peerAHex + "\ntx_bytes=1\nrx_bytes=2\n" +
				"public_key=" + peerBHex + "\ntx_bytes=3\nrx_bytes=4\nlast_handshake_time_sec=5\n",
			want: []awgPeerObject{
				{PublicKey: peerAB64, TxBytes: 1, RxBytes: 2},
				{PublicKey: peerBB64, TxBytes: 3, RxBytes: 4, LastHandshake: 5},
			},
		},
		{
			name: "never handshaked",
			uapi: "public_key=" + peerAHex + "\nlast_handshake_time_sec=0\ntx_bytes=0\nrx_bytes=0\n",
			want: []awgPeerObject{{PublicKey: peerAB64}},
		},
		{
			// A broken key drops its peer; the fields that follow must not be
			// credited to the previous one.
			name: "unparsable public_key drops the peer and its fields",
			uapi: "public_key=" + peerAHex + "\ntx_bytes=1\n" +
				"public_key=zzzz\ntx_bytes=999\nrx_bytes=999\n",
			want: []awgPeerObject{{PublicKey: peerAB64, TxBytes: 1}},
		},
		{
			name: "no peers",
			uapi: "private_key=" + peerAHex + "\nlisten_port=51820\nerrno=0\n",
			want: []awgPeerObject{},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			got := parseAwgUAPIPeers(testCase.uapi)
			if !reflect.DeepEqual(got, testCase.want) {
				t.Fatalf("parseAwgUAPIPeers() = %+v, want %+v", got, testCase.want)
			}
		})
	}
}

// TestParseAwgUAPIPeersEmptyIsArray guards the JSON shape: a nil slice would
// render as null and break callers that iterate the list without a guard.
func TestParseAwgUAPIPeersEmptyIsArray(t *testing.T) {
	encoded, err := json.Marshal(map[string]any{"peers": parseAwgUAPIPeers("errno=0\n")})
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `{"peers":[]}` {
		t.Fatalf("encoded = %s, want {\"peers\":[]}", encoded)
	}
}
