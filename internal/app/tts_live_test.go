package app

import (
	"context"
	"os"
	"testing"
	"time"
)

// Run explicitly when checking compatibility with the online Edge service:
// RAIKIRI_TEST_EDGE_LIVE=1 go test ./internal/app -run TestSynthesizeEdgeLive -count=1 -v
func TestSynthesizeEdgeLive(t *testing.T) {
	if os.Getenv("RAIKIRI_TEST_EDGE_LIVE") != "1" {
		t.Skip("requires the online Edge TTS service")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	audio, err := synthesizeEdge(ctx, "Esta es una prueba de voz de Raikiri.", "es-MX-DaliaNeural")
	if err != nil {
		t.Fatal(err)
	}
	if len(audio) < 1000 {
		t.Fatalf("expected synthesized MP3 audio, received %d bytes", len(audio))
	}
	t.Logf("received %d bytes of synthesized audio", len(audio))
}
