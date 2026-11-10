//go:build badlinkname

package vless

import (
	"net"
	"reflect"
	"unsafe"

	"github.com/sagernet/sing-box/protocol/vless/encryption"
	N "github.com/sagernet/sing/common/network"

	_ "github.com/sagernet/sing-vmess/vless"
)

// Vision в sing-vmess находит соединение по реестру tlsRegistry и через
// reflect берёт у него буферы input/rawInput. У CommonConn они называются и
// устроены так же, как у TLS-соединения (Xray читает их тем же
// vless.AccessTLSBuffers), поэтому достаточно записи в реестре; прямой режим
// пишет в CommonConn.Conn, как UnwrapRawConn у Xray. Реестр неэкспортирован —
// отсюда linkname, как в common/badtls.
//
//go:linkname visionTLSRegistry github.com/sagernet/sing-vmess/vless.tlsRegistry
var visionTLSRegistry []func(conn net.Conn) (loaded bool, netConn net.Conn, reflectType reflect.Type, reflectPointer uintptr)

const visionOverEncryption = true

func init() {
	visionTLSRegistry = append(visionTLSRegistry, func(conn net.Conn) (loaded bool, netConn net.Conn, reflectType reflect.Type, reflectPointer uintptr) {
		commonConn, loaded := N.CastReader[*encryption.CommonConn](conn)
		if !loaded {
			return
		}
		return true, commonConn.Conn, reflect.TypeOf(commonConn).Elem(), uintptr(unsafe.Pointer(commonConn))
	})
}
