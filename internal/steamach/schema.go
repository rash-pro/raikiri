package steamach

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Achievement is one entry of a game's stats schema.
type Achievement struct {
	APIName  string            `json:"apiName"`
	StatID   string            `json:"-"`
	Bit      uint              `json:"-"`
	Hidden   bool              `json:"hidden"`
	Icon     string            `json:"icon"`
	IconGray string            `json:"iconGray"`
	Names    map[string]string `json:"names"` // steam language -> display name (trimmed)
	Descs    map[string]string `json:"descs"`
}

// Text returns the display name/description for a Steam language with an
// english fallback, then any language.
func pick(m map[string]string, lang string) string {
	if v := m[lang]; v != "" {
		return v
	}
	if v := m["english"]; v != "" {
		return v
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if k != "token" && m[k] != "" {
			return m[k]
		}
	}
	return ""
}

// Name returns the achievement's name in lang (english fallback).
func (a *Achievement) Name(lang string) string { return pick(a.Names, lang) }

// Desc returns the achievement's description in lang (english fallback).
func (a *Achievement) Desc(lang string) string { return pick(a.Descs, lang) }

// Schema is the parsed UserGameStatsSchema_<appid>.bin.
type Schema struct {
	AppID        uint32
	GameName     string
	Version      string
	Achievements []Achievement
	byAPI        map[string]int
	byBit        map[string]int // "statID:bit"
}

// ParseSchema decodes a UserGameStatsSchema_<appid>.bin blob.
func ParseSchema(data []byte) (*Schema, error) {
	root, err := ParseBinary(data)
	if err != nil {
		return nil, err
	}
	if len(root.Children) == 0 {
		return nil, errors.New("steamach: empty schema")
	}
	app := root.Children[0]
	appid, _ := strconv.ParseUint(app.Key, 10, 32)
	s := &Schema{
		AppID:    uint32(appid),
		GameName: app.Child("gamename").String(),
		Version:  app.Child("version").String(),
		byAPI:    map[string]int{},
		byBit:    map[string]int{},
	}
	stats := app.Child("stats")
	if stats == nil {
		return s, nil
	}
	for _, stat := range stats.Children {
		bits := stat.Child("bits")
		if bits == nil {
			continue
		}
		for _, bitNode := range bits.Children {
			bit, err := strconv.ParseUint(bitNode.Key, 10, 8)
			if err != nil || bit > 31 {
				continue
			}
			ach := Achievement{
				APIName: strings.TrimSpace(bitNode.Child("name").String()),
				StatID:  stat.Key,
				Bit:     uint(bit),
				Names:   map[string]string{},
				Descs:   map[string]string{},
			}
			if display := bitNode.Child("display"); display != nil {
				ach.Hidden = display.Child("hidden").Int64() != 0
				ach.Icon = strings.TrimSpace(display.Child("icon").String())
				ach.IconGray = strings.TrimSpace(display.Child("icon_gray").String())
				collectLangs(display.Child("name"), ach.Names)
				collectLangs(display.Child("desc"), ach.Descs)
			}
			if ach.APIName == "" {
				ach.APIName = fmt.Sprintf("%s_%d", stat.Key, bit)
			}
			s.byAPI[strings.ToUpper(ach.APIName)] = len(s.Achievements)
			s.byBit[bitKey(stat.Key, uint(bit))] = len(s.Achievements)
			s.Achievements = append(s.Achievements, ach)
		}
	}
	return s, nil
}

func collectLangs(node *Node, dst map[string]string) {
	if node == nil {
		return
	}
	if node.Type != kvNone {
		// Some schemas store a single untranslated string.
		if v := strings.TrimSpace(node.String()); v != "" {
			dst["english"] = v
		}
		return
	}
	for _, c := range node.Children {
		if strings.EqualFold(c.Key, "token") {
			continue // localization token id, not a language
		}
		if v := strings.TrimSpace(c.String()); v != "" {
			dst[strings.ToLower(c.Key)] = v
		}
	}
}

func bitKey(statID string, bit uint) string {
	return statID + ":" + strconv.FormatUint(uint64(bit), 10)
}

// ByAPIName looks an achievement up by its API name (case-insensitive).
func (s *Schema) ByAPIName(name string) *Achievement {
	if s == nil {
		return nil
	}
	i, ok := s.byAPI[strings.ToUpper(strings.TrimSpace(name))]
	if !ok {
		return nil
	}
	return &s.Achievements[i]
}

// ByBit looks an achievement up by stat id and bit.
func (s *Schema) ByBit(statID string, bit uint) *Achievement {
	if s == nil {
		return nil
	}
	i, ok := s.byBit[bitKey(statID, bit)]
	if !ok {
		return nil
	}
	return &s.Achievements[i]
}

// Total is the number of achievements in the schema.
func (s *Schema) Total() int {
	if s == nil {
		return 0
	}
	return len(s.Achievements)
}

// UserStats is the parsed UserGameStats_<account>_<appid>.bin.
type UserStats struct {
	Stats map[string]uint32         // statID -> packed data
	Times map[string]map[uint]int64 // statID -> bit -> unlock unix time
	CRC   int64
}

// ParseUserStats decodes a UserGameStats_<account>_<appid>.bin blob.
func ParseUserStats(data []byte) (*UserStats, error) {
	root, err := ParseBinary(data)
	if err != nil {
		return nil, err
	}
	cache := root.Child("cache")
	if cache == nil {
		if len(root.Children) == 0 {
			return nil, errors.New("steamach: empty user stats")
		}
		cache = root.Children[0]
	}
	u := &UserStats{Stats: map[string]uint32{}, Times: map[string]map[uint]int64{}, CRC: cache.Child("crc").Int64()}
	for _, stat := range cache.Children {
		if stat.Type != kvNone {
			continue
		}
		if _, err := strconv.Atoi(stat.Key); err != nil {
			continue
		}
		dataNode := stat.Child("data")
		if dataNode == nil {
			continue
		}
		u.Stats[stat.Key] = uint32(dataNode.Int64())
		if times := stat.Child("AchievementTimes"); times != nil {
			m := map[uint]int64{}
			for _, t := range times.Children {
				bit, err := strconv.ParseUint(t.Key, 10, 8)
				if err != nil {
					continue
				}
				m[uint(bit)] = t.Int64()
			}
			u.Times[stat.Key] = m
		}
	}
	return u, nil
}

// IsSet reports whether bit of statID is set in stats.
func IsSet(stats map[string]uint32, statID string, bit uint) bool {
	return (stats[statID]>>bit)&1 == 1
}

// UnlockTime returns the recorded unlock time (0 if unknown).
func (u *UserStats) UnlockTime(statID string, bit uint) int64 {
	if u == nil {
		return 0
	}
	return u.Times[statID][bit]
}

// CountUnlocked counts schema achievements whose bit is set in stats.
func (s *Schema) CountUnlocked(stats map[string]uint32) int {
	n := 0
	for i := range s.Achievements {
		a := &s.Achievements[i]
		if IsSet(stats, a.StatID, a.Bit) {
			n++
		}
	}
	return n
}

// Diff returns the achievements set in next but not in prev, and whether any
// previously set achievement bit was cleared (an achievements reset).
func (s *Schema) Diff(prev, next map[string]uint32) (unlocked []*Achievement, cleared bool) {
	for i := range s.Achievements {
		a := &s.Achievements[i]
		was := IsSet(prev, a.StatID, a.Bit)
		now := IsSet(next, a.StatID, a.Bit)
		switch {
		case now && !was:
			unlocked = append(unlocked, a)
		case was && !now:
			cleared = true
		}
	}
	return unlocked, cleared
}
