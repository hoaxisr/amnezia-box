//go:build ultra_minimal && !nano

package include

import (
	"context"

	box "github.com/sagernet/sing-box"
	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/adapter/endpoint"
	"github.com/sagernet/sing-box/adapter/inbound"
	"github.com/sagernet/sing-box/adapter/outbound"
	"github.com/sagernet/sing-box/adapter/service"
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/dns"
	"github.com/sagernet/sing-box/dns/transport"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing-box/protocol/block"
	"github.com/sagernet/sing-box/protocol/direct"
	protocolDNS "github.com/sagernet/sing-box/protocol/dns"
	"github.com/sagernet/sing-box/protocol/mixed"
	"github.com/sagernet/sing-box/protocol/tun"
	"github.com/sagernet/sing-box/protocol/vless"
	E "github.com/sagernet/sing/common/exceptions"
)

// Ultra-minimal context for maximum size reduction
// Only: tun, mixed (inbound), vless, direct, block, dns (outbound), awg (endpoint)
// No: selector, urltest, fakeip, hosts, local, resolved, TLS/HTTPS DNS
func Context(ctx context.Context) context.Context {
	return box.Context(ctx, InboundRegistry(), OutboundRegistry(), EndpointRegistry(), DNSTransportRegistry(), ServiceRegistry())
}

func InboundRegistry() *inbound.Registry {
	registry := inbound.NewRegistry()
	tun.RegisterInbound(registry)
	mixed.RegisterInbound(registry)
	registerUltraMinimalInboundStubs(registry)
	return registry
}

func OutboundRegistry() *outbound.Registry {
	registry := outbound.NewRegistry()
	direct.RegisterOutbound(registry)
	block.RegisterOutbound(registry)
	protocolDNS.RegisterOutbound(registry)
	vless.RegisterOutbound(registry)
	registerUltraMinimalOutboundStubs(registry)
	return registry
}

func EndpointRegistry() *endpoint.Registry {
	registry := endpoint.NewRegistry()
	registerAwgEndpoint(registry)
	registerUltraMinimalEndpointStubs(registry)
	return registry
}

func DNSTransportRegistry() *dns.TransportRegistry {
	registry := dns.NewTransportRegistry()
	// Only UDP DNS - minimal footprint
	transport.RegisterUDP(registry)
	return registry
}

func ServiceRegistry() *service.Registry {
	return service.NewRegistry()
}

func registerUltraMinimalInboundStubs(registry *inbound.Registry) {
	inbound.Register[option.DirectInboundOptions](registry, C.TypeDirect, func(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, options option.DirectInboundOptions) (adapter.Inbound, error) {
		return nil, E.New("direct inbound not in ultra-minimal build")
	})
	inbound.Register[option.SocksInboundOptions](registry, C.TypeSOCKS, func(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, options option.SocksInboundOptions) (adapter.Inbound, error) {
		return nil, E.New("socks not in ultra-minimal build, use mixed")
	})
	inbound.Register[option.HTTPMixedInboundOptions](registry, C.TypeHTTP, func(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, options option.HTTPMixedInboundOptions) (adapter.Inbound, error) {
		return nil, E.New("http not in ultra-minimal build, use mixed")
	})
	inbound.Register[option.ShadowsocksInboundOptions](registry, C.TypeShadowsocks, func(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, options option.ShadowsocksInboundOptions) (adapter.Inbound, error) {
		return nil, E.New("shadowsocks not in ultra-minimal build")
	})
	inbound.Register[option.VMessInboundOptions](registry, C.TypeVMess, func(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, options option.VMessInboundOptions) (adapter.Inbound, error) {
		return nil, E.New("vmess not in ultra-minimal build")
	})
	inbound.Register[option.TrojanInboundOptions](registry, C.TypeTrojan, func(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, options option.TrojanInboundOptions) (adapter.Inbound, error) {
		return nil, E.New("trojan not in ultra-minimal build")
	})
	inbound.Register[option.VLESSInboundOptions](registry, C.TypeVLESS, func(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, options option.VLESSInboundOptions) (adapter.Inbound, error) {
		return nil, E.New("vless inbound not in ultra-minimal build")
	})
	inbound.Register[option.RedirectInboundOptions](registry, C.TypeRedirect, func(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, options option.RedirectInboundOptions) (adapter.Inbound, error) {
		return nil, E.New("redirect not in ultra-minimal build")
	})
	inbound.Register[option.TProxyInboundOptions](registry, C.TypeTProxy, func(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, options option.TProxyInboundOptions) (adapter.Inbound, error) {
		return nil, E.New("tproxy not in ultra-minimal build")
	})
}

func registerUltraMinimalOutboundStubs(registry *outbound.Registry) {
	outbound.Register[option.SelectorOutboundOptions](registry, C.TypeSelector, func(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, options option.SelectorOutboundOptions) (adapter.Outbound, error) {
		return nil, E.New("selector not in ultra-minimal build")
	})
	outbound.Register[option.URLTestOutboundOptions](registry, C.TypeURLTest, func(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, options option.URLTestOutboundOptions) (adapter.Outbound, error) {
		return nil, E.New("urltest not in ultra-minimal build")
	})
	outbound.Register[option.SOCKSOutboundOptions](registry, C.TypeSOCKS, func(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, options option.SOCKSOutboundOptions) (adapter.Outbound, error) {
		return nil, E.New("socks outbound not in ultra-minimal build")
	})
	outbound.Register[option.HTTPOutboundOptions](registry, C.TypeHTTP, func(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, options option.HTTPOutboundOptions) (adapter.Outbound, error) {
		return nil, E.New("http outbound not in ultra-minimal build")
	})
	outbound.Register[option.ShadowsocksOutboundOptions](registry, C.TypeShadowsocks, func(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, options option.ShadowsocksOutboundOptions) (adapter.Outbound, error) {
		return nil, E.New("shadowsocks not in ultra-minimal build")
	})
	outbound.Register[option.VMessOutboundOptions](registry, C.TypeVMess, func(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, options option.VMessOutboundOptions) (adapter.Outbound, error) {
		return nil, E.New("vmess not in ultra-minimal build")
	})
	outbound.Register[option.TrojanOutboundOptions](registry, C.TypeTrojan, func(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, options option.TrojanOutboundOptions) (adapter.Outbound, error) {
		return nil, E.New("trojan not in ultra-minimal build")
	})
	outbound.Register[option.SSHOutboundOptions](registry, C.TypeSSH, func(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, options option.SSHOutboundOptions) (adapter.Outbound, error) {
		return nil, E.New("ssh not in ultra-minimal build")
	})
	outbound.Register[option.TorOutboundOptions](registry, C.TypeTor, func(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, options option.TorOutboundOptions) (adapter.Outbound, error) {
		return nil, E.New("tor not in ultra-minimal build")
	})
}

func registerUltraMinimalEndpointStubs(registry *endpoint.Registry) {
	endpoint.Register[option.WireGuardEndpointOptions](registry, C.TypeWireGuard, func(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, options option.WireGuardEndpointOptions) (adapter.Endpoint, error) {
		return nil, E.New("WireGuard not in ultra-minimal build, use AWG")
	})
	endpoint.Register[option.TailscaleEndpointOptions](registry, C.TypeTailscale, func(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, options option.TailscaleEndpointOptions) (adapter.Endpoint, error) {
		return nil, E.New("Tailscale not in ultra-minimal build")
	})
}
