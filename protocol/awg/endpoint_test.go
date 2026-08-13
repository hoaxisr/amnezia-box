package awg

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/sagernet/sing-box/option"
)

func TestGenIpcConfigMissingPrivateKey(t *testing.T) {
	_, err := genIpcConfig(option.AwgEndpointOptions{}, nil)
	if err == nil {
		t.Fatal("expected an error for an empty private key, got nil")
	}
}

func TestGenIpcConfigValidPrivateKey(t *testing.T) {
	privateKey := base64.StdEncoding.EncodeToString(make([]byte, 32))
	ipcConfig, err := genIpcConfig(option.AwgEndpointOptions{PrivateKey: privateKey}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(ipcConfig, "private_key=") {
		t.Fatalf("expected ipc config to contain private_key=, got: %q", ipcConfig)
	}
}

func TestGenIpcConfigEndpointHostPort(t *testing.T) {
	key := base64.StdEncoding.EncodeToString(make([]byte, 32))
	cases := map[string]struct {
		addr string
		want string
	}{
		"ipv4":         {"1.2.3.4", "endpoint=1.2.3.4:51820"},
		"ipv6-literal": {"2001:db8::1", "endpoint=[2001:db8::1]:51820"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cfg, err := genIpcConfig(option.AwgEndpointOptions{
				PrivateKey: key,
				Peers: []option.AwgPeerOptions{{
					PublicKey: key,
					Address:   tc.addr,
					Port:      51820,
				}},
			}, nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !strings.Contains(cfg, tc.want) {
				t.Fatalf("expected ipc config to contain %q, got: %q", tc.want, cfg)
			}
		})
	}
}

func TestGenIpcConfigAwg31Flags(t *testing.T) {
	key := base64.StdEncoding.EncodeToString(make([]byte, 32))
	cases := map[string]struct {
		opts    option.AwgEndpointOptions
		want    []string
		notWant []string
	}{
		"both off": {
			opts:    option.AwgEndpointOptions{PrivateKey: key},
			notWant: []string{"random_trailers=", "disable_cookies="},
		},
		"trailers on": {
			opts:    option.AwgEndpointOptions{PrivateKey: key, RandomTrailers: true},
			want:    []string{"random_trailers=true"},
			notWant: []string{"disable_cookies="},
		},
		"cookies disabled": {
			opts:    option.AwgEndpointOptions{PrivateKey: key, DisableCookies: true},
			want:    []string{"disable_cookies=true"},
			notWant: []string{"random_trailers="},
		},
		"both on": {
			opts: option.AwgEndpointOptions{PrivateKey: key, RandomTrailers: true, DisableCookies: true},
			want: []string{"random_trailers=true", "disable_cookies=true"},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cfg, err := genIpcConfig(tc.opts, nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			for _, want := range tc.want {
				if !strings.Contains(cfg, want) {
					t.Fatalf("expected ipc config to contain %q, got: %q", want, cfg)
				}
			}
			for _, notWant := range tc.notWant {
				if strings.Contains(cfg, notWant) {
					t.Fatalf("expected ipc config not to contain %q, got: %q", notWant, cfg)
				}
			}
		})
	}
}

// TestGenIpcConfigAwg31FlagsPrecedePeers keeps the two flags device-scoped in
// the UAPI stream. Everything after the first public_key= line is parsed as a
// peer field, so a flag emitted below the peers would be rejected outright.
func TestGenIpcConfigAwg31FlagsPrecedePeers(t *testing.T) {
	key := base64.StdEncoding.EncodeToString(make([]byte, 32))
	cfg, err := genIpcConfig(option.AwgEndpointOptions{
		PrivateKey:     key,
		RandomTrailers: true,
		DisableCookies: true,
		Peers:          []option.AwgPeerOptions{{PublicKey: key}},
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	firstPeer := strings.Index(cfg, "public_key=")
	if firstPeer < 0 {
		t.Fatalf("expected a peer in the ipc config, got: %q", cfg)
	}
	for _, flag := range []string{"random_trailers=", "disable_cookies="} {
		at := strings.Index(cfg, flag)
		if at < 0 {
			t.Fatalf("expected ipc config to contain %q, got: %q", flag, cfg)
		}
		if at > firstPeer {
			t.Fatalf("%q must precede the first public_key=, got: %q", flag, cfg)
		}
	}
}

// TestAwgEndpointOptionsAwg31FlagsJSON pins the wire names of the two flags and
// their absence when off: a renamed key would be silently dropped by the config
// parser instead of failing, and an emitted false would be a no-op line that
// still churns the applied-config hash on every rebuild.
func TestAwgEndpointOptionsAwg31FlagsJSON(t *testing.T) {
	var opts option.AwgEndpointOptions
	if err := json.Unmarshal([]byte(`{"private_key":"x","random_trailers":true,"disable_cookies":true}`), &opts); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !opts.RandomTrailers {
		t.Fatal("random_trailers did not unmarshal into RandomTrailers")
	}
	if !opts.DisableCookies {
		t.Fatal("disable_cookies did not unmarshal into DisableCookies")
	}

	encoded, err := json.Marshal(option.AwgEndpointOptions{PrivateKey: "x"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, key := range []string{"random_trailers", "disable_cookies"} {
		if strings.Contains(string(encoded), key) {
			t.Fatalf("expected %q to be omitted when off, got: %s", key, encoded)
		}
	}
}
