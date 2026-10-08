package steamach

import (
	"bytes"
	"context"
	"io"
	"os"
	"regexp"
	"strings"
	"time"
)

// FFNx logs "[00001234] TRACE: setAchievement - Achievement NAME set, Store
// request sent to Steam" once per unlock. Progress popups log
// OnAchievementStored instead, and repeats log "already achieved, skip".
var ffnxUnlockRE = regexp.MustCompile(`(?i)\bset\w*Achievement\w*\b[^\r\n]*?\bAchievement\s+(\S+)\s+set,\s*Store request sent to Steam`)

// ParseFFNxLine extracts the API name of an achievement unlock from one FFNx
// log line.
func ParseFFNxLine(line string) (string, bool) {
	m := ffnxUnlockRE.FindStringSubmatch(line)
	if m == nil {
		return "", false
	}
	return strings.TrimSpace(m[1]), true
}

// Tailer follows a log file that may not exist yet, is truncated on every game
// launch (fopen "wb") and may be replaced with a new inode.
type Tailer struct {
	Path     string
	Interval time.Duration
	OnLine   func(line string)
}

// Run polls the file until ctx is done. An already-existing file is followed
// from its end; a file that appears or is truncated later is read from byte 0.
func (t *Tailer) Run(ctx context.Context) {
	interval := t.Interval
	if interval <= 0 {
		interval = 250 * time.Millisecond
	}
	var (
		f       *os.File
		info    os.FileInfo
		offset  int64
		partial []byte
		first   = true
		buf     = make([]byte, 64*1024)
	)
	closeFile := func() {
		if f != nil {
			_ = f.Close()
			f = nil
		}
		partial = nil
		offset = 0
	}
	defer closeFile()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		st, err := os.Stat(t.Path)
		if err != nil {
			closeFile()
			first = false // once it reappears every line is new
		} else {
			if f != nil && !os.SameFile(info, st) {
				closeFile()
			}
			if f == nil {
				nf, err := os.Open(t.Path)
				if err == nil {
					f, info = nf, st
					if first {
						offset = st.Size()
					}
					first = false
					_, _ = f.Seek(offset, io.SeekStart)
				}
			} else if st.Size() < offset {
				offset = 0
				partial = nil
				_, _ = f.Seek(0, io.SeekStart)
			}
			if f != nil {
				info = st
				for {
					n, err := f.Read(buf)
					if n > 0 {
						offset += int64(n)
						partial = append(partial, buf[:n]...)
						for {
							i := bytes.IndexByte(partial, '\n')
							if i < 0 {
								break
							}
							line := strings.TrimRight(string(partial[:i]), "\r")
							partial = partial[i+1:]
							if t.OnLine != nil && line != "" {
								t.OnLine(line)
							}
						}
					}
					if err != nil || n == 0 {
						break
					}
				}
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
