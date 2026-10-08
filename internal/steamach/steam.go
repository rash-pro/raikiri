package steamach

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const steamID64Base = 76561197960265728

// DefaultRoot returns the first Steam installation root found for this user.
func DefaultRoot() string {
	home, _ := os.UserHomeDir()
	candidates := []string{
		filepath.Join(home, ".local", "share", "Steam"),
		filepath.Join(home, ".steam", "steam"),
		filepath.Join(home, ".steam", "root"),
		filepath.Join(home, ".var", "app", "com.valvesoftware.Steam", ".local", "share", "Steam"),
	}
	for _, c := range candidates {
		if _, err := os.Stat(filepath.Join(c, "config", "loginusers.vdf")); err == nil {
			return c
		}
	}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && st.IsDir() {
			return c
		}
	}
	return ""
}

// AccountFromSteamID64 converts a 64-bit SteamID to the 32-bit account id used
// in userdata/ and UserGameStats file names.
func AccountFromSteamID64(id uint64) uint32 {
	if id < steamID64Base {
		return uint32(id)
	}
	return uint32(id - steamID64Base)
}

// FindAccountID picks the most recent user from config/loginusers.vdf, falling
// back to the single numeric directory under userdata/.
func FindAccountID(root string) (uint32, error) {
	if data, err := os.ReadFile(filepath.Join(root, "config", "loginusers.vdf")); err == nil {
		if id, err := AccountFromLoginUsers(string(data)); err == nil {
			return id, nil
		}
	}
	entries, err := os.ReadDir(filepath.Join(root, "userdata"))
	if err != nil {
		return 0, fmt.Errorf("steamach: no loginusers.vdf and no userdata in %s", root)
	}
	var ids []uint64
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		id, err := strconv.ParseUint(e.Name(), 10, 32)
		if err == nil && id != 0 {
			ids = append(ids, id)
		}
	}
	if len(ids) != 1 {
		return 0, fmt.Errorf("steamach: cannot pick account id among %d userdata folders", len(ids))
	}
	return uint32(ids[0]), nil
}

// AccountFromLoginUsers parses loginusers.vdf text and returns the account id
// of the MostRecent user, else the one with the newest Timestamp.
func AccountFromLoginUsers(text string) (uint32, error) {
	root, err := ParseText(text)
	if err != nil {
		return 0, err
	}
	users := root.Child("users")
	if users == nil {
		return 0, errors.New("steamach: loginusers.vdf has no users block")
	}
	var best *Node
	var bestTS int64 = -1
	for _, u := range users.Children {
		if u.Child("MostRecent").Int64() == 1 {
			best = u
			break
		}
		if ts := u.Child("Timestamp").Int64(); ts > bestTS {
			best, bestTS = u, ts
		}
	}
	if best == nil {
		return 0, errors.New("steamach: loginusers.vdf has no users")
	}
	id64, err := strconv.ParseUint(best.Key, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("steamach: bad steamid %q", best.Key)
	}
	return AccountFromSteamID64(id64), nil
}

// LibraryFolders lists Steam library roots (each containing steamapps/).
func LibraryFolders(root string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(p string) {
		p = filepath.Clean(p)
		if p == "." || seen[p] {
			return
		}
		seen[p] = true
		out = append(out, p)
	}
	add(root)
	for _, file := range []string{"steamapps/libraryfolders.vdf", "config/libraryfolders.vdf"} {
		data, err := os.ReadFile(filepath.Join(root, file))
		if err != nil {
			continue
		}
		node, err := ParseText(string(data))
		if err != nil {
			continue
		}
		for _, top := range node.Children {
			for _, lib := range top.Children {
				if lib.Type == kvNone {
					if p := lib.Child("path").String(); p != "" {
						add(p)
					}
				} else if _, err := strconv.Atoi(lib.Key); err == nil && lib.Str != "" {
					add(lib.Str) // legacy format: "1" "/path"
				}
			}
		}
	}
	return out
}

// Manifest is the useful part of steamapps/appmanifest_<appid>.acf.
type Manifest struct {
	AppID      uint32
	Name       string
	InstallDir string
	Library    string // library root containing steamapps/
}

// FindManifest searches all libraries for an app's manifest.
func FindManifest(root string, appid uint32) (Manifest, bool) {
	for _, lib := range LibraryFolders(root) {
		path := filepath.Join(lib, "steamapps", fmt.Sprintf("appmanifest_%d.acf", appid))
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		node, err := ParseText(string(data))
		if err != nil || len(node.Children) == 0 {
			continue
		}
		state := node.Children[0]
		return Manifest{
			AppID:      appid,
			Name:       strings.TrimSpace(state.Child("name").String()),
			InstallDir: strings.TrimSpace(state.Child("installdir").String()),
			Library:    lib,
		}, true
	}
	return Manifest{}, false
}

