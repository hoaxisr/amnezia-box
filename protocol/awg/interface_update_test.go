package awg

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/transport/awg"
	"github.com/sagernet/sing/common/logger"
	M "github.com/sagernet/sing/common/metadata"
)

// countingDialer counts the sockets the device bind opens.
type countingDialer struct{ listens atomic.Int32 }

func (d *countingDialer) DialContext(context.Context, string, M.Socksaddr) (net.Conn, error) {
	return nil, errors.New("dial not supported in tests")
}

func (d *countingDialer) ListenPacket(context.Context, M.Socksaddr) (net.PacketConn, error) {
	d.listens.Add(1)
	return net.ListenUDP("udp", &net.UDPAddr{})
}

func newCountingEndpoint(t *testing.T, start bool) (*Endpoint, *countingDialer) {
	t.Helper()
	dial := &countingDialer{}
	ipc := "private_key=" + strings.Repeat("cd", 32) +
		"\npublic_key=" + strings.Repeat("ab", 32) +
		"\nallowed_ip=0.0.0.0/0"
	dev, err := awg.NewDevice(context.Background(), logger.NOP(), dial, ipc, awg.DeviceOpts{
		Address: []netip.Prefix{netip.MustParsePrefix("10.82.0.1/24")},
		MTU:     1280,
	})
	if err != nil {
		t.Fatal(err)
	}
	scope := adapter.NewScope(context.Background(), log.NewNOPFactory().Logger())
	t.Cleanup(func() { scope.Close() })
	if start {
		for _, stage := range []adapter.StartStage{adapter.StartStateInitialize, adapter.StartStateStart} {
			if err = dev.Start(stage, scope); err != nil {
				t.Fatal(err)
			}
		}
	}
	return &Endpoint{Device: dev, logger: log.NewNOPFactory().Logger()}, dial
}

// #1183: a default-interface change must reopen the sockets, otherwise the
// endpoint keeps sending through the interface that disappeared.
func TestInterfaceUpdatedReopensSockets(t *testing.T) {
	ep, dial := newCountingEndpoint(t, true)
	before := dial.listens.Load()
	if before == 0 {
		t.Fatal("device opened no sockets on start")
	}
	ep.InterfaceUpdated(context.Background())
	deadline := time.Now().Add(5 * time.Second)
	for dial.listens.Load() <= before {
		if time.Now().After(deadline) {
			t.Fatalf("sockets were not reopened: %d listens before and after", before)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestInterfaceUpdatedIgnoresStoppedEndpoint(t *testing.T) {
	ep, dial := newCountingEndpoint(t, false)
	ep.InterfaceUpdated(context.Background())
	time.Sleep(100 * time.Millisecond)
	if n := dial.listens.Load(); n != 0 {
		t.Fatalf("stopped endpoint opened %d sockets", n)
	}
}
