//go:build ultra_minimal && !nano

package v2ray

import (
	"context"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/common/tls"
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/option"
	E "github.com/sagernet/sing/common/exceptions"
	"github.com/sagernet/sing/common/logger"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

// Type definitions needed by other files in this package
type (
	ServerConstructor[O any] func(ctx context.Context, logger logger.ContextLogger, options O, tlsConfig tls.ServerConfig, handler adapter.V2RayServerTransportHandler) (adapter.V2RayServerTransport, error)
	ClientConstructor[O any] func(ctx context.Context, dialer N.Dialer, serverAddr M.Socksaddr, options O, tlsConfig tls.Config) (adapter.V2RayClientTransport, error)
)

// Ultra-minimal transport handler - only TCP (no transport), no gRPC
// All V2Ray transports are excluded

func NewServerTransport(ctx context.Context, logger logger.ContextLogger, options option.V2RayTransportOptions, tlsConfig tls.ServerConfig, handler adapter.V2RayServerTransportHandler) (adapter.V2RayServerTransport, error) {
	if options.Type == "" {
		return nil, nil
	}
	switch options.Type {
	case C.V2RayTransportTypeGRPC:
		return nil, E.New("gRPC transport is not included in ultra-minimal build")
	case C.V2RayTransportTypeHTTP:
		return nil, E.New("HTTP transport is not included in ultra-minimal build")
	case C.V2RayTransportTypeWebsocket:
		return nil, E.New("WebSocket transport is not included in ultra-minimal build")
	case C.V2RayTransportTypeQUIC:
		return nil, E.New("QUIC transport is not included in ultra-minimal build")
	case C.V2RayTransportTypeHTTPUpgrade:
		return nil, E.New("HTTPUpgrade transport is not included in ultra-minimal build")
	default:
		return nil, E.New("unknown transport type: " + options.Type)
	}
}

func NewClientTransport(ctx context.Context, dialer N.Dialer, serverAddr M.Socksaddr, options option.V2RayTransportOptions, tlsConfig tls.Config) (adapter.V2RayClientTransport, error) {
	if options.Type == "" {
		return nil, nil
	}
	switch options.Type {
	case C.V2RayTransportTypeGRPC:
		return nil, E.New("gRPC transport is not included in ultra-minimal build")
	case C.V2RayTransportTypeHTTP:
		return nil, E.New("HTTP transport is not included in ultra-minimal build")
	case C.V2RayTransportTypeWebsocket:
		return nil, E.New("WebSocket transport is not included in ultra-minimal build")
	case C.V2RayTransportTypeQUIC:
		return nil, E.New("QUIC transport is not included in ultra-minimal build")
	case C.V2RayTransportTypeHTTPUpgrade:
		return nil, E.New("HTTPUpgrade transport is not included in ultra-minimal build")
	default:
		return nil, E.New("unknown transport type: " + options.Type)
	}
}
