package awg

import (
	"encoding/base64"
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
