package app

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
	"unicode"

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
	text = normalizeTTSText(text, 150)
	if ttsContainsBlockedWord(text, cfg.TTSBlockedWords) {
		t.logger.Info("tts skipped because text matched blocked word list")
		return
	}
	if ttsLooksRepetitive(text) {
		t.logger.Info("tts skipped because text looks repetitive")
		return
	}
	if text == "" {
		return
	}
	segments := splitTTSVoiceSegments(text, voice)
	if len(segments) == 0 {
		return
	}
	t.mu.Lock()
	for _, segment := range segments {
		t.queue = append(t.queue, ttsJob{text: segment.Text, voice: segment.Voice, cfg: cfg})
	}
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

func normalizeTTSText(text string, maxRunes int) string {
	text = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(text, "<", ""), ">", ""))
	if maxRunes <= 0 {
		return text
	}
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	return string(runes[:maxRunes])
}

func ttsContainsBlockedWord(text, blockedWords string) bool {
	normalizedText := normalizeTTSFilterText(text)
	paddedText := " " + normalizedText + " "
	for _, blocked := range strings.FieldsFunc(blockedWords, func(r rune) bool {
		return r == '\n' || r == ',' || r == ';'
	}) {
		normalizedBlocked := normalizeTTSFilterText(blocked)
		if normalizedBlocked == "" {
			continue
		}
		if containsCJK(normalizedBlocked) && strings.Contains(normalizedText, normalizedBlocked) {
			return true
		}
		if strings.Contains(paddedText, " "+normalizedBlocked+" ") {
			return true
		}
	}
	return false
}

func normalizeTTSFilterText(value string) string {
	value = strings.ToLower(value)
	var b strings.Builder
	lastWasSpace := true
	for _, r := range value {
		r = normalizeSpanishRune(r)
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastWasSpace = false
			continue
		}
		if !lastWasSpace {
			b.WriteByte(' ')
			lastWasSpace = true
		}
	}
	return strings.TrimSpace(b.String())
}

func normalizeSpanishRune(r rune) rune {
	switch r {
	case 'á', 'à', 'ä', 'â':
		return 'a'
	case 'é', 'è', 'ë', 'ê':
		return 'e'
	case 'í', 'ì', 'ï', 'î':
		return 'i'
	case 'ó', 'ò', 'ö', 'ô':
		return 'o'
	case 'ú', 'ù', 'ü', 'û':
		return 'u'
	default:
		return r
	}
}

func containsCJK(value string) bool {
	for _, r := range value {
		if isCJKRune(r) {
			return true
		}
	}
	return false
}

func isCJKRune(r rune) bool {
	return unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul)
}

func ttsLooksRepetitive(text string) bool {
	normalized := normalizeTTSFilterText(text)
	if normalized == "" {
		return false
	}

	for _, token := range strings.Fields(normalized) {
		if hasLongRepeatedRune(token, 6) || isRepeatedShortPattern(token, 4, 4) || isNoisyRepeatedShortPattern(token, 4) {
			return true
		}
	}

	words := strings.Fields(normalized)
	if len(words) < 4 {
		return false
	}
	repeats := 1
	for i := 1; i < len(words); i++ {
		if words[i] == words[i-1] {
			repeats++
			if repeats >= 4 {
				return true
			}
			continue
		}
		repeats = 1
	}
	return false
}

func hasLongRepeatedRune(token string, maxAllowed int) bool {
	var prev rune
	count := 0
	for _, r := range token {
		if r == prev {
			count++
		} else {
			prev = r
			count = 1
		}
		if count > maxAllowed {
			return true
		}
	}
	return false
}

func isRepeatedShortPattern(token string, maxPatternLen, minRepeats int) bool {
	runes := []rune(token)
	if len(runes) < maxPatternLen*minRepeats {
		return false
	}
	for patternLen := 1; patternLen <= maxPatternLen; patternLen++ {
		if len(runes)%patternLen != 0 || len(runes)/patternLen < minRepeats {
			continue
		}
		matches := true
		for i := patternLen; i < len(runes); i++ {
			if runes[i] != runes[i%patternLen] {
				matches = false
				break
			}
		}
		if matches {
			return true
		}
	}
	return false
}

func isNoisyRepeatedShortPattern(token string, maxPatternLen int) bool {
	runes := []rune(token)
	if len(runes) < 16 {
		return false
	}
	for patternLen := 1; patternLen <= maxPatternLen; patternLen++ {
		matches := 0
		total := 0
		for i := 0; i+patternLen < len(runes); i++ {
			total++
			if runes[i] == runes[i+patternLen] {
				matches++
			}
		}
		if total > 0 && float64(matches)/float64(total) >= 0.72 {
			return true
		}
	}
	return false
}
