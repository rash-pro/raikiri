package steamach

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseSchema(t *testing.T) {
	s, err := ParseSchema(schemaBlob(39150, ff8Like))
	if err != nil {
		t.Fatal(err)
	}
	if s.AppID != 39150 || s.GameName != "Fixture Fantasy" || s.Version != "7" {
		t.Fatalf("header: %+v", s)
	}
	if s.Total() != len(ff8Like) {
		t.Fatalf("total = %d, want %d (plain int stat must not count)", s.Total(), len(ff8Like))
	}
	q := s.ByAPIName("unlock_gf_quezacotl")
	if q == nil {
		t.Fatal("lookup by api name (case-insensitive) failed")
	}
	if q.Name("english") != "Quezacotl" {
		t.Fatalf("name not trimmed: %q", q.Name("english"))
	}
	if q.Name("spanish") != "Quetzal" || q.Desc("spanish") != "Desbloquea al Guardián de la Fuerza Quetzal" {
		t.Fatalf("spanish: %q / %q", q.Name("spanish"), q.Desc("spanish"))
	}
	if _, ok := q.Names["token"]; ok {
		t.Fatal("token id must not be exposed as a language")
	}
	if q.Name("german") != "Quezacotl" {
		t.Fatalf("missing language should fall back to english, got %q", q.Name("german"))
	}
	if q.StatID != "1" || q.Bit != 0 || q.Icon != "aaa.jpg" || q.IconGray != "gray_aaa.jpg" || q.Hidden {
		t.Fatalf("fields: %+v", q)
	}
	if !s.ByAPIName("SECRET_ENDING").Hidden {
		t.Fatal("hidden flag lost")
	}
	if s.ByBit("1", 2).APIName != "CARDGAME_FIRST_TIME" {
		t.Fatal("lookup by bit failed")
	}
	if s.ByBit("2", 0) != nil || s.ByAPIName("NOPE") != nil {
		t.Fatal("unknown lookups must be nil")
	}
	if IconURL(39150, q.Icon) != "https://shared.steamstatic.com/community_assets/images/apps/39150/aaa.jpg" {
		t.Fatalf("icon url: %s", IconURL(39150, q.Icon))
	}
}

func TestParseUserStatsAndDiff(t *testing.T) {
	s, err := ParseSchema(schemaBlob(1, ff8Like))
	if err != nil {
		t.Fatal(err)
	}
	before, err := ParseUserStats(userStatsBlob(0b0001, map[uint]int64{0: 1700000000}, map[string]int32{"2": 57}))
	if err != nil {
		t.Fatal(err)
	}
	if before.Stats["1"] != 1 || before.Stats["2"] != 57 || before.UnlockTime("1", 0) != 1700000000 {
		t.Fatalf("user stats: %+v", before)
	}
	if n := s.CountUnlocked(before.Stats); n != 1 {
		t.Fatalf("count = %d", n)
	}

	after, err := ParseUserStats(userStatsBlob(0b1011, map[uint]int64{0: 1700000000, 1: 1800000000, 3: 1800000001}, nil))
	if err != nil {
		t.Fatal(err)
	}
	unlocked, cleared := s.Diff(before.Stats, after.Stats)
	if cleared {
		t.Fatal("no bits were cleared")
	}
	if len(unlocked) != 2 || unlocked[0].APIName != "UNLOCK_GF_SHIVA" || unlocked[1].APIName != "SECRET_ENDING" {
		t.Fatalf("unlocked = %+v", unlocked)
	}
	if n := s.CountUnlocked(after.Stats); n != 3 {
		t.Fatalf("count after = %d", n)
	}

	// Reset (SAM style): bits cleared, nothing new.
	empty, _ := ParseUserStats(emptyUserStatsBlob())
	unlocked, cleared = s.Diff(after.Stats, empty.Stats)
	if len(unlocked) != 0 || !cleared {
		t.Fatalf("reset: unlocked=%v cleared=%v", unlocked, cleared)
	}

	// Nil previous snapshot behaves like all-zero.
	unlocked, cleared = s.Diff(nil, before.Stats)
	if len(unlocked) != 1 || cleared {
		t.Fatalf("nil prev: %v %v", unlocked, cleared)
	}
}

func TestParseProgress(t *testing.T) {
	data := []byte(`{"nVersion":3,"mapCache":[[550,{"appid":550,"unlocked":61,"total":101,"percentage":60.4,"all_unlocked":false,"cache_time":1772339069,"vetted":true}],[39150,{"appid":39150,"unlocked":1,"total":45}]]}`)
	m, err := ParseProgress(data)
	if err != nil {
		t.Fatal(err)
	}
	if m[550].Unlocked != 61 || m[550].Total != 101 || m[39150].Unlocked != 1 || m[39150].AppID != 39150 {
		t.Fatalf("progress: %+v", m)
	}
	if _, err := ParseProgress([]byte(`{"nVersion":3,"mapCache":[[550,{"unl`)); err == nil {
		t.Fatal("partial write should fail")
	}
}

func TestParseLibraryCache(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(libraryCacheDir(root, fixtureAccount), 0o755); err != nil {
		t.Fatal(err)
	}
	writeLibraryCache(t, root, 39150, map[string]bool{"UNLOCK_GF_SHIVA": true})
	data, _ := os.ReadFile(libraryCachePath(root, fixtureAccount, 39150))
	list, err := ParseLibraryCache(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != len(ff8Like) {
		t.Fatalf("entries = %d", len(list))
	}
	var shiva *CacheAchievement
	for i := range list {
		if list[i].ID == "UNLOCK_GF_SHIVA" {
			shiva = &list[i]
		}
	}
	if shiva == nil || !shiva.Achieved || shiva.Unlocked != 1800000000 || shiva.Name != "Shiva" {
		t.Fatalf("shiva: %+v", shiva)
	}
}

func TestLibraryFoldersAndManifest(t *testing.T) {
	root := steamRootFixture(t, 39150, nil)
	other := t.TempDir()
	if err := os.MkdirAll(filepath.Join(other, "steamapps"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(other, "steamapps", "appmanifest_39140.acf"), []byte(`"AppState" { "appid" "39140" "name" "FINAL FANTASY VII" "installdir" "FINAL FANTASY VII" }`), 0o644); err != nil {
		t.Fatal(err)
	}
	vdf := `"libraryfolders" { "0" { "path" "` + root + `" } "1" { "path" "` + other + `" } }`
	if err := os.WriteFile(filepath.Join(root, "steamapps", "libraryfolders.vdf"), []byte(vdf), 0o644); err != nil {
		t.Fatal(err)
	}
	libs := LibraryFolders(root)
	if len(libs) != 2 || libs[0] != root || libs[1] != other {
		t.Fatalf("libs = %v", libs)
	}
	m, ok := FindManifest(root, 39140)
	if !ok || m.Name != "FINAL FANTASY VII" || m.InstallPath() != filepath.Join(other, "steamapps", "common", "FINAL FANTASY VII") {
		t.Fatalf("manifest: %+v %v", m, ok)
	}
	if m, ok := FindManifest(root, 39150); !ok || m.Name != "FINAL FANTASY FIXTURE" {
		t.Fatalf("root manifest: %+v %v", m, ok)
	}
	if _, ok := FindManifest(root, 1); ok {
		t.Fatal("unknown app must not resolve")
	}
	if id, err := FindAccountID(root); err != nil || id != fixtureAccount {
		t.Fatalf("account: %d %v", id, err)
	}
}
