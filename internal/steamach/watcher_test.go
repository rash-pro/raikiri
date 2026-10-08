package steamach

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type eventSink struct {
	ch chan Event
}

func newSink() *eventSink { return &eventSink{ch: make(chan Event, 16)} }

func (s *eventSink) next(t *testing.T, within time.Duration) Event {
	t.Helper()
	select {
	case ev := <-s.ch:
		return ev
	case <-time.After(within):
		t.Fatal("no event")
		return Event{}
	}
}

func (s *eventSink) none(t *testing.T, within time.Duration) {
	t.Helper()
	select {
	case ev := <-s.ch:
		t.Fatalf("unexpected event %+v", ev)
	case <-time.After(within):
	}
}

func startWatcher(t *testing.T, root string, ffnx map[string]uint32) (*Watcher, *eventSink) {
	t.Helper()
	sink := newSink()
	w := New(Config{
		SteamRoot: root, FFNxLogs: ffnx,
		Logger:   slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
		Debounce: 20 * time.Millisecond, PollEvery: 20 * time.Millisecond, PollFor: 300 * time.Millisecond,
	}, func(ev Event) { sink.ch <- ev })
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	errc := make(chan error, 1)
	go func() { errc <- w.Run(ctx) }()
	deadline := time.Now().Add(2 * time.Second)
	for w.started.IsZero() && time.Now().Before(deadline) {
		select {
		case err := <-errc:
			t.Fatalf("watcher exited: %v", err)
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}
	time.Sleep(50 * time.Millisecond) // let fsnotify watches settle
	return w, sink
}

func TestWatcherDetectsUnlockFromStatsFile(t *testing.T) {
	const appid = 39150
	root := steamRootFixture(t, appid, userStatsBlob(0b0001, map[uint]int64{0: 1600000000}, nil))
	writeProgress(t, root, appid, 1, 4)
	w, sink := startWatcher(t, root, nil)
	if w.Account() != fixtureAccount {
		t.Fatalf("account resolved to %d", w.Account())
	}
	sink.none(t, 100*time.Millisecond) // startup snapshot is silent

	// Unlock Shiva with a plausible AchievementTimes entry.
	now := time.Now().Unix()
	if err := os.WriteFile(userStatsPath(root, fixtureAccount, appid), userStatsBlob(0b0011, map[uint]int64{0: 1600000000, 1: now}, nil), 0o644); err != nil {
		t.Fatal(err)
	}
	ev := sink.next(t, 2*time.Second)
	if ev.APIName != "UNLOCK_GF_SHIVA" || ev.AppID != appid || ev.Source != "stats" {
		t.Fatalf("event: %+v", ev)
	}
	if ev.Names["spanish"] != "Shiva" || ev.Descs["english"] != "Unlock Guardian Force Shiva" || ev.Game != "FINAL FANTASY FIXTURE" {
		t.Fatalf("event text: %+v", ev)
	}
	if ev.Unlocked != 2 || ev.Total != 4 || ev.Time != now || ev.Icon != IconURL(appid, "bbb.jpg") {
		t.Fatalf("event counters: %+v", ev)
	}

	// The progress file catching up must not fire a second toast.
	writeProgress(t, root, appid, 2, 4)
	sink.none(t, 400*time.Millisecond)

	// Rewriting the same bits (Steam re-saves) is silent; a stale time is replaced by now.
	if err := os.WriteFile(userStatsPath(root, fixtureAccount, appid), userStatsBlob(0b0111, map[uint]int64{0: 1600000000, 1: now, 2: 1500000000}, nil), 0o644); err != nil {
		t.Fatal(err)
	}
	ev = sink.next(t, 2*time.Second)
	if ev.APIName != "CARDGAME_FIRST_TIME" || ev.Time < now {
		t.Fatalf("stale time not replaced: %+v", ev)
	}

	// Reset: bits cleared, silent re-snapshot; the next unlock is detected again.
	if err := os.WriteFile(userStatsPath(root, fixtureAccount, appid), emptyUserStatsBlob(), 0o644); err != nil {
		t.Fatal(err)
	}
	sink.none(t, 300*time.Millisecond)
	if err := os.WriteFile(userStatsPath(root, fixtureAccount, appid), userStatsBlob(0b1000, map[uint]int64{3: now}, nil), 0o644); err != nil {
		t.Fatal(err)
	}
	ev = sink.next(t, 2*time.Second)
	if ev.APIName != "SECRET_ENDING" || !ev.Hidden || ev.Unlocked != 1 {
		t.Fatalf("post-reset event: %+v", ev)
	}

	status, err := w.Status(appid)
	if err != nil {
		t.Fatal(err)
	}
	if status.Unlocked != 1 || status.Total != 4 || len(status.Achievements) != 4 || status.Game != "FINAL FANTASY FIXTURE" {
		t.Fatalf("status: %+v", status)
	}
}

func TestWatcherProgressTriggerFallsBackToLibraryCache(t *testing.T) {
	const appid = 39150
	root := steamRootFixture(t, appid, emptyUserStatsBlob())
	writeProgress(t, root, appid, 0, 4)
	writeLibraryCache(t, root, appid, nil)
	_, sink := startWatcher(t, root, nil)

	// Steam bumps the progress cache but the .bin never changes: after polling,
	// the librarycache json identifies the achievement.
	writeLibraryCache(t, root, appid, map[string]bool{"UNLOCK_GF_QUEZACOTL": true})
	writeProgress(t, root, appid, 1, 4)
	ev := sink.next(t, 3*time.Second)
	if ev.APIName != "UNLOCK_GF_QUEZACOTL" || ev.Source != "librarycache" || ev.Unlocked != 1 || ev.Total != 4 {
		t.Fatalf("fallback event: %+v", ev)
	}
	if ev.Names["spanish"] != "Quetzal" {
		t.Fatalf("fallback should still use schema text: %+v", ev)
	}

	// Nothing identifies the unlock: generic event with the progress counters.
	writeProgress(t, root, appid, 2, 4)
	ev = sink.next(t, 3*time.Second)
	if !ev.Generic || ev.Source != "progress" || ev.Unlocked != 2 || ev.Total != 4 {
		t.Fatalf("generic event: %+v", ev)
	}
}

func TestWatcherFFNxFastPathDedupesStatsFile(t *testing.T) {
	const appid = 39150
	root := steamRootFixture(t, appid, userStatsBlob(0b0001, map[uint]int64{0: 1600000000}, nil))
	writeProgress(t, root, appid, 1, 4)
	logPath := filepath.Join(t.TempDir(), "FFNx.log")
	if err := os.WriteFile(logPath, []byte("[00000001] INFO: FFNx driver\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, sink := startWatcher(t, root, map[string]uint32{logPath: appid})

	f, _ := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0o644)
	_, _ = f.WriteString("[00000002] TRACE: setAchievement - Achievement UNLOCK_GF_SHIVA already achieved, skip\n")
	_, _ = f.WriteString("[00000003] TRACE: setAchievement - Achievement UNLOCK_GF_SHIVA set, Store request sent to Steam\n")
	_, _ = f.WriteString("[00000004] TRACE: OnAchievementStored - UNLOCK_GF_SHIVA\n")
	_ = f.Close()
	ev := sink.next(t, 2*time.Second)
	if ev.APIName != "UNLOCK_GF_SHIVA" || ev.Source != "ffnx" || ev.Unlocked != 2 || ev.Total != 4 || ev.Names["spanish"] != "Shiva" {
		t.Fatalf("ffnx event: %+v", ev)
	}

	// Steam writes the same unlock to the .bin a second later: one toast only.
	if err := os.WriteFile(userStatsPath(root, fixtureAccount, appid), userStatsBlob(0b0011, map[uint]int64{0: 1600000000, 1: time.Now().Unix()}, nil), 0o644); err != nil {
		t.Fatal(err)
	}
	writeProgress(t, root, appid, 2, 4)
	sink.none(t, 500*time.Millisecond)

	// An achievement unknown to the schema still produces a toast.
	f, _ = os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0o644)
	_, _ = f.WriteString("[00000005] TRACE: setAchievement - Achievement BRAND_NEW_THING set, Store request sent to Steam\n")
	_ = f.Close()
	ev = sink.next(t, 2*time.Second)
	if ev.APIName != "BRAND_NEW_THING" || ev.Names["english"] != "Brand New Thing" {
		t.Fatalf("unknown ffnx event: %+v", ev)
	}
}

func TestMakeEventAndApps(t *testing.T) {
	const appid = 39150
	root := steamRootFixture(t, appid, userStatsBlob(0b0001, map[uint]int64{0: 1600000000}, nil))
	writeProgress(t, root, appid, 1, 4)
	w := New(Config{SteamRoot: root}, nil)
	ev, err := w.MakeEvent(appid, "unlock_gf_shiva")
	if err != nil {
		t.Fatal(err)
	}
	if ev.APIName != "UNLOCK_GF_SHIVA" || ev.Source != "test" || ev.Unlocked != 2 || ev.Total != 4 || ev.Game != "FINAL FANTASY FIXTURE" {
		t.Fatalf("test event: %+v", ev)
	}
	if ev, err := w.MakeEvent(appid, ""); err != nil || ev.APIName == "" {
		t.Fatalf("random event: %+v %v", ev, err)
	}
	if _, err := w.MakeEvent(appid, "NOPE"); err == nil {
		t.Fatal("unknown api must error")
	}
	if _, err := w.MakeEvent(999, ""); err == nil {
		t.Fatal("unknown app must error")
	}
	apps, err := w.Apps()
	if err != nil {
		t.Fatal(err)
	}
	if len(apps) != 1 || apps[0].AppID != appid || apps[0].Unlocked != 1 || apps[0].Total != 4 {
		t.Fatalf("apps: %+v", apps)
	}
}
