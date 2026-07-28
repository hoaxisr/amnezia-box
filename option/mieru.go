package option

import "github.com/sagernet/sing/common/json/badoption"

type MieruOutboundOptions struct {
	DialerOptions
	ServerOptions
	ServerPortRanges badoption.Listable[string] `json:"server_ports,omitempty"`
	Transport        string                     `json:"transport,omitempty"`
	UserName         string                     `json:"username,omitempty"`
	Password         string                     `json:"password,omitempty"`
	Multiplexing     string                     `json:"multiplexing,omitempty"`
	TrafficPattern   string                     `json:"traffic_pattern,omitempty"`
}

type MieruInboundOptions struct {
	ListenOptions
	// ListenPortRanges are extra "lo-hi" port ranges the server binds, on top of
	// listen_port. Mirrors server_ports on the outbound: mieru's own server takes
	// a list of port bindings, and clients that rotate across a range need the
	// server to be listening on all of them. Either listen_port or listen_ports
	// must be set.
	ListenPortRanges    badoption.Listable[string] `json:"listen_ports,omitempty"`
	Users               []MieruUser                `json:"users,omitempty"`
	Transport           string                     `json:"transport,omitempty"`
	TrafficPattern      string                     `json:"traffic_pattern,omitempty"`
	UserHintIsMandatory bool                       `json:"user_hint_is_mandatory,omitempty"`
}

type MieruUser struct {
	Name     string `json:"name,omitempty"`
	Password string `json:"password,omitempty"`
}
