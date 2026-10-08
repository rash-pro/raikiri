package steamach

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"
)

// Synthetic binary KeyValues writer used by the tests; mirrors the Steam client format.
type kvw struct{ b []byte }

func (w *kvw) key(t byte, key string) *kvw {
	w.b = append(w.b, t)
	w.b = append(w.b, key...)
	w.b = append(w.b, 0)
	return w
}

func (w *kvw) str(key, val string) *kvw {
	w.key(kvString, key)
	w.b = append(w.b, val...)
	w.b = append(w.b, 0)
	return w
}

func (w *kvw) i32(key string, val int32) *kvw {
	w.key(kvInt32, key)
	w.b = binary.LittleEndian.AppendUint32(w.b, uint32(val))
	return w
}

func (w *kvw) f32(key string, val float32) *kvw {
	w.key(kvFloat32, key)
	w.b = binary.LittleEndian.AppendUint32(w.b, math.Float32bits(val))
	return w
}

func (w *kvw) u64(key string, val uint64) *kvw {
	w.key(kvUint64, key)
	w.b = binary.LittleEndian.AppendUint64(w.b, val)
	return w
}

func (w *kvw) wstr(key, val string) *kvw {
	w.key(kvWString, key)
	for _, r := range val {
		w.b = binary.LittleEndian.AppendUint16(w.b, uint16(r))
	}
	w.b = append(w.b, 0, 0)
	return w
}

func (w *kvw) open(key string) *kvw { return w.key(kvNone, key) }
func (w *kvw) close() *kvw          { w.b = append(w.b, kvEnd); return w }

type fixtureAch struct {
	api, en, es, descEn, descEs, icon string
	hidden                            bool
}

var ff8Like = []fixtureAch{
	{"UNLOCK_GF_QUEZACOTL", "Quezacotl  ", "Quetzal", "Unlock Guardian Force Quezacotl", "Desbloquea al Guardián de la Fuerza Quetzal", "aaa.jpg", false},
	{"UNLOCK_GF_SHIVA", "Shiva", "Shiva", "Unlock Guardian Force Shiva", "Desbloquea a Shiva", "bbb.jpg", false},
	{"CARDGAME_FIRST_TIME", "Card player", "Jugador de cartas", "Play Triple Triad", "Juega al Triple Triad", "ccc.jpg", false},
	{"SECRET_ENDING", "Secret", "Secreto", "???", "???", "ddd.jpg", true},
}

// schemaBlob builds a UserGameStatsSchema_<appid>.bin with one achievement stat
// ("1", bits 0..n-1) and one plain integer stat ("2") without bits.
func schemaBlob(appid uint32, achs []fixtureAch) []byte {
	w := &kvw{}
	w.open(fmt.Sprint(appid))
	w.str("gamename", "Fixture Fantasy")
	w.open("stats")
	w.open("1").i32("type", 4)
	w.open("bits")
	for i, a := range achs {
		w.open(fmt.Sprint(i)).str("name", a.api)
		w.open("display")
		w.open("name").str("english", a.en).str("token", "NEW_ACHIEVEMENT_1_"+fmt.Sprint(i)+"_NAME").str("spanish", a.es).close()
		w.open("desc").str("english", a.descEn).str("spanish", a.descEs).close()
		hidden := int32(0)
		if a.hidden {
			hidden = 1
		}
		w.i32("hidden", hidden).str("icon", a.icon).str("icon_gray", "gray_"+a.icon)
		w.close() // display
		w.close() // bit
	}
	w.close() // bits
	w.close() // stat 1
	w.open("2").i32("type", 1).str("name", "TOTAL_KILLS").i32("incrementonly", 1).close()
	w.close() // stats
	w.str("version", "7")
	w.close() // app
	w.close() // root
	return w.b
}

