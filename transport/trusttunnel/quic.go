//go:build with_quic

package trusttunnel

import (
	"context"
	stdTLS "crypto/tls"
	"net"
	"runtime"
	"time"

	"github.com/sagernet/quic-go"
	"github.com/sagernet/quic-go/congestion"
	"github.com/sagernet/quic-go/http3"
	"github.com/sagernet/sing-quic"
	congestion_meta1 "github.com/sagernet/sing-quic/congestion_meta1"
	congestion_meta2 "github.com/sagernet/sing-quic/congestion_meta2"
	E "github.com/sagernet/sing/common/exceptions"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/common/tls"
)

func (c *Client) quicRoundTripper(tlsConfig tls.Config, congestionControlName string) error {
	tlsConfig = tlsConfig.Clone()
	c.connTracker = newConnTracker()
	c.roundTripper = &http3.Transport{
		QUICConfig: &quic.Config{
			Versions:                   []quic.Version{quic.Version1},
			MaxIdleTimeout:             DefaultQuicMaxIdleTimeout,
			InitialStreamReceiveWindow: DefaultQuicStreamReceiveWindow,
			DisablePathMTUDiscovery:    !(runtime.GOOS == "windows" || runtime.GOOS == "linux" || runtime.GOOS == "android" || runtime.GOOS == "darwin"),
			Allow0RTT:                  false,
		},
		Dial: func(ctx context.Context, addr string, _ *stdTLS.Config, cfg *quic.Config) (*quic.Conn, error) {
			conn, err := c.detour.DialContext(ctx, N.NetworkUDP, c.server)
			if err != nil {
				return nil, err
			}
			conn = c.connTracker.track(conn)
			// What http3 do for tls config: set SNI and set ALPN.
			// We have already done.
			quicConn, err := qtls.DialEarly(ctx, conn, tlsConfig, cfg)
			if err != nil {
				_ = conn.Close()
				return nil, err
			}
			setCongestionControl(c.timeFunc, quicConn, congestionControlName)
			// quic-go never closes a net.Conn it did not create itself, so the socket
			// and its tracker entry have to be released when the connection ends.
			_ = context.AfterFunc(quicConn.Context(), func() {
				_ = conn.Close()
			})
			return quicConn, nil
		},
	}
	c.wrapError = qtls.WrapError
	return nil
}

func (s *Service) configHTTP3Server(tlsConfig tls.ServerConfig, packetConn net.PacketConn) error {
	tlsConfig = tlsConfig.Clone().(tls.ServerConfig)
	err := qtls.ConfigureHTTP3(tlsConfig)
	if err != nil {
		return err
	}
	// https://github.com/SagerNet/sing-quic/blob/2afc335e0cddca3346d22ac42b26098faa783975/quic.go#L125
	// qtls.ConfigureHTTP3 never work because http3.ConfigureTLSConfig modified and returns a copy.
	// https://github.com/quic-go/quic-go/blob/c56e8c79d1627cc1ed6005b421b4b0adadd83665/http3/server.go#L47-L63
	tlsConfig.SetNextProtos([]string{http3.NextProtoH3})
	quicListener, err := qtls.ListenEarly(packetConn, tlsConfig, &quic.Config{
		Versions:           []quic.Version{quic.Version1},
		MaxIdleTimeout:     DefaultQuicMaxIdleTimeout,
		MaxIncomingStreams: 1 << 60,
		Allow0RTT:          true,
	})
	if err != nil {
		return err
	}
	h3Server := &http3.Server{
		Handler:     s,
		IdleTimeout: DefaultSessionTimeout,
		ConnContext: func(ctx context.Context, conn *quic.Conn) context.Context {
			setCongestionControl(s.timeFunc, conn, s.quicCongestionControl)
			ctx = contextWithWrapError(ctx, qtls.WrapError)
			return ctx
		},
	}
	s.h3Server = h3Server
	s.packetConn = packetConn
	go func() {
		sErr := h3Server.ServeListener(quicListener)
		if sErr != nil && !E.IsClosedOrCanceled(sErr) {
			s.logger.ErrorContext(s.ctx, "HTTP3 server close: ", sErr)
		}
	}()
	return nil
}

func setCongestionControl(timeFunc func() time.Time, conn *quic.Conn, name string) {
	var congestionControl congestion.CongestionControl
	initialPacketSize := conn.InitialPacketSize()
	switch name {
	case "cubic":
		congestionControl = congestion_meta1.NewCubicSender(
			congestion_meta1.DefaultClock{TimeFunc: timeFunc},
			initialPacketSize,
			false,
		)
	case "reno":
		congestionControl = congestion_meta1.NewCubicSender(
			congestion_meta1.DefaultClock{TimeFunc: timeFunc},
			initialPacketSize,
			true,
		)
	case "", "bbr":
		fallthrough
	default:
		congestionControl = congestion_meta2.NewBbrSenderWithProfile(initialPacketSize, congestion_meta2.ProfileStandard)
	}
	conn.SetCongestionControl(congestionControl)
}
