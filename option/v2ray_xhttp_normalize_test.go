package option

import (
	"testing"

	Xbadoption "github.com/sagernet/sing-box/common/xray/json/badoption"
)

// Zero upper bound means "not set" in Xray, which substitutes the default;
// without the parity the zero range reached the client and killed the process.
func TestXHTTPNormalizeZeroUpperBoundFallsBackToDefault(t *testing.T) {
	zero := &Xbadoption.Range{}
	o := V2RayXHTTPBaseOptions{
		ScMaxEachPostBytes:   zero,
		ScMinPostsIntervalMs: zero,
		ScStreamUpServerSecs: zero,
	}
	for name, got := range map[string]Xbadoption.Range{
		"sc_max_each_post_bytes":   o.GetNormalizedScMaxEachPostBytes(),
		"sc_min_posts_interval_ms": o.GetNormalizedScMinPostsIntervalMs(),
		"sc_stream_up_server_secs": o.GetNormalizedScStreamUpServerSecs(),
	} {
		if got.From <= 0 || got.To <= 0 {
			t.Errorf("%s: got %+v, want non-zero default", name, got)
		}
	}
}
