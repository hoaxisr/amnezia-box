package group

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/adapter/outbound"
	"github.com/sagernet/sing-box/common/urltest"
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/log"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

type mockTestOutbound struct {
	*outbound.Adapter
	tag string
}

func newMockTestOutbound(tag string) adapter.Outbound {
	ad := outbound.NewAdapter("direct", tag, []string{N.NetworkTCP, N.NetworkUDP}, nil)
	return &mockTestOutbound{
		Adapter: &ad,
		tag:     tag,
	}
}

func (m *mockTestOutbound) Tag() string {
	return m.tag
}

func (m *mockTestOutbound) Network() []string {
	return []string{N.NetworkTCP, N.NetworkUDP}
}

func (m *mockTestOutbound) DialContext(ctx context.Context, network string, destination M.Socksaddr) (net.Conn, error) {
	return nil, net.ErrClosed
}

func (m *mockTestOutbound) ListenPacket(ctx context.Context, destination M.Socksaddr) (net.PacketConn, error) {
	return nil, net.ErrClosed
}

func TestFallback_PriorityAndFailover(t *testing.T) {
	out1 := newMockTestOutbound("node1-primary")
	out2 := newMockTestOutbound("node2-backup")
	out3 := newMockTestOutbound("node3-tertiary")

	history := urltest.NewHistoryStorage()
	logger := log.NewNOPFactory().Logger()

	g := &FallbackGroup{
		ctx:       context.Background(),
		logger:    logger,
		outbounds: []adapter.Outbound{out1, out2, out3},
		history:   history,
	}

	// 1. Fail-Open when no history exists: should pick the first outbound
	selected, ok := g.Select(N.NetworkTCP)
	if selected == nil || selected.Tag() != "node1-primary" {
		t.Fatalf("expected node1-primary on cold start, got %v", selected)
	}
	if ok {
		t.Fatalf("expected ok=false on cold start fail-open")
	}

	// 2. Both node1 and node2 are healthy, but node2 has lower ping.
	// Fallback MUST pick node1 (strict priority), NOT node2 (lowest latency).
	history.StoreURLTestHistory(RealTag(out1, N.NetworkTCP), &adapter.URLTestHistory{
		Time:  time.Now(),
		Delay: 150, // higher ping
	})
	history.StoreURLTestHistory(RealTag(out2, N.NetworkTCP), &adapter.URLTestHistory{
		Time:  time.Now(),
		Delay: 20, // lower ping
	})
	history.StoreURLTestHistory(RealTag(out3, N.NetworkTCP), &adapter.URLTestHistory{
		Time:  time.Now(),
		Delay: 300,
	})

	selected, ok = g.Select(N.NetworkTCP)
	if !ok || selected.Tag() != "node1-primary" {
		t.Fatalf("expected node1-primary (priority), got %v (ok=%v)", selected, ok)
	}

	// 3. Primary node1 fails (deleted or Delay=0).
	// Fallback MUST failover to node2.
	history.DeleteURLTestHistory(RealTag(out1, N.NetworkTCP))

	selected, ok = g.Select(N.NetworkTCP)
	if !ok || selected.Tag() != "node2-backup" {
		t.Fatalf("expected node2-backup on primary failure, got %v (ok=%v)", selected, ok)
	}

	// 4. Primary and secondary fail. Fallback MUST failover to node3.
	history.DeleteURLTestHistory(RealTag(out2, N.NetworkTCP))

	selected, ok = g.Select(N.NetworkTCP)
	if !ok || selected.Tag() != "node3-tertiary" {
		t.Fatalf("expected node3-tertiary, got %v (ok=%v)", selected, ok)
	}

	// 5. Automatic recovery: primary node1 comes back online.
	// Fallback MUST immediately switch back to node1.
	history.StoreURLTestHistory(RealTag(out1, N.NetworkTCP), &adapter.URLTestHistory{
		Time:  time.Now(),
		Delay: 80,
	})

	selected, ok = g.Select(N.NetworkTCP)
	if !ok || selected.Tag() != "node1-primary" {
		t.Fatalf("expected immediate recovery to node1-primary, got %v (ok=%v)", selected, ok)
	}
}

func TestFallback_DisplayNameAndType(t *testing.T) {
	if C.TypeFallback != "fallback" {
		t.Fatalf("expected TypeFallback = 'fallback', got %q", C.TypeFallback)
	}
	if disp := C.ProxyDisplayName(C.TypeFallback); disp != "Fallback" {
		t.Fatalf("expected DisplayName = 'Fallback', got %q", disp)
	}
}
