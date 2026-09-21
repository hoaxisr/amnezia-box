package option

type TrustTunnelOutboundOptions struct {
	DialerOptions
	ServerOptions
	Username              string `json:"username,omitempty"`
	Password              string `json:"password,omitempty"`
	HealthCheck           bool   `json:"health_check,omitempty"`
	QUIC                  bool   `json:"quic,omitempty"`
	QUICCongestionControl string `json:"quic_congestion_control,omitempty"`
	OutboundTLSOptionsContainer
}
