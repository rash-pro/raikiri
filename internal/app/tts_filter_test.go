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
