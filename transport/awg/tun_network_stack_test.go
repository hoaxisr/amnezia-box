package awg

import (
	"context"
	"encoding/binary"
	"net"
	"net/netip"
	"testing"
	"time"

	tun "github.com/sagernet/sing-tun"
	"github.com/sagernet/sing/common/logger"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

// recordingHandler accepts every flow and reports the destinations it was
// handed, so a test can assert that the stack actually forwarded inbound
// traffic (the gateway/exit role).
type recordingHandler struct {
	udp chan M.Socksaddr
	tcp chan M.Socksaddr
}

func (h *recordingHandler) JudgeFlow(network uint8, source netip.AddrPort, destination netip.AddrPort, firstPacket []byte) tun.FlowVerdict {
	return tun.FlowVerdict{Action: tun.ActionAccept}
}

func (h *recordingHandler) NewDNSPacket(payload []byte, source M.Socksaddr, destination M.Socksaddr, writer N.PacketWriter) {
}

func (h *recordingHandler) NewConnectionEx(ctx context.Context, conn net.Conn, source M.Socksaddr, destination M.Socksaddr, onClose N.CloseHandlerFunc) {
	select {
	case h.tcp <- destination:
	default:
	}
	conn.Close()
}

func (h *recordingHandler) NewPacketConnectionEx(ctx context.Context, conn N.PacketConn, source M.Socksaddr, destination M.Socksaddr, onClose N.CloseHandlerFunc) {
	select {
	case h.udp <- destination:
	default:
	}
	conn.Close()
}

var _ tun.Handler = (*recordingHandler)(nil)

// onesComplement is the internet checksum of data, with an optional carry-in
// from a pseudo header.
func onesComplement(data []byte, initial uint32) uint16 {
	sum := initial
	for i := 0; i+1 < len(data); i += 2 {
		sum += uint32(binary.BigEndian.Uint16(data[i : i+2]))
	}
	if len(data)%2 == 1 {
		sum += uint32(data[len(data)-1]) << 8
	}
	for sum>>16 != 0 {
		sum = sum&0xffff + sum>>16
	}
	return ^uint16(sum)
}

func pseudoHeaderSum(source, destination netip.Addr, protocol byte, length int) uint32 {
	var sum uint32
	src, dst := source.As4(), destination.As4()
	for _, pair := range [][2]byte{{src[0], src[1]}, {src[2], src[3]}, {dst[0], dst[1]}, {dst[2], dst[3]}} {
		sum += uint32(binary.BigEndian.Uint16(pair[:]))
	}
	return sum + uint32(protocol) + uint32(length)
}

// ipv4Packet builds a checksummed IPv4 packet with the given transport payload.
// The Go stack validates checksums on packets written into a memory tun, so a
// hand-built packet must carry correct ones or it is silently dropped.
func ipv4Packet(source, destination netip.Addr, protocol byte, transport []byte) []byte {
	packet := make([]byte, 20+len(transport))
	packet[0] = 0x45
	binary.BigEndian.PutUint16(packet[2:4], uint16(len(packet)))
	packet[8] = 64
	packet[9] = protocol
	copy(packet[12:16], source.AsSlice())
	copy(packet[16:20], destination.AsSlice())
	binary.BigEndian.PutUint16(packet[10:12], onesComplement(packet[:20], 0))
	copy(packet[20:], transport)
	return packet
}

func udpPacketV4(source, destination netip.AddrPort, payload []byte) []byte {
	udp := make([]byte, 8+len(payload))
	binary.BigEndian.PutUint16(udp[0:2], source.Port())
	binary.BigEndian.PutUint16(udp[2:4], destination.Port())
	binary.BigEndian.PutUint16(udp[4:6], uint16(len(udp)))
	copy(udp[8:], payload)
	sum := pseudoHeaderSum(source.Addr(), destination.Addr(), 17, len(udp))
	binary.BigEndian.PutUint16(udp[6:8], onesComplement(udp, sum))
	return ipv4Packet(source.Addr(), destination.Addr(), 17, udp)
}

func tcpSynV4(source, destination netip.AddrPort) []byte {
	tcp := make([]byte, 20)
	binary.BigEndian.PutUint16(tcp[0:2], source.Port())
	binary.BigEndian.PutUint16(tcp[2:4], destination.Port())
	binary.BigEndian.PutUint32(tcp[4:8], 1)
	tcp[12] = 5 << 4 // data offset, no options
	tcp[13] = 0x02   // SYN
	binary.BigEndian.PutUint16(tcp[14:16], 65535)
	sum := pseudoHeaderSum(source.Addr(), destination.Addr(), 6, len(tcp))
	binary.BigEndian.PutUint16(tcp[16:18], onesComplement(tcp, sum))
	return ipv4Packet(source.Addr(), destination.Addr(), 6, tcp)
}

func newTestTun(t *testing.T, handler tun.Handler) tunAdapter {
	t.Helper()
	tunAdapter, err := newNonIntegratedTun(
		context.Background(),
		[]netip.Prefix{netip.MustParsePrefix("10.80.0.1/24")},
		1280,
		handler,
		30*time.Second,
		nil,
		logger.NOP(),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { tunAdapter.Close() })
	if err = tunAdapter.Start(); err != nil {
		t.Fatal(err)
	}
	return tunAdapter
}

// TestStackTunForwardsInbound guards the gateway/exit role: a packet arriving
// from the tunnel towards an arbitrary destination must reach the Handler. A
// stack that only dials outbound passes every client test while traffic
// entering the tunnel disappears without a trace.
func TestStackTunForwardsInbound(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name        string
		destination netip.AddrPort
		packet      func(source, destination netip.AddrPort) []byte
		got         func(h *recordingHandler) chan M.Socksaddr
	}{
		{
			name:        "udp",
			destination: netip.MustParseAddrPort("1.1.1.1:123"),
			packet:      func(s, d netip.AddrPort) []byte { return udpPacketV4(s, d, []byte("ping")) },
			got:         func(h *recordingHandler) chan M.Socksaddr { return h.udp },
		},
		{
			name:        "tcp",
			destination: netip.MustParseAddrPort("1.1.1.1:80"),
			packet:      tcpSynV4,
			got:         func(h *recordingHandler) chan M.Socksaddr { return h.tcp },
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			handler := &recordingHandler{
				udp: make(chan M.Socksaddr, 1),
				tcp: make(chan M.Socksaddr, 1),
			}
			tunAdapter := newTestTun(t, handler)
			packet := test.packet(netip.MustParseAddrPort("10.80.0.2:41234"), test.destination)
			if _, err := tunAdapter.Write([][]byte{packet}, 0); err != nil {
				t.Fatal(err)
			}
			select {
			case got := <-test.got(handler):
				if got.AddrPort() != test.destination {
					t.Fatalf("forwarded to %s, want %s", got, test.destination)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("inbound packet never reached the handler")
			}
		})
	}
}

