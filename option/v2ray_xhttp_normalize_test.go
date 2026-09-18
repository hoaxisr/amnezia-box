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

// Хвостовой "/" нужен серверу только чтобы отделить sessionID и seq, когда те
// лежат в пути. При любом другом размещении он ломает совпадение пути на
// стороне сервера (Xray #6410).
func TestXHTTPNormalizedPathTrailingSlashOnlyForPathPlacement(t *testing.T) {
	for name, tc := range map[string]struct {
		session, seq string
		want         string
	}{
		"default":      {"", "", "/upload/"},
		"session path": {PlacementPath, PlacementQuery, "/upload/"},
		"seq path":     {PlacementQuery, PlacementPath, "/upload/"},
		"neither path": {PlacementQuery, PlacementHeader, "/upload"},
	} {
		o := V2RayXHTTPBaseOptions{
			Path:             "upload",
			SessionPlacement: tc.session,
			SeqPlacement:     tc.seq,
		}
		if got := o.GetNormalizedPath(); got != tc.want {
			t.Errorf("%s: got %q, want %q", name, got, tc.want)
		}
	}
}
