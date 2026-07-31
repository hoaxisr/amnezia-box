package option

import (
	"encoding/json"
	"net/netip"
	"strconv"
	"strings"

	E "github.com/sagernet/sing/common/exceptions"
	"github.com/sagernet/sing/common/json/badoption"
)

type AwgEndpointOptions struct {
	UseIntegratedTun bool                             `json:"useIntegratedTun"`
	PrivateKey       string                           `json:"private_key"`
	Address          badoption.Listable[netip.Prefix] `json:"address"`
	MTU              uint32                           `json:"mtu,omitempty"`
	ListenPort       uint16                           `json:"listen_port,omitempty"`
	Jc               int                              `json:"jc,omitempty"`
	Jmin             int                              `json:"jmin,omitempty"`
	Jmax             int                              `json:"jmax,omitempty"`
	S1               int                              `json:"s1,omitempty"`
	S2               int                              `json:"s2,omitempty"`
	S3               int                              `json:"s3,omitempty"`
	S4               int                              `json:"s4,omitempty"`
	H1               string                           `json:"h1,omitempty"`
	H2               string                           `json:"h2,omitempty"`
	H3               string                           `json:"h3,omitempty"`
	H4               string                           `json:"h4,omitempty"`
	I1               string                           `json:"i1,omitempty"`
	I2               string                           `json:"i2,omitempty"`
	I3               string                           `json:"i3,omitempty"`
	I4               string                           `json:"i4,omitempty"`
	I5               string                           `json:"i5,omitempty"`

	HeaderProtectionKey    string `json:"header_protection_key,omitempty"`
	ContentPaddingAddition string `json:"content_padding_addition,omitempty"`
	RekeyAfterTime         string `json:"rekey_after_time,omitempty"`
	RekeyTimeout           string `json:"rekey_timeout,omitempty"`
	RejectAfterTime        string `json:"reject_after_time,omitempty"`
	KeepaliveTimeout       string `json:"keepalive_timeout,omitempty"`
	MaxHandshakeAttempts   string `json:"max_handshake_attempts,omitempty"`

	Peers []AwgPeerOptions `json:"peers,omitempty"`
	DialerOptions
}

type AwgPeerOptions struct {
	Address                     string                           `json:"address,omitempty"`
	Port                        uint16                           `json:"port,omitempty"`
	PublicKey                   string                           `json:"public_key,omitempty"`
	PresharedKey                string                           `json:"preshared_key,omitempty"`
	AllowedIPs                  badoption.Listable[netip.Prefix] `json:"allowed_ips,omitempty"`
	PersistentKeepaliveInterval AwgKeepalive                     `json:"persistent_keepalive_interval,omitempty"`
}

// AwgKeepalive is a persistent keepalive interval in seconds. AWG 3.0 made it a
// range, so a config carries either a plain number or "min-max", and the device
// draws a fresh value inside the range every time it arms the timer. Configs
// written before 3.0 hold a JSON number, so both shapes have to unmarshal.
type AwgKeepalive string

func (k AwgKeepalive) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(k))
}

func (k *AwgKeepalive) UnmarshalJSON(data []byte) error {
	var number uint16
	if err := json.Unmarshal(data, &number); err == nil {
		*k = AwgKeepalive(strconv.Itoa(int(number)))
		return nil
	}
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return E.New("persistent_keepalive_interval: expected a number or a \"min-max\" range")
	}
	if err := validateUintRange(value); err != nil {
		return E.Cause(err, "persistent_keepalive_interval")
	}
	*k = AwgKeepalive(value)
	return nil
}

// validateUintRange mirrors UintRange.FromString in amneziawg-go, so a broken
// range is reported while reading the config instead of when the device starts.
func validateUintRange(value string) error {
	lo, hi, isRange := strings.Cut(value, "-")
	loNum, err := strconv.ParseUint(strings.TrimSpace(lo), 10, 16)
	if err != nil {
		return E.New("bad lower bound in ", value)
	}
	if !isRange {
		return nil
	}
	hiNum, err := strconv.ParseUint(strings.TrimSpace(hi), 10, 16)
	if err != nil {
		return E.New("bad upper bound in ", value)
	}
	if hiNum < loNum {
		return E.New("upper bound below lower bound in ", value)
	}
	return nil
}
