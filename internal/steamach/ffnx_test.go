package steamach

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestParseFFNxLine(t *testing.T) {
	cases := map[string]string{
		"[00012345] TRACE: setAchievement - Achievement UNLOCK_GF_SHIVA set, Store request sent to Steam":                    "UNLOCK_GF_SHIVA",
		"[00012345] TRACE: SteamManager::setAchievement - Achievement CARDGAME_FIRST_TIME set,  Store request sent to Steam": "CARDGAME_FIRST_TIME",
		"[00012345] TRACE: setAchievement - Achievement UNLOCK_GF_SHIVA already achieved, skip":                              "",
		"[00012345] TRACE: OnAchievementStored - Achievement UNLOCK_GF_SHIVA stored":                                         "",
		"[00012345] INFO: something else": "",
	}
	for line, want := range cases {
		got, ok := ParseFFNxLine(line)
		if (want != "") != ok || got != want {
			t.Errorf("%q => %q,%v want %q", line, got, ok, want)
		}
	}
}

func TestTailerFollowsTruncationAndRecreation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "FFNx.log")
	if err := os.WriteFile(path, []byte("old line 1\nold line 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var lines []string
	tailer := &Tailer{Path: path, Interval: 10 * time.Millisecond, OnLine: func(l string) {
		mu.Lock()
		lines = append(lines, l)
		mu.Unlock()
	}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go tailer.Run(ctx)

	wait := func(n int) {
		t.Helper()
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			mu.Lock()
			got := len(lines)
			mu.Unlock()
			if got >= n {
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
		mu.Lock()
		defer mu.Unlock()
		t.Fatalf("got %d lines, want %d: %v", len(lines), n, lines)
	}

	time.Sleep(50 * time.Millisecond) // existing content must be skipped
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	_, _ = f.WriteString("appended, partial")
	time.Sleep(30 * time.Millisecond)
	_, _ = f.WriteString(" then complete\r\n")
	_ = f.Close()
	wait(1)

	// Game relaunch: fopen("wb") truncates the same inode.
	f, _ = os.OpenFile(path, os.O_TRUNC|os.O_WRONLY, 0o644)
	time.Sleep(30 * time.Millisecond)
	_, _ = f.WriteString("after truncate\n")
	_ = f.Close()
	wait(2)

	// Replaced with a new inode.
	_ = os.Remove(path)
	time.Sleep(30 * time.Millisecond)
	if err := os.WriteFile(path, []byte("fresh file\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	wait(3)

	mu.Lock()
	defer mu.Unlock()
	want := []string{"appended, partial then complete", "after truncate", "fresh file"}
	for i, w := range want {
		if lines[i] != w {
			t.Fatalf("line %d = %q, want %q (all: %v)", i, lines[i], w, lines)
		}
	}
}

func TestTailerStartsBeforeFileExists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "FFNx.log")
	got := make(chan string, 4)
	tailer := &Tailer{Path: path, Interval: 10 * time.Millisecond, OnLine: func(l string) { got <- l }}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go tailer.Run(ctx)
	time.Sleep(40 * time.Millisecond)
	if err := os.WriteFile(path, []byte("[1] TRACE: setAchievement - Achievement X set, Store request sent to Steam\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	select {
	case line := <-got:
		if api, ok := ParseFFNxLine(line); !ok || api != "X" {
			t.Fatalf("line %q", line)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("line from newly created file never arrived")
	}
}
