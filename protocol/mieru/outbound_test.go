package mieru

import (
	"context"
	"net"
	"net/netip"
	"testing"

	"github.com/sagernet/sing-box/adapter"
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/option"
)

// stubDNSRouter answers every lookup with a fixed address, recording the strategy.
type stubDNSRouter struct {
	adapter.DNSRouter
	addresses []netip.Addr
	strategy  C.DomainStrategy
}

func (r *stubDNSRouter) Lookup(ctx context.Context, domain string, options adapter.DNSQueryOptions) ([]netip.Addr, error) {
	r.strategy = options.Strategy
	return r.addresses, nil
}

// The packet underlay resolves the server address itself, so a mieru client
// without a resolver cannot use UDP transport with a domain server address.
func TestClientConfigCarriesResolver(t *testing.T) {
	options := option.MieruOutboundOptions{
		Transport: "UDP",
		UserName:  "user",
		Password:  "pass",
	}
	options.Server = "example.com"
	options.ServerPortRanges = []string{"25010-25012"}

	config, err := buildMieruClientConfig(options, mieruDialer{}, mieruResolver{router: &stubDNSRouter{}})
	if err != nil {
		t.Fatal(err)
	}
	if config.Resolver == nil {
		t.Fatal("client config has no resolver: UDP transport would fail on a domain server")
	}
}

func TestResolverLookupIP(t *testing.T) {
	router := &stubDNSRouter{addresses: []netip.Addr{netip.MustParseAddr("192.0.2.7")}}
	resolver := mieruResolver{router: router}

	addresses, err := resolver.LookupIP(context.Background(), "ip4", "example.com")
	if err != nil {
		t.Fatal(err)
	}
	if len(addresses) != 1 || !addresses[0].Equal(net.ParseIP("192.0.2.7")) {
		t.Fatalf("unexpected addresses: %v", addresses)
	}
	if router.strategy != C.DomainStrategyIPv4Only {
		t.Fatalf("expected an IPv4-only lookup, got %v", router.strategy)
	}

	if _, err := (mieruResolver{}).LookupIP(context.Background(), "ip", "example.com"); err == nil {
		t.Fatal("expected an error without a DNS router")
	}
}
