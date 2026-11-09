//go:build with_utls

package tls

import (
	"net"
	"testing"

	utls "github.com/metacubex/utls"
)

// firefox и safari обязаны слать X25519MLKEM768 перед X25519 — иначе сервер
// REALITY Xray v26.9.8+ ClientHello отвергает.
func TestCustomFingerprintsSendMLKEM(t *testing.T) {
	for _, name := range []string{"firefox", "safari", "chrome"} {
		id, err := uTLSClientHelloID(name)
		if err != nil {
			t.Fatal(err)
		}
		client, server := net.Pipe()
		server.Close()
		uConn, err := newUConn(client, &utls.Config{ServerName: "example.com"}, id)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if err := uConn.BuildHandshakeState(); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		var groups []utls.CurveID
		for _, ext := range uConn.Extensions {
			if ks, ok := ext.(*utls.KeyShareExtension); ok {
				for _, share := range ks.KeyShares {
					if share.Group != utls.X25519MLKEM768 && share.Group != utls.X25519 {
						continue
					}
					groups = append(groups, share.Group)
				}
			}
		}
		if len(groups) < 2 || groups[0] != utls.X25519MLKEM768 || groups[1] != utls.X25519 {
			t.Errorf("%s: key shares %v, want X25519MLKEM768 before X25519", name, groups)
		}
		client.Close()
	}
}
