package clashapi

import (
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"

	"github.com/sagernet/sing-box/adapter"
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing/common"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
)

// ipcGetter is satisfied by *transport/awg.Device (via its promotion onto
// *protocol/awg.Endpoint) without importing either package — avoids a
// clashapi -> protocol/awg import that this package has no other reason to
// take on.
type ipcGetter interface {
	IpcGet() (string, error)
}

// awgPeerObject is one peer's slice of the UAPI "get" response — see
// transport/awg.Device.IpcGet. LastHandshake is unix seconds, 0 = never.
type awgPeerObject struct {
	PublicKey     string `json:"public_key"`
	LastHandshake int64  `json:"last_handshake"`
	TxBytes       int64  `json:"tx_bytes"`
	RxBytes       int64  `json:"rx_bytes"`
}

func awgRouter(endpointManager adapter.EndpointManager) http.Handler {
	r := chi.NewRouter()
	r.Get("/{tag}/peers", getAwgPeers(endpointManager))
	return r
}

// getAwgPeers reports live per-peer handshake/traffic state for an "awg"
// endpoint by tag, straight from the underlying WireGuard device's UAPI —
// there is no kernel interface to run "wg show" against, and the endpoint's
// traffic never surfaces through /connections (see IpcGet's doc comment), so
// this is the only accurate signal available for it.
func getAwgPeers(endpointManager adapter.EndpointManager) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		tag := chi.URLParam(r, "tag")
		ep, loaded := endpointManager.Get(tag)
		if !loaded || ep.Type() != C.TypeAwg {
			render.Status(r, http.StatusNotFound)
			render.JSON(w, r, ErrNotFound)
			return
		}
		getter, ok := ep.(ipcGetter)
		if !ok {
			render.Status(r, http.StatusNotImplemented)
			render.JSON(w, r, newError("endpoint does not expose peer stats"))
			return
		}
		uapi, err := getter.IpcGet()
		if err != nil {
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, newError(err.Error()))
			return
		}
		render.JSON(w, r, render.M{"peers": parseAwgUAPIPeers(uapi)})
	}
}

// parseAwgUAPIPeers walks the WireGuard configuration-protocol "get" text
// (https://www.wireguard.com/xplatform/#configuration-protocol) and collects
// one awgPeerObject per "public_key=" line, keyed by the fields that follow
// it up to the next "public_key=" line or EOF. Unparseable lines/values are
// skipped rather than failing the whole response — a partially-degraded
// snapshot is more useful to the caller than none.
func parseAwgUAPIPeers(uapi string) []awgPeerObject {
	var (
		peers []awgPeerObject
		cur   *awgPeerObject
	)
	for _, line := range strings.Split(uapi, "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch key {
		case "public_key":
			raw, err := hex.DecodeString(value)
			if err != nil {
				cur = nil
				continue
			}
			peers = append(peers, awgPeerObject{PublicKey: base64.StdEncoding.EncodeToString(raw)})
			cur = &peers[len(peers)-1]
		case "last_handshake_time_sec":
			if cur == nil {
				continue
			}
			cur.LastHandshake, _ = strconv.ParseInt(value, 10, 64)
		case "tx_bytes":
			if cur == nil {
				continue
			}
			cur.TxBytes, _ = strconv.ParseInt(value, 10, 64)
		case "rx_bytes":
			if cur == nil {
				continue
			}
			cur.RxBytes, _ = strconv.ParseInt(value, 10, 64)
		}
	}
	return common.Filter(peers, func(p awgPeerObject) bool { return p.PublicKey != "" })
}
