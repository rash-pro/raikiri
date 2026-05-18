package app

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	edge_tts "github.com/bytectlgo/edge-tts/pkg/edge_tts"
)

type AudioPayload struct {
	ID     string `json:"id"`
	Buffer []byte `json:"buffer"`
	Volume int    `json:"volume"`
}

type TTSEngine struct {
	logger *slog.Logger
	emit   func(AudioPayload)
	mu     sync.Mutex
	queue  []ttsJob
	busy   bool
}

type ttsJob struct {
	text  string
	voice string
	cfg   AppConfig
}

func NewTTSEngine(logger *slog.Logger, emit func(AudioPayload)) *TTSEngine {
	return &TTSEngine{logger: logger, emit: emit}
}

func (t *TTSEngine) Enqueue(ctx context.Context, text string, cfg AppConfig) {
	t.enqueue(ctx, text, cfg.TTSVoice, cfg)
}

func (t *TTSEngine) EnqueueEvent(ctx context.Context, evt Event, cfg AppConfig) {
	conf, ok := cfg.AlertsConfig[evt.Type]
	if ok && !conf.Enabled {
		return
	}
	text := conf.MessageTemplate
	if text == "" {
		text = "{user}: {message}"
	}
	repl := map[string]string{
		"{user}": fmt.Sprint(evt.User), "{amount}": fmt.Sprint(evt.Amount), "{count}": fmt.Sprint(evt.Count),
		"{tier}": fmt.Sprint(evt.Tier), "{message}": evt.Message,
	}
	for key, val := range repl {
		text = strings.ReplaceAll(text, key, val)
	}
	voice := cfg.TTSVoice
	if conf.Voice != "" {
		voice = conf.Voice
	}
	t.enqueue(ctx, text, voice, cfg)
}

func (t *TTSEngine) enqueue(ctx context.Context, text, voice string, cfg AppConfig) {
	if !cfg.TTSEnabled {
		return
	}
	text = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(text, "<", ""), ">", ""))
	if len(text) > 150 {
		text = text[:150]
	}
	if text == "" {
		return
	}
	t.mu.Lock()
	t.queue = append(t.queue, ttsJob{text: text, voice: voice, cfg: cfg})
	if !t.busy {
		t.busy = true
		go t.run()
	}
	t.mu.Unlock()
	_ = ctx
}

func (t *TTSEngine) run() {
	for {
		t.mu.Lock()
		if len(t.queue) == 0 {
			t.busy = false
			t.mu.Unlock()
			return
		}
		job := t.queue[0]
		t.queue = t.queue[1:]
		t.mu.Unlock()

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		audio, err := synthesizeEdge(ctx, job.text, job.voice)
		cancel()
		if err != nil {
			t.logger.Warn("tts synthesis failed", "error", err)
		} else {
			t.emit(AudioPayload{ID: fmt.Sprint(time.Now().UnixNano()), Buffer: audio, Volume: job.cfg.AudioVol})
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func synthesizeEdge(ctx context.Context, text, voice string) ([]byte, error) {
	if voice == "" {
		voice = "es-MX-DaliaNeural"
	}
	comm := edge_tts.NewCommunicate(text, voice)
	chunks, err := comm.Stream(ctx)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	for chunk := range chunks {
		switch {
		case strings.EqualFold(chunk.Type, "audio"):
			buf.Write(chunk.Data)
		case strings.EqualFold(chunk.Type, "error"):
			return nil, fmt.Errorf("edge tts stream error: %s", string(chunk.Data))
		}
	}
	if buf.Len() == 0 {
		return nil, fmt.Errorf("edge tts returned no audio")
	}
	return buf.Bytes(), nil
}
