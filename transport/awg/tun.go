package awg

import (
	"github.com/amnezia-vpn/amneziawg-go/v3/device"
	awgTun "github.com/amnezia-vpn/amneziawg-go/v3/tun"
	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing/common/network"
)

type tunAdapter interface {
	network.Dialer
	awgTun.Device
	adapter.SimpleLifecycle
	SetDevice(device *device.Device, peers []*device.Peer)
}
