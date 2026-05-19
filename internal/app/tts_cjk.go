package app

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

type ttsVoiceSegment struct {
	Text  string
	Voice string
}

type ttsScript int

const (
	ttsScriptDefault ttsScript = iota
	ttsScriptJapanese
	ttsScriptChinese
	ttsScriptKorean
)

func splitTTSVoiceSegments(text, defaultVoice string) []ttsVoiceSegment {
	defaultVoice = strings.TrimSpace(defaultVoice)
	if defaultVoice == "" {
		defaultVoice = "es-MX-DaliaNeural"
	}

	var segments []ttsVoiceSegment
	for len(text) > 0 {
		r, size := utf8.DecodeRuneInString(text)
		if size == 0 {
			break
		}
		if !isCJKRune(r) {
			end := size
			for end < len(text) {
				next, nextSize := utf8.DecodeRuneInString(text[end:])
				if isCJKRune(next) {
					break
				}
				end += nextSize
			}
			segments = appendTTSSegment(segments, text[:end], defaultVoice)
			text = text[end:]
			continue
		}

		end := size
		script := cjkScriptForRune(r)
		hasKana := unicode.In(r, unicode.Hiragana, unicode.Katakana)
		for end < len(text) {
			next, nextSize := utf8.DecodeRuneInString(text[end:])
			if !isCJKRune(next) {
				break
			}
			nextScript := cjkScriptForRune(next)
			if nextScript == ttsScriptKorean || script == ttsScriptKorean {
				if nextScript != script {
					break
				}
			}
			if unicode.In(next, unicode.Hiragana, unicode.Katakana) {
				hasKana = true
			}
			if script == ttsScriptChinese && nextScript == ttsScriptJapanese {
				script = ttsScriptJapanese
			}
			end += nextSize
		}
		if hasKana {
			script = ttsScriptJapanese
		}
		segments = appendTTSSegment(segments, text[:end], voiceForTTSScript(script, defaultVoice))
		text = text[end:]
	}

	return trimEmptyTTSSegments(segments)
}

func appendTTSSegment(segments []ttsVoiceSegment, text, voice string) []ttsVoiceSegment {
	if text == "" {
		return segments
	}
	if len(segments) > 0 && segments[len(segments)-1].Voice == voice {
		segments[len(segments)-1].Text += text
		return segments
	}
	return append(segments, ttsVoiceSegment{Text: text, Voice: voice})
}

func trimEmptyTTSSegments(segments []ttsVoiceSegment) []ttsVoiceSegment {
	var trimmed []ttsVoiceSegment
	for _, segment := range segments {
		text := strings.TrimSpace(segment.Text)
		if text == "" {
			continue
		}
		segment.Text = text
		trimmed = append(trimmed, segment)
	}
	return trimmed
}

func cjkScriptForRune(r rune) ttsScript {
	switch {
	case unicode.In(r, unicode.Hangul):
		return ttsScriptKorean
	case unicode.In(r, unicode.Hiragana, unicode.Katakana):
		return ttsScriptJapanese
	case unicode.In(r, unicode.Han):
		return ttsScriptChinese
	default:
		return ttsScriptDefault
	}
}

func voiceForTTSScript(script ttsScript, defaultVoice string) string {
	switch script {
	case ttsScriptJapanese:
		return "ja-JP-NanamiNeural"
	case ttsScriptChinese:
		return "zh-CN-XiaoxiaoNeural"
	case ttsScriptKorean:
		return "ko-KR-SunHiNeural"
	default:
		return defaultVoice
	}
}
