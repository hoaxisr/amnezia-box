//go:build linux

package awg

import (
	"bytes"
	"errors"
	"net"
	"net/netip"
	"os"
	"testing"
	"time"

	awgTun "github.com/amnezia-vpn/amneziawg-go/v3/tun"

	tun "github.com/sagernet/sing-tun"
	"github.com/sagernet/sing/common/control"
	"github.com/sagernet/sing/common/logger"

	"golang.org/x/sys/unix"
)

func TestSystemTunDeliversPackets(t *testing.T) {
	for _, gso := range []bool{false, true} {
		name := "plain"
		if gso {
			name = "gso"
		}
		t.Run(name, func(t *testing.T) {
			address := netip.MustParseAddr("192.0.2.77")
			interfaceName := tun.CalculateInterfaceName("awgt")
			networkMonitor, err := tun.NewNetworkUpdateMonitor(logger.NOP())
			if err != nil {
				t.Fatal(err)
			}
			defer networkMonitor.Close()
			interfaceMonitor, err := tun.NewDefaultInterfaceMonitor(networkMonitor, logger.NOP(), tun.DefaultInterfaceMonitorOptions{
				InterfaceFinder: control.NewDefaultInterfaceFinder(),
			})
			if err != nil {
				t.Fatal(err)
			}
			defer interfaceMonitor.Close()
			native, err := tun.New(tun.Options{
				Name:             interfaceName,
				MTU:              1408,
				GSO:              gso,
				DNSMode:          tun.DNSModeDisabled,
				Inet4Address:     []netip.Prefix{netip.PrefixFrom(address, 32)},
				Logger:           logger.NOP(),
				InterfaceMonitor: interfaceMonitor,
			})
			if errors.Is(err, unix.EPERM) || errors.Is(err, os.ErrNotExist) {
				t.Skipf("system TUN unavailable: %v", err)
			}
			if err != nil {
				t.Fatal(err)
			}
			adapter := &systemTun{singtun: native, events: make(chan awgTun.Event, 1)}
			t.Cleanup(func() { adapter.Close() })
			if err = adapter.Start(); err != nil {
				t.Fatal(err)
			}
			conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IP(address.AsSlice())})
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			if err = conn.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
				t.Fatal(err)
			}
			payload := []byte("AWG system TUN must preserve the complete IP packet")
			packet := udpPacketV4(netip.MustParseAddrPort("192.0.2.78:12345"), conn.LocalAddr().(*net.UDPAddr).AddrPort(), payload)
			const offset = 16
			buffer := make([]byte, offset+len(packet))
			copy(buffer[offset:], packet)
			if _, err = adapter.Write([][]byte{buffer}, offset); err != nil {
				t.Fatal(err)
			}
			received := make([]byte, 256)
			n, source, err := conn.ReadFromUDPAddrPort(received)
			if err != nil {
				t.Fatal(err)
			}
			if source != netip.MustParseAddrPort("192.0.2.78:12345") || !bytes.Equal(received[:n], payload) {
				t.Fatalf("packet corrupted: source=%v payload=%q", source, received[:n])
			}
			if err = adapter.Close(); err != nil {
				t.Fatal(err)
			}
			if _, err = net.InterfaceByName(interfaceName); err == nil {
				t.Fatal("TUN interface remains after close")
			}
		})
	}
}
