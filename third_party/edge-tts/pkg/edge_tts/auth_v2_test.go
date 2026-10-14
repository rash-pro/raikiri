package edge_tts

import (
	"bytes"
	"context"
	"encoding/binary"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestStreamUsesV2Authentication(t *testing.T) {
	audio := []byte{0xff, 0xf3, 0x44, 0xc4}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if query.Get("Sec-MS-GEC-Version") != "2-143.0.3650.75" ||
			!regexp.MustCompile(`^[0-9A-F]{16}-[0-9A-F]{64}$`).MatchString(query.Get("Sec-MS-GEC")) {
			http.Error(w, "Edge requires Sec-MS-GEC v2", http.StatusForbidden)
			return
		}
		upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.Close()
		for range 2 {
			if _, _, err := conn.ReadMessage(); err != nil {
				t.Error(err)
				return
			}
		}
		header := []byte("Path:audio\r\nContent-Type:audio/mpeg\r\n")
		frame := binary.BigEndian.AppendUint16(nil, uint16(len(header)))
		frame = append(frame, header...)
		frame = append(frame, audio...)
		if err := conn.WriteMessage(websocket.BinaryMessage, frame); err != nil {
			t.Error(err)
		}
		if err := conn.WriteMessage(websocket.TextMessage, []byte("Path:turn.end\r\n\r\n{}")); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	comm := NewCommunicate("Prueba de voz", "es-MX-DaliaNeural")
	comm.wsURL = "ws" + strings.TrimPrefix(server.URL, "http") + "?TrustedClientToken=" + TrustedClientToken
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	chunks, err := comm.Stream(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var received []byte
	for chunk := range chunks {
		if chunk.Type == "error" {
			t.Fatalf("synthesis failed: %s", chunk.Data)
		}
		if chunk.Type == "audio" {
			received = append(received, chunk.Data...)
		}
	}
	if !bytes.Equal(received, audio) {
		t.Fatalf("received %x, want %x", received, audio)
	}
}
