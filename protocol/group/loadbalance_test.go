package group

import (
	"context"
	"net/netip"
	"testing"
	"time"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/common/urltest"
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/log"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

func TestLoadBalance_RoundRobin(t *testing.T) {
	out1 := newMockTestOutbound("lb-node1")
	out2 := newMockTestOutbound("lb-node2")
	out3 := newMockTestOutbound("lb-node3")

	history := urltest.NewHistoryStorage()
	logger := log.NewNOPFactory().Logger()

	g := &LoadBalanceGroup{
		ctx:       context.Background(),
		logger:    logger,
		outbounds: []adapter.Outbound{out1, out2, out3},
		strategy:  "round-robin",
		history:   history,
	}

	// 1. Fail-open on cold start: should distribute across all outbounds
	picks := make(map[string]int)
	for i := 0; i < 9; i++ {
		detour, _ := g.SelectCandidate(N.NetworkTCP, M.Socksaddr{})
		picks[detour.Tag()]++
	}
	if picks["lb-node1"] != 3 || picks["lb-node2"] != 3 || picks["lb-node3"] != 3 {
		t.Fatalf("expected 3 picks each on cold start round-robin, got %+v", picks)
	}

	// 2. Mark node1 and node2 as healthy, node3 as down
	history.StoreURLTestHistory(RealTag(out1, N.NetworkTCP), &adapter.URLTestHistory{Time: time.Now(), Delay: 50})
	history.StoreURLTestHistory(RealTag(out2, N.NetworkTCP), &adapter.URLTestHistory{Time: time.Now(), Delay: 60})
	// node3 has no history (down)

	picks = make(map[string]int)
	for i := 0; i < 10; i++ {
		detour, isTested := g.SelectCandidate(N.NetworkTCP, M.Socksaddr{})
		if !isTested {
			t.Fatalf("expected isTested=true when healthy nodes exist")
		}
		picks[detour.Tag()]++
	}
	if picks["lb-node1"] != 5 || picks["lb-node2"] != 5 {
		t.Fatalf("expected 5 picks each for node1 and node2, got %+v", picks)
	}
	if picks["lb-node3"] != 0 {
		t.Fatalf("expected 0 picks for dead node3, got %d", picks["lb-node3"])
	}
}

func TestLoadBalance_ConsistentHashing(t *testing.T) {
	out1 := newMockTestOutbound("lb-node1")
	out2 := newMockTestOutbound("lb-node2")
	out3 := newMockTestOutbound("lb-node3")

	history := urltest.NewHistoryStorage()
	logger := log.NewNOPFactory().Logger()
	history.StoreURLTestHistory(RealTag(out1, N.NetworkTCP), &adapter.URLTestHistory{Time: time.Now(), Delay: 50})
	history.StoreURLTestHistory(RealTag(out2, N.NetworkTCP), &adapter.URLTestHistory{Time: time.Now(), Delay: 60})
	history.StoreURLTestHistory(RealTag(out3, N.NetworkTCP), &adapter.URLTestHistory{Time: time.Now(), Delay: 70})

	g := &LoadBalanceGroup{
		ctx:       context.Background(),
		logger:    logger,
		outbounds: []adapter.Outbound{out1, out2, out3},
		strategy:  "consistent-hashing",
		history:   history,
	}

	destA := M.Socksaddr{Fqdn: "google.com", Port: 443}
	destB := M.Socksaddr{Fqdn: "youtube.com", Port: 443}
	destC := M.Socksaddr{Addr: netip.MustParseAddr("1.1.1.1"), Port: 53}

	// For destA, every request MUST route to the exact same outbound
	firstPickA, _ := g.SelectCandidate(N.NetworkTCP, destA)
	for i := 0; i < 20; i++ {
		pick, _ := g.SelectCandidate(N.NetworkTCP, destA)
		if pick.Tag() != firstPickA.Tag() {
			t.Fatalf("consistent-hashing instability: expected %s, got %s", firstPickA.Tag(), pick.Tag())
		}
	}

	// For destB and destC, each must be deterministic
	firstPickB, _ := g.SelectCandidate(N.NetworkTCP, destB)
	for i := 0; i < 20; i++ {
		pick, _ := g.SelectCandidate(N.NetworkTCP, destB)
		if pick.Tag() != firstPickB.Tag() {
			t.Fatalf("consistent-hashing instability on destB: expected %s, got %s", firstPickB.Tag(), pick.Tag())
		}
	}

	firstPickC, _ := g.SelectCandidate(N.NetworkUDP, destC)
	for i := 0; i < 20; i++ {
		pick, _ := g.SelectCandidate(N.NetworkUDP, destC)
		if pick.Tag() != firstPickC.Tag() {
			t.Fatalf("consistent-hashing instability on destC: expected %s, got %s", firstPickC.Tag(), pick.Tag())
		}
	}
}

func TestLoadBalance_DisplayNameAndType(t *testing.T) {
	if C.TypeLoadBalance != "load-balance" {
		t.Fatalf("expected TypeLoadBalance = 'load-balance', got %q", C.TypeLoadBalance)
	}
	if disp := C.ProxyDisplayName(C.TypeLoadBalance); disp != "LoadBalance" {
		t.Fatalf("expected DisplayName = 'LoadBalance', got %q", disp)
	}
}