// userStatsBlob builds a UserGameStats_<acc>_<appid>.bin with stat "1" = bits
// and AchievementTimes for each set bit.
func userStatsBlob(bits uint32, times map[uint]int64, extra map[string]int32) []byte {
	w := &kvw{}
	w.open("cache").i32("crc", 0x1234).i32("PendingChanges", 0)
	w.open("1").i32("data", int32(bits))
	w.open("AchievementTimes")
	for bit, t := range times {
		w.i32(fmt.Sprint(bit), int32(t))
	}
	w.close()
	w.close()
	for k, v := range extra {
		w.open(k).i32("data", v).close()
	}
	w.close() // cache
	w.close() // root
	return w.b
}

func emptyUserStatsBlob() []byte {
	w := &kvw{}
	w.open("cache").i32("crc", 0x1234).i32("PendingChanges", 0).close().close()
	return w.b
}

const fixtureAccount uint32 = 12345678
const fixtureSteamID64 = "76561197972611406"

// steamRootFixture lays out a fake Steam root in a temp dir.
func steamRootFixture(t *testing.T, appid uint32, stats []byte) string {
	t.Helper()
	root := t.TempDir()
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.MkdirAll(filepath.Join(root, "config"), 0o755))
	must(os.MkdirAll(statsDir(root), 0o755))
	must(os.MkdirAll(libraryCacheDir(root, fixtureAccount), 0o755))
	must(os.MkdirAll(filepath.Join(root, "steamapps"), 0o755))
	must(os.WriteFile(filepath.Join(root, "config", "loginusers.vdf"), []byte(`"users"
{
	"`+fixtureSteamID64+`"
	{
		"AccountName"		"rash_pro"
		"PersonaName"		"ラシ"
		"MostRecent"		"1"
		"Timestamp"		"1791340129"
	}
}
`), 0o644))
	must(os.WriteFile(filepath.Join(root, "steamapps", fmt.Sprintf("appmanifest_%d.acf", appid)), []byte(`"AppState"
{
	"appid"		"`+fmt.Sprint(appid)+`"
	"name"		"FINAL FANTASY FIXTURE"
	"installdir"		"FINAL FANTASY FIXTURE"
}
`), 0o644))
	must(os.WriteFile(schemaPath(root, appid), schemaBlob(appid, ff8Like), 0o644))
	if stats != nil {
		must(os.WriteFile(userStatsPath(root, fixtureAccount, appid), stats, 0o644))
	}
	return root
}

func writeProgress(t *testing.T, root string, appid uint32, unlocked, total int) {
	t.Helper()
	body := fmt.Sprintf(`{"nVersion":3,"mapCache":[[%d,{"appid":%d,"unlocked":%d,"total":%d,"percentage":%f,"all_unlocked":false,"cache_time":1700000000,"vetted":true}]]}`,
		appid, appid, unlocked, total, float64(unlocked)*100/float64(total))
	if err := os.WriteFile(progressPath(root, fixtureAccount), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeLibraryCache(t *testing.T, root string, appid uint32, achieved map[string]bool) {
	t.Helper()
	body := `[["achievements",{"version":2,"data":{"vecHighlight":[`
	first := true
	entry := func(a fixtureAch, done bool) string {
		return fmt.Sprintf(`{"strID":%q,"strName":%q,"strDescription":%q,"strImage":"https://img/%s","bAchieved":%v,"rtUnlocked":%d}`,
			a.api, a.en, a.descEn, a.icon, done, map[bool]int{true: 1800000000, false: 0}[done])
	}
	for _, a := range ff8Like {
		if !achieved[a.api] {
			continue
		}
		if !first {
			body += ","
		}
		first = false
		body += entry(a, true)
	}
	body += `],"vecUnachieved":[`
	first = true
	for _, a := range ff8Like {
		if achieved[a.api] {
			continue
		}
		if !first {
			body += ","
		}
		first = false
		body += entry(a, false)
	}
	body += fmt.Sprintf(`],"vecAchievedHidden":[],"nTotal":%d,"nAchieved":%d}}],["descriptions",{}]]`, len(ff8Like), len(achieved))
	if err := os.WriteFile(libraryCachePath(root, fixtureAccount, appid), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
