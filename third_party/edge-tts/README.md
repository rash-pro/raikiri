# Edge TTS compatibility patch

This directory contains `github.com/bytectlgo/edge-tts` v1.0.5's Go module
and library source, selected through the root `go.mod` replacement.

On 2026-10-13, the live service rejected its v1 authentication with
`websocket: bad handshake`, so Raikiri accepted TTS jobs without producing
any audio. The Go upstream had no newer release or fix available.

The local patch changes `Sec-MS-GEC-Version` to v2 and ports the v2 token
algorithm from rany2/edge-tts commit
[`ba61d94442c18893f0c4d3c5ad253718b21ab93e`](https://github.com/rany2/edge-tts/commit/ba61d94442c18893f0c4d3c5ad253718b21ab93e).
It preserves the endpoint, voices, audio format, and five-minute time windows.
`drm_v2.go` is under LGPLv3; the original Go source is under MIT. The upstream
license texts are retained in `LICENSE`, `LICENSE.edge-tts-python`, and `COPYING`.

Validation from the repository root:

```sh
go test ./... github.com/bytectlgo/edge-tts/pkg/edge_tts
RAIKIRI_TEST_EDGE_LIVE=1 go test ./internal/app -run TestSynthesizeEdgeLive -count=1 -v
```

The offline tests check the real WebSocket request against a server rejecting
v1 tokens, plus fixed reference vectors generated independently with the
upstream Python implementation. The opt-in test exercises Raikiri's actual
synthesis function against the online service with `es-MX-DaliaNeural`.

Remove the local replacement when the Go upstream supports v2, after running
both checks and verifying `/api/tts/test` delivers playable audio to `/ws/audio`.
