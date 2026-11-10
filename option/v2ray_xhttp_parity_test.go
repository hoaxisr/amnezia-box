package option

import (
	"testing"

	"github.com/sagernet/sing/common/json"
)

func parseXHTTP(t *testing.T, raw string) (V2RayXHTTPOptions, error) {
	t.Helper()
	var o V2RayXHTTPOptions
	err := json.Unmarshal([]byte(raw), &o)
	return o, err
}

// Умолчания и проверки как у Xray (infra/conf/transport_method.go).
func TestXHTTPXrayParity(t *testing.T) {
	if _, err := parseXHTTP(t, `{"mode":"packet-up"}`); err != nil {
		t.Fatalf("x_padding_bytes absent must default, got %v", err)
	}
	if _, err := parseXHTTP(t, `{"x_padding_bytes":"0-10"}`); err == nil {
		t.Fatal("explicit zero padding must be rejected")
	}
	o, err := parseXHTTP(t, `{"xmux":{}}`)
	if err != nil || o.Xmux.MaxConnections.To != 3 || o.Xmux.HMaxRequestTimes.From != 600 {
		t.Fatalf("empty xmux must get Xray defaults: %+v %v", o.Xmux, err)
	}
	if _, err := parseXHTTP(t, `{"mode":"stream-one","download":{"server":"h","server_port":443}}`); err == nil {
		t.Fatal("download in stream-one must be rejected")
	}
	o, err = parseXHTTP(t, `{"session_id_table":"Base62","session_id_length":"8-12"}`)
	if err != nil || len(o.SessionIDTable) != 62 {
		t.Fatalf("predefined table not expanded: %q %v", o.SessionIDTable, err)
	}
	for _, bad := range []string{
		`{"session_id_table":"hex","session_id_length":"4"}`,
		`{"session_id_table":"hex","session_id_length":"0-10"}`,
		`{"session_id_table":"абв","session_id_length":"40"}`,
	} {
		if _, err := parseXHTTP(t, bad); err == nil {
			t.Errorf("%s must be rejected", bad)
		}
	}
}
