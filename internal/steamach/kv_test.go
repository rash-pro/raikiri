package steamach

import (
	"errors"
	"testing"
)

func TestParseBinaryAllTypes(t *testing.T) {
	w := &kvw{}
	w.open("root").
		str("s", "hello").
		i32("i", -42).
		f32("f", 1.5).
		u64("u", 1<<40).
		wstr("w", "ñandú").
		open("sub").i32("x", 7).close().
		close().close()

	root, err := ParseBinary(w.b)
	if err != nil {
		t.Fatal(err)
	}
	n := root.Child("ROOT") // case-insensitive
	if n == nil {
		t.Fatal("root subtree missing")
	}
	if got := n.Child("s").String(); got != "hello" {
		t.Fatalf("string = %q", got)
	}
	if got := n.Child("i").Int64(); got != -42 {
		t.Fatalf("int32 = %d", got)
	}
	if got := n.Child("f").Float; got != 1.5 {
		t.Fatalf("float = %v", got)
	}
	if got := n.Child("u").Int64(); got != 1<<40 {
		t.Fatalf("uint64 = %d", got)
	}
	if got := n.Child("w").String(); got != "ñandú" {
		t.Fatalf("wstring = %q", got)
	}
	if got := n.Get("sub", "x").Int64(); got != 7 {
		t.Fatalf("nested int = %d", got)
	}
	if n.Get("sub", "missing") != nil || n.Get("nope") != nil {
		t.Fatal("missing paths should be nil")
	}
}

func TestParseBinaryTruncatedAndUnknownType(t *testing.T) {
	w := &kvw{}
	w.open("cache").i32("crc", 1).close().close()
	for cut := 1; cut < len(w.b); cut++ {
		if _, err := ParseBinary(w.b[:cut]); !errors.Is(err, ErrTruncated) {
			// A cut right after a complete top-level list is a valid shorter doc; everything else must be truncated.
			if err == nil && cut == len(w.b)-1 {
				continue
			}
			t.Fatalf("cut at %d: err = %v, want ErrTruncated", cut, err)
		}
	}
	bad := append([]byte{0x09}, "key\x00"...)
	if _, err := ParseBinary(bad); err == nil || errors.Is(err, ErrTruncated) {
		t.Fatalf("unknown type: err = %v", err)
	}
}

func TestParseBinaryRealLayoutEmptyStats(t *testing.T) {
	// Exact bytes of a freshly reset UserGameStats file: cache{crc, PendingChanges}.
	data := []byte("\x00cache\x00\x02crc\x00\x42\x9e\x5d\x15\x02PendingChanges\x00\x00\x00\x00\x00\x08\x08")
	u, err := ParseUserStats(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(u.Stats) != 0 || u.CRC != 0x155d9e42 {
		t.Fatalf("unexpected parse: %+v", u)
	}
}

func TestParseTextAndLoginUsers(t *testing.T) {
	text := `"users"
{
	// comment line
	"76561197972611406"
	{
		"AccountName"		"rash_pro"
		"PersonaName"		"ラシ＿プロ"
		"Timestamp"		"1791340129"
	}
	"76561197960265729"
	{
		"AccountName"		"older"
		"Timestamp"		"100"
	}
}
`
	id, err := AccountFromLoginUsers(text)
	if err != nil {
		t.Fatal(err)
	}
	if id != 12345678 {
		t.Fatalf("account = %d, want 12345678 (newest Timestamp)", id)
	}

	mostRecent := `"users" { "76561197960265729" { "MostRecent" "1" "Timestamp" "1" } "76561197972611406" { "Timestamp" "999999999999" } }`
	id, err = AccountFromLoginUsers(mostRecent)
	if err != nil {
		t.Fatal(err)
	}
	if id != 1 {
		t.Fatalf("account = %d, want MostRecent user 1", id)
	}

	if _, err := AccountFromLoginUsers(`"users" { }`); err == nil {
		t.Fatal("expected error for empty users")
	}
	if _, err := ParseText(`"a" { "b" "c"`); !errors.Is(err, ErrTruncated) {
		t.Fatalf("unterminated block: %v", err)
	}
	if AccountFromSteamID64(76561197972611406) != 12345678 {
		t.Fatal("steamid64 conversion")
	}
}

func TestParseTextEscapes(t *testing.T) {
	root, err := ParseText(`"k" "a \"quoted\" \\ value"  bare token`)
	if err != nil {
		t.Fatal(err)
	}
	if got := root.Child("k").String(); got != `a "quoted" \ value` {
		t.Fatalf("escaped = %q", got)
	}
	if got := root.Child("bare").String(); got != "token" {
		t.Fatalf("bare = %q", got)
	}
}
