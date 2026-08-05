package awg

import (
	"net/netip"
	"sync/atomic"

	"github.com/amnezia-vpn/amneziawg-go/v3/device"

	tun "github.com/sagernet/sing-tun"
	"github.com/sagernet/sing-tun/gtcpip/header"
	E "github.com/sagernet/sing/common/exceptions"
)

func (d *Device) PortAddresses() (netip.Addr, netip.Addr) {
	var v4, v6 netip.Addr
	for _, prefix := range d.address {
		if prefix.Addr().Is4() {
			v4 = prefix.Addr()
		} else {
			v6 = prefix.Addr()
		}
	}
	return v4, v6
}

func (d *Device) PortMTU() uint32 {
	return d.mtu
}

// WritePackets injects raw IP packets into the tunnel so they get encrypted and
// sent to the peer, as if RoutineReadFromTUN had read them from the tun device.
// It mirrors transport/wireguard.Endpoint.WritePackets, using amneziawg-go's
// device.InputPackets (added in the patched fork). Packets whose destination
// matches no peer are answered with an ICMP unreachable via the return path.
func (d *Device) WritePackets(packets [][]byte) error {
	awgDevice := d.awgDevice
	if awgDevice == nil || !d.started.Load() {
		return E.New("AmneziaWG is not ready yet")
	}
	packetRefs := make([]*device.InputPacketRef, 0, len(packets))
	refs := make([]device.InputPacketRef, len(packets))
	packetSlices := make([][]byte, len(packets))
	for i, packet := range packets {
		if len(packet) == 0 {
			continue
		}
		var destination []byte
		switch header.IPVersion(packet) {
		case header.IPv4Version:
			if len(packet) < header.IPv4MinimumSize {
				continue
			}
			destination = header.IPv4(packet).DestinationAddressSlice()
		case header.IPv6Version:
			if len(packet) < header.IPv6MinimumSize {
				continue
			}
			destination = header.IPv6(packet).DestinationAddressSlice()
		default:
			continue
		}
		packetSlices[i] = packet
		refs[i] = device.InputPacketRef{
			Destination:  destination,
			PacketSlices: packetSlices[i : i+1],
		}
		packetRefs = append(packetRefs, &refs[i])
	}
	if len(packetRefs) == 0 {
		return nil
	}
	unmatchedRefs := awgDevice.InputPackets(packetRefs)
	if len(unmatchedRefs) == 0 {
		return nil
	}
	state := d.returnDevice.state.Load()
	if state == nil {
		return nil
	}
	v4, v6 := d.PortAddresses()
	var replies [][]byte
	for _, packetRef := range unmatchedRefs {
		packet := packetRef.PacketSlices[0]
		var source netip.Addr
		if header.IPVersion(packet) == header.IPv4Version {
			source = v4
		} else {
			source = v6
		}
		reply, replyOk := tun.BuildUnreachable(packet, source, state.headroom)
		if replyOk {
			replies = append(replies, reply)
		}
	}
	if len(replies) > 0 {
		state.returnPath.ReturnPackets(replies)
	}
	return nil
}

func (d *Device) AttachReturn(returnPath tun.Return) error {
	headroom := returnPath.ReturnHeadroom()
	if headroom > device.MessageTransportOffsetContent {
		return E.New("return path headroom ", headroom, " exceeds available ", device.MessageTransportOffsetContent)
	}
	newState := &returnPathState{
		returnPath: returnPath,
		headroom:   headroom,
	}
	for {
		currentState := d.returnDevice.state.Load()
		if currentState != nil {
			if currentState.returnPath == returnPath {
				return nil
			}
			return E.New("return path already attached")
		}
		if d.returnDevice.state.CompareAndSwap(nil, newState) {
			return nil
		}
	}
}

func (d *Device) DetachReturn(returnPath tun.Return) error {
	currentState := d.returnDevice.state.Load()
	if currentState != nil && currentState.returnPath == returnPath {
		d.returnDevice.state.CompareAndSwap(currentState, nil)
	}
	return nil
}

type returnPathState struct {
	returnPath tun.Return
	headroom   int
}

// returnDeviceWrapper wraps the AWG tunAdapter so decrypted inbound packets the
// AWG device writes are first offered to an attached flow return path before
// falling through to the local tun. It is the amneziawg-go analogue of
// transport/wireguard.returnDeviceWrapper.
type returnDeviceWrapper struct {
	tunAdapter
	state atomic.Pointer[returnPathState]
}

func newReturnDevice(real tunAdapter) *returnDeviceWrapper {
	return &returnDeviceWrapper{tunAdapter: real}
}

func (d *returnDeviceWrapper) Write(bufs [][]byte, offset int) (int, error) {
	state := d.state.Load()
	if state == nil || len(bufs) == 0 {
		return d.tunAdapter.Write(bufs, offset)
	}
	packets := make([][]byte, len(bufs))
	for i, packet := range bufs {
		// amneziawg-go leaves device.MessageTransportOffsetContent writable
		// bytes in front of the decrypted packet.
		packets[i] = packet[offset-state.headroom:]
	}
	unconsumed := state.returnPath.ReturnPackets(packets)
	if len(unconsumed) == 0 {
		return 0, nil
	}
	if len(unconsumed) == len(bufs) {
		return d.tunAdapter.Write(bufs, offset)
	}
	remaining := make([][]byte, 0, len(unconsumed))
	searchIndex := 0
	for _, packet := range unconsumed {
		for searchIndex < len(bufs) && &packet[0] != &bufs[searchIndex][offset-state.headroom] {
			searchIndex++
		}
		if searchIndex == len(bufs) {
			break
		}
		remaining = append(remaining, bufs[searchIndex])
		searchIndex++
	}
	return d.tunAdapter.Write(remaining, offset)
}
