package steamach

import (
	"os"
	"strconv"
	"testing"
)

// TestRealSteamFiles reads this machine's Steam cache read-only and prints what
// the parser sees. It only runs with STEAMACH_REAL=<appid> (e.g. 39150).
func TestRealSteamFiles(t *testing.T) {
	raw := os.Getenv("STEAMACH_REAL")
	if raw == "" {
		t.Skip("set STEAMACH_REAL=<appid> to read the local Steam cache")
	}
	appid, err := strconv.ParseUint(raw, 10, 32)
	if err != nil {
		t.Fatal(err)
	}
	w := New(Config{}, nil)
	if err := w.Init(); err != nil {
		t.Fatal(err)
	}
	t.Logf("root=%s account=%d", w.Root(), w.Account())
	status, err := w.Status(uint32(appid))
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%s (%d): %d/%d unlocked", status.Game, status.AppID, status.Unlocked, status.Total)
	for _, a := range status.Achievements {
		mark := " "
		if a.Unlocked {
			mark = "x"
		}
		t.Logf("[%s] %-28s %-22q es=%q hidden=%v time=%d icon=%s", mark, a.APIName, a.Name("english"), a.Name("spanish"), a.Hidden, a.Time, a.Icon)
	}
	apps, err := w.Apps()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%d apps with schemas", len(apps))
	for _, s := range apps {
		if p, ok := w.progress[s.AppID]; ok && (p.Unlocked != s.Unlocked || p.Total != s.Total) {
			t.Logf("MISMATCH app %d %s: parser %d/%d vs steam progress %d/%d", s.AppID, s.Game, s.Unlocked, s.Total, p.Unlocked, p.Total)
		}
	}
}