// TestStackTunDialsOutbound guards the client role: a dial through the stack
// must emit a packet towards the tunnel, sourced from the local tunnel address.
func TestStackTunDialsOutbound(t *testing.T) {
	t.Parallel()
	tunAdapter := newTestTun(t, nil)

	go func() {
		conn, err := tunAdapter.DialContext(context.Background(), N.NetworkTCP, M.ParseSocksaddr("1.1.1.1:80"))
		if err == nil {
			conn.Close()
		}
	}()

	bufs := [][]byte{make([]byte, 2048)}
	sizes := make([]int, 1)
	done := make(chan []byte, 1)
	go func() {
		count, err := tunAdapter.Read(bufs, sizes, 0)
		if err != nil || count == 0 {
			return
		}
		done <- bufs[0][:sizes[0]]
	}()

	select {
	case packet := <-done:
		source, _ := netip.AddrFromSlice(packet[12:16])
		if source != netip.MustParseAddr("10.80.0.1") {
			t.Fatalf("outbound packet sourced from %s, want the tunnel address", source)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("dial produced no outbound packet")
	}
}

// newUnstartedTun builds a stack tun with no handler, for tests that only need
// a real tun underneath the AWG device.
func newUnstartedTun(address []netip.Prefix, mtu uint32) (tunAdapter, error) {
	return newNonIntegratedTun(context.Background(), address, mtu, nil, 0, nil, logger.NOP())
}
