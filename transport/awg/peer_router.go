package awg

import (
	"net/netip"
	"sync/atomic"

	"github.com/amnezia-vpn/amneziawg-go/v3/device"

	"github.com/sagernet/sing-tun"
	"github.com/sagernet/sing-tun/gtcpip/header"
	"github.com/sagernet/sing/common/buf"
)

// peerRouter feeds packets leaving the Go stack straight into the peer that
// owns their destination, one sing-tun OutboundQueue per peer, so a stalled
// peer backpressures only its own flows. It mirrors
// transport/wireguard.peerRouter; routing uses the AllowedIPs trie by
// destination, amneziawg-go has no per-packet peer callback.
type peerRouter struct {
	memoryTun    *tun.MemoryTun
	inet4Address netip.Addr
	inet6Address netip.Addr
	routes       atomic.Pointer[peerRoutes]
}

type peerRoutes struct {
	allowedIPs *device.AllowedIPs
	queues     map[*device.Peer]*peerQueue
}

type peerQueue struct {
	peer    *device.Peer
	queue   *tun.OutboundQueue
	packets [][]byte
}

func newPeerRouter(address []netip.Prefix, mtu uint32) *peerRouter {
	router := &peerRouter{}
	for _, prefix := range address {
		if prefix.Addr().Is4() {
			router.inet4Address = prefix.Addr()
		} else {
			router.inet6Address = prefix.Addr()
		}
	}
	router.memoryTun = tun.NewMemoryTun(tun.MemoryTunOptions{
		MTU:      int(mtu),
		Outbound: router.writeUnrouted,
		Route:    router.route,
	})
	return router
}

func (r *peerRouter) setPeers(allowedIPs *device.AllowedIPs, peers []*device.Peer) {
	routes := &peerRoutes{
		allowedIPs: allowedIPs,
		queues:     make(map[*device.Peer]*peerQueue, len(peers)),
	}
	for _, peer := range peers {
		current := &peerQueue{peer: peer}
		current.queue = r.memoryTun.NewOutboundQueue(current.write)
		routes.queues[peer] = current
	}
	previous := r.routes.Swap(routes)
	if previous != nil {
		previous.close()
	}
}

func (r *peerRouter) route(packet []byte) *tun.OutboundQueue {
	routes := r.routes.Load()
	if routes == nil {
		return nil
	}
	var destination []byte
	switch header.IPVersion(packet) {
	case header.IPv4Version:
		destination = header.IPv4(packet).DestinationAddressSlice()
	case header.IPv6Version:
		destination = header.IPv6(packet).DestinationAddressSlice()
	default:
		return nil
	}
	current := routes.queues[routes.allowedIPs.Lookup(destination)]
	if current == nil {
		return nil
	}
	return current.queue
}

func (r *peerRouter) writeUnrouted(packetBuffers []*buf.Buffer) {
	defer buf.ReleaseMulti(packetBuffers)
	replies := make([][]byte, 0, len(packetBuffers))
	for _, packetBuffer := range packetBuffers {
		packet := packetBuffer.Bytes()
		source := r.inet4Address
		if header.IPVersion(packet) == header.IPv6Version {
			source = r.inet6Address
		}
		reply, built := tun.BuildICMPError(packet, tun.ICMPErrorNoRoute, source, 0, 0)
		if built {
			replies = append(replies, reply)
		}
	}
	if len(replies) > 0 {
		r.memoryTun.WritePackets(replies)
	}
}

func (r *peerRouter) close() error {
	routes := r.routes.Swap(nil)
	if routes != nil {
		routes.close()
	}
	return r.memoryTun.Close()
}

func (r *peerRoutes) close() {
	for _, current := range r.queues {
		current.queue.Close()
	}
}

func (q *peerQueue) write(packetBuffers []*buf.Buffer) {
	for _, packetBuffer := range packetBuffers {
		q.packets = append(q.packets, packetBuffer.Bytes())
	}
	q.peer.WritePackets(q.packets)
	clear(q.packets)
	q.packets = q.packets[:0]
	buf.ReleaseMulti(packetBuffers)
}
