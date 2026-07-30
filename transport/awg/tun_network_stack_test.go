//go:build with_gvisor

package awg

import (
	"context"
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/sagernet/gvisor/pkg/tcpip"
	"github.com/sagernet/gvisor/pkg/tcpip/checksum"
	"github.com/sagernet/gvisor/pkg/tcpip/header"
	tun "github.com/sagernet/sing-tun"
	"github.com/sagernet/sing/common/logger"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

// recordingHandler accepts every flow and reports the destinations it was
// handed, so a test can assert that a forwarder actually fired.
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

func udpPacketV4(source, destination netip.AddrPort, payload []byte) []byte {
	totalLength := header.IPv4MinimumSize + header.UDPMinimumSize + len(payload)
	packet := make([]byte, totalLength)

	ip := header.IPv4(packet)
	ip.Encode(&header.IPv4Fields{
		TotalLength: uint16(totalLength),
		TTL:         64,
		Protocol:    uint8(header.UDPProtocolNumber),
		SrcAddr:     tcpip.AddrFrom4(source.Addr().As4()),
		DstAddr:     tcpip.AddrFrom4(destination.Addr().As4()),
	})
	ip.SetChecksum(^ip.CalculateChecksum())

	udp := header.UDP(packet[header.IPv4MinimumSize:])
	udp.Encode(&header.UDPFields{
		SrcPort: source.Port(),
		DstPort: destination.Port(),
		Length:  uint16(header.UDPMinimumSize + len(payload)),
	})
	copy(packet[header.IPv4MinimumSize+header.UDPMinimumSize:], payload)
	xsum := header.PseudoHeaderChecksum(header.UDPProtocolNumber, ip.SourceAddress(), ip.DestinationAddress(), uint16(len(udp)))
	udp.SetChecksum(^checksum.Checksum(payload, xsum))

	return packet
}

// TestStackTunForwardsUDP guards the UDP forwarder lifecycle: sing-tun's UDP NAT
// silently drops every packet until UDPForwarder.Start() is called, so a
// forwarder that is registered but never started makes the endpoint pass TCP and
// ICMP while UDP disappears without a trace.
func TestStackTunForwardsUDP(t *testing.T) {
	handler := &recordingHandler{
		udp: make(chan M.Socksaddr, 1),
		tcp: make(chan M.Socksaddr, 1),
	}

	tunAdapter, err := newNonIntegratedTun(
		context.Background(),
		[]netip.Prefix{netip.MustParsePrefix("10.80.0.1/24")},
		1280,
		handler,
		30*time.Second,
		logger.NOP(),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer tunAdapter.Close()

	if err = tunAdapter.Start(); err != nil {
		t.Fatal(err)
	}

	destination := netip.MustParseAddrPort("1.1.1.1:123")
	packet := udpPacketV4(netip.MustParseAddrPort("10.80.0.2:41234"), destination, []byte("ping"))
	if _, err = tunAdapter.Write([][]byte{packet}, 0); err != nil {
		t.Fatal(err)
	}

	select {
	case got := <-handler.udp:
		if got.AddrPort() != destination {
			t.Fatalf("forwarded to %s, want %s", got, destination)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("inbound UDP packet never reached the handler")
	}
}
