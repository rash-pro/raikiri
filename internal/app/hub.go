package app

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
)

type Hub struct {
	logger  *slog.Logger
	mu      sync.Mutex
	clients map[string]map[*websocket.Conn]struct{}
	writers map[*websocket.Conn]*sync.Mutex
}

func NewHub(logger *slog.Logger) *Hub {
	return &Hub{logger: logger, clients: map[string]map[*websocket.Conn]struct{}{}, writers: map[*websocket.Conn]*sync.Mutex{}}
}

func (h *Hub) Serve(channel string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			h.logger.Warn("websocket accept failed", "channel", channel, "error", err)
			return
		}
		h.add(channel, conn)
		defer h.remove(channel, conn)
		for {
			if _, _, err := conn.Read(r.Context()); err != nil {
				return
			}
		}
	}
}

func (h *Hub) Publish(channel, event string, payload any) {
	msg := map[string]any{"event": event, "data": payload}
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}
	h.mu.Lock()
	conns := make([]*websocket.Conn, 0, len(h.clients[channel]))
	writers := make(map[*websocket.Conn]*sync.Mutex, len(h.clients[channel]))
	for conn := range h.clients[channel] {
		conns = append(conns, conn)
		writers[conn] = h.writers[conn]
	}
	h.mu.Unlock()
	for _, conn := range conns {
		writeMu := writers[conn]
		if writeMu == nil {
			h.remove(channel, conn)
			_ = conn.Close(websocket.StatusGoingAway, "missing writer")
			continue
		}
		writeMu.Lock()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		err := conn.Write(ctx, websocket.MessageText, data)
		cancel()
		writeMu.Unlock()
		if err != nil {
			h.remove(channel, conn)
			_ = conn.Close(websocket.StatusGoingAway, "write failed")
		}
	}
}

func (h *Hub) add(channel string, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.clients[channel] == nil {
		h.clients[channel] = map[*websocket.Conn]struct{}{}
	}
	h.clients[channel][conn] = struct{}{}
	h.writers[conn] = &sync.Mutex{}
}

func (h *Hub) remove(channel string, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.clients[channel], conn)
	delete(h.writers, conn)
}
