//go:build !with_gvisor

package awg

import (
	"context"
	"net/netip"
	"time"

	tun "github.com/sagernet/sing-tun"
	"github.com/sagernet/sing/common/logger"
)

// newNonIntegratedTun without gVisor falls back to the amneziawg-go netstack
// (client-only DialContext); the tunnel cannot act as a gateway/exit because no
// inbound forwarder is available.
func newNonIntegratedTun(_ context.Context, address []netip.Prefix, mtu uint32, _ tun.Handler, _ time.Duration, _ logger.ContextLogger) (tunAdapter, error) {
	return newNetworkTun(address, mtu)
}
