package app

import "testing"

func TestTTSContainsBlockedWordMatchesWholeWords(t *testing.T) {
	if !ttsContainsBlockedWord("No leas esta palabra fea.", "palabra fea") {
		t.Fatal("expected blocked phrase to match")
	}
	if ttsContainsBlockedWord("Esto es un putativo ejemplo.", "puta") {
		t.Fatal("blocked word should not match inside another word")
	}
}

func TestTTSContainsBlockedWordNormalizesCaseAndAccents(t *testing.T) {
	if !ttsContainsBlockedWord("No digas IMBÉCIL en voz alta.", "imbecil") {
		t.Fatal("expected accent-insensitive blocked word match")
	}
}

func TestTTSContainsBlockedWordMatchesCJKWithoutSpaces(t *testing.T) {
	if !ttsContainsBlockedWord("これは禁止ワードです", "禁止") {
		t.Fatal("expected CJK blocked phrase to match without word spaces")
	}
	if !ttsContainsBlockedWord("不要说坏话", "坏话") {
		t.Fatal("expected Han blocked phrase to match without word spaces")
	}
	if !ttsContainsBlockedWord("나쁜말하지마", "나쁜말") {
		t.Fatal("expected Hangul blocked phrase to match without word spaces")
	}
}

func TestTTSContainsBlockedWordAllowsEmptyList(t *testing.T) {
	if ttsContainsBlockedWord("cualquier texto", "") {
		t.Fatal("empty blocked word list should not block text")
	}
}

func TestTTSLooksRepetitiveBlocksShortPatternSpam(t *testing.T) {
	cases := []string{
		"owowowowowoowowowowowow",
		"bobobooboboboboboboobobobob",
		"jajajajajajajaja",
	}
	for _, text := range cases {
		if !ttsLooksRepetitive(text) {
			t.Fatalf("expected %q to be detected as repetitive", text)
		}
	}
}

func TestTTSLooksRepetitiveBlocksRepeatedCharactersAndWords(t *testing.T) {
	cases := []string{
		"holaaaaaaaa como estas",
		"hola hola hola hola",
	}
	for _, text := range cases {
		if !ttsLooksRepetitive(text) {
			t.Fatalf("expected %q to be detected as repetitive", text)
		}
	}
}

func TestTTSLooksRepetitiveAllowsNormalText(t *testing.T) {
	if ttsLooksRepetitive("hola como estas, gracias por pasar al stream") {
		t.Fatal("normal text should not be detected as repetitive")
	}
}

func TestNormalizeTTSTextTruncatesByRune(t *testing.T) {
	text := normalizeTTSText("こんにちは世界", 5)
	if text != "こんにちは" {
		t.Fatalf("expected rune-safe truncation, got %q", text)
	}
}

func TestSplitTTSVoiceSegmentsUsesJapaneseVoiceForKanaAndKanji(t *testing.T) {
	segments := splitTTSVoiceSegments("フロントエンド開発者 (thefabi8a)", "es-MX-DaliaNeural")
	want := []ttsVoiceSegment{
		{Text: "フロントエンド開発者", Voice: "ja-JP-NanamiNeural"},
		{Text: "(thefabi8a)", Voice: "es-MX-DaliaNeural"},
	}
	if len(segments) != len(want) {
		t.Fatalf("expected %d segments, got %#v", len(want), segments)
	}
	for i := range want {
		if segments[i] != want[i] {
			t.Fatalf("segment %d = %#v, want %#v", i, segments[i], want[i])
		}
	}
}

func TestSplitTTSVoiceSegmentsUsesChineseAndKoreanVoices(t *testing.T) {
	segments := splitTTSVoiceSegments("hola 开发자 안녕", "es-MX-DaliaNeural")
	want := []ttsVoiceSegment{
		{Text: "hola", Voice: "es-MX-DaliaNeural"},
		{Text: "开发", Voice: "zh-CN-XiaoxiaoNeural"},
		{Text: "자", Voice: "ko-KR-SunHiNeural"},
		{Text: "안녕", Voice: "ko-KR-SunHiNeural"},
	}
	if len(segments) != len(want) {
		t.Fatalf("expected %d segments, got %#v", len(want), segments)
	}
	for i := range want {
		if segments[i] != want[i] {
			t.Fatalf("segment %d = %#v, want %#v", i, segments[i], want[i])
		}
	}
}