// InstallPath returns <library>/steamapps/common/<installdir> for an app.
func (m Manifest) InstallPath() string {
	if m.InstallDir == "" {
		return ""
	}
	return filepath.Join(m.Library, "steamapps", "common", m.InstallDir)
}

// Progress is one entry of librarycache/achievement_progress.json.
type Progress struct {
	AppID     uint32  `json:"appid"`
	Unlocked  int     `json:"unlocked"`
	Total     int     `json:"total"`
	Percent   float64 `json:"percentage"`
	CacheTime int64   `json:"cache_time"`
}

// ParseProgress decodes achievement_progress.json.
func ParseProgress(data []byte) (map[uint32]Progress, error) {
	var doc struct {
		MapCache [][]json.RawMessage `json:"mapCache"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	out := make(map[uint32]Progress, len(doc.MapCache))
	for _, pair := range doc.MapCache {
		if len(pair) != 2 {
			continue
		}
		var appid uint32
		if err := json.Unmarshal(pair[0], &appid); err != nil {
			continue
		}
		var p Progress
		if err := json.Unmarshal(pair[1], &p); err != nil {
			continue
		}
		if p.AppID == 0 {
			p.AppID = appid
		}
		out[appid] = p
	}
	return out, nil
}

// CacheAchievement is an entry of librarycache/<appid>.json "achievements".
type CacheAchievement struct {
	ID       string `json:"strID"`
	Name     string `json:"strName"`
	Desc     string `json:"strDescription"`
	Image    string `json:"strImage"`
	Achieved bool   `json:"bAchieved"`
	Unlocked int64  `json:"rtUnlocked"`
}

// ParseLibraryCache decodes the achievements section of librarycache/<appid>.json.
func ParseLibraryCache(data []byte) ([]CacheAchievement, error) {
	var sections [][]json.RawMessage
	if err := json.Unmarshal(data, &sections); err != nil {
		return nil, err
	}
	for _, pair := range sections {
		if len(pair) != 2 {
			continue
		}
		var key string
		if json.Unmarshal(pair[0], &key) != nil || key != "achievements" {
			continue
		}
		var body struct {
			Data struct {
				Highlight  []CacheAchievement `json:"vecHighlight"`
				Unachieved []CacheAchievement `json:"vecUnachieved"`
				HiddenDone []CacheAchievement `json:"vecAchievedHidden"`
			} `json:"data"`
		}
		if err := json.Unmarshal(pair[1], &body); err != nil {
			return nil, err
		}
		seen := map[string]bool{}
		var out []CacheAchievement
		for _, list := range [][]CacheAchievement{body.Data.Highlight, body.Data.HiddenDone, body.Data.Unachieved} {
			for _, a := range list {
				if a.ID == "" || seen[a.ID] {
					continue
				}
				seen[a.ID] = true
				a.Name = strings.TrimSpace(a.Name)
				a.Desc = strings.TrimSpace(a.Desc)
				out = append(out, a)
			}
		}
		sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
		return out, nil
	}
	return nil, errors.New("steamach: no achievements section")
}

// IconURL builds the public URL of an achievement icon.
func IconURL(appid uint32, icon string) string {
	icon = strings.TrimSpace(icon)
	if icon == "" {
		return ""
	}
	if strings.HasPrefix(icon, "http://") || strings.HasPrefix(icon, "https://") {
		return icon
	}
	return fmt.Sprintf("https://shared.steamstatic.com/community_assets/images/apps/%d/%s", appid, icon)
}

func progressPath(root string, account uint32) string {
	return filepath.Join(libraryCacheDir(root, account), "achievement_progress.json")
}

func libraryCacheDir(root string, account uint32) string {
	return filepath.Join(root, "userdata", strconv.FormatUint(uint64(account), 10), "config", "librarycache")
}

func libraryCachePath(root string, account uint32, appid uint32) string {
	return filepath.Join(libraryCacheDir(root, account), fmt.Sprintf("%d.json", appid))
}

func statsDir(root string) string { return filepath.Join(root, "appcache", "stats") }

func userStatsPath(root string, account, appid uint32) string {
	return filepath.Join(statsDir(root), fmt.Sprintf("UserGameStats_%d_%d.bin", account, appid))
}

func schemaPath(root string, appid uint32) string {
	return filepath.Join(statsDir(root), fmt.Sprintf("UserGameStatsSchema_%d.bin", appid))
}
