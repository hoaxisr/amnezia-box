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
