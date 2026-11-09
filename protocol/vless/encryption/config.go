// Package encryption — клиентская часть VLESS Encryption
// (mlkem768x25519plus), порт xray-core proxy/vless/encryption v26.10.31
// (MPL-2.0). Серверная часть не перенесена.
package encryption

import (
	"encoding/base64"
	"errors"
	"runtime"
	"strings"

	"golang.org/x/sys/cpu"
)

// Как xray-core common/protocol: сервер пробует оба AEAD, клиент выбирает свой.
var hasAESGCMHardwareSupport = cpu.X86.HasAES && cpu.X86.HasPCLMULQDQ && cpu.X86.HasSSE41 && cpu.X86.HasSSSE3 ||
	cpu.ARM64.HasAES && cpu.ARM64.HasPMULL || runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" ||
	cpu.S390X.HasAES && cpu.S390X.HasAESCTR && cpu.S390X.HasGHASH ||
	runtime.GOARCH == "ppc64" || runtime.GOARCH == "ppc64le"

// NewClient разбирает строку encryption клиента VLESS в формате Xray
// (infra/conf/vless.go): mlkem768x25519plus.<native|xorpub|random>.<1rtt|0rtt>.[padding.]<ключи>.
// "" и "none" — шифрования нет, возвращается nil.
func NewClient(encryption string) (*ClientInstance, error) {
	if encryption == "" || encryption == "none" {
		return nil, nil
	}
	s := strings.Split(encryption, ".")
	if len(s) < 4 || s[0] != "mlkem768x25519plus" {
		return nil, errors.New("unsupported encryption: " + encryption)
	}
	var xorMode, seconds uint32
	switch s[1] {
	case "native":
	case "xorpub":
		xorMode = 1
	case "random":
		xorMode = 2
	default:
		return nil, errors.New("unknown encryption mode: " + s[1])
	}
	switch s[2] {
	case "1rtt":
	case "0rtt":
		seconds = 1
	default:
		return nil, errors.New("unknown encryption rtt: " + s[2])
	}
	var padding []string
	var keys [][]byte
	for _, r := range s[3:] {
		if len(r) < 20 {
			padding = append(padding, r)
			continue
		}
		b, _ := base64.RawURLEncoding.DecodeString(r)
		if len(b) != 32 && len(b) != 1184 {
			return nil, errors.New("invalid encryption key: " + r)
		}
		keys = append(keys, b)
	}
	client := &ClientInstance{}
	if err := client.Init(keys, xorMode, seconds, strings.Join(padding, ".")); err != nil {
		return nil, err
	}
	return client, nil
}
