//go:build windows

package main

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"fyne.io/systray"
	"golang.org/x/sys/windows"

	"raikiri/internal/app"
)

//go:embed raikiri.ico
var trayIcon []byte

// runTray is how the Windows installer starts Raikiri: no console window, a tray icon
// to reach the dashboard or quit, and logs in <data-dir>/raikiri.log.
func runTray(ctx context.Context, opts app.Options) {
	dashboard := fmt.Sprintf("http://localhost:%d/dashboard/", opts.Port)
	runtimeURL := fmt.Sprintf("http://127.0.0.1:%d/api/runtime", opts.Port)

	// Launching it again (Start menu, desktop) while it runs just brings up the dashboard.
	if responds(runtimeURL) {
		openURL(dashboard)
		return
	}

	if err := os.MkdirAll(opts.DataDir, 0o755); err != nil {
		fatalBox(fmt.Sprintf("Raikiri can't create its data folder:\n%s\n\n%v", opts.DataDir, err))
		return
	}
	logFile, err := os.OpenFile(filepath.Join(opts.DataDir, "raikiri.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err == nil {
		defer logFile.Close()
		opts.Logger = slog.New(slog.NewTextHandler(logFile, &slog.HandlerOptions{Level: slog.LevelInfo}))
	}
	_, statErr := os.Stat(filepath.Join(opts.DataDir, "raikiri.db"))
	firstRun := errors.Is(statErr, os.ErrNotExist)

	ctx, stop := context.WithCancel(ctx)
	defer stop()
	served := make(chan error, 1)
	go func() {
		opts.Logger.Info("starting raikiri", "version", version, "tray", true)
		served <- app.Serve(ctx, opts)
	}()

	onReady := func() {
		systray.SetIcon(trayIcon)
		systray.SetTooltip("Raikiri " + version)
		title := systray.AddMenuItem("Raikiri "+version, "")
		title.Disable()
		systray.AddSeparator()
		openDash := systray.AddMenuItem("Open dashboard", dashboard)
		openData := systray.AddMenuItem("Open data folder", opts.DataDir)
		systray.AddSeparator()
		quit := systray.AddMenuItem("Quit", "Stop Raikiri")

		if firstRun {
			go func() {
				if waitFor(runtimeURL, 15*time.Second) {
					openURL(dashboard)
				}
			}()
		}

		go func() {
			for {
				select {
				case <-openDash.ClickedCh:
					openURL(dashboard)
				case <-openData.ClickedCh:
					_ = exec.Command("explorer", opts.DataDir).Start()
				case <-quit.ClickedCh:
					systray.Quit()
					return
				case err := <-served:
					if err != nil {
						opts.Logger.Error("server stopped", "error", err)
						fatalBox(fmt.Sprintf("Raikiri stopped:\n%v\n\nIs another program using port %d?", err, opts.Port))
					}
					systray.Quit()
					return
				}
			}
		}()
	}

	// Blocks until systray.Quit; then shut the server down cleanly.
	systray.Run(onReady, func() {})
	stop()
	select {
	case <-served:
	case <-time.After(5 * time.Second):
	}
}

func responds(url string) bool {
	client := http.Client{Timeout: 700 * time.Millisecond}
	res, err := client.Get(url)
	if err != nil {
		return false
	}
	res.Body.Close()
	return res.StatusCode == http.StatusOK
}

func waitFor(url string, limit time.Duration) bool {
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		if responds(url) {
			return true
		}
		time.Sleep(250 * time.Millisecond)
	}
	return false
}

func openURL(url string) {
	_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
}

func fatalBox(text string) {
	t, _ := windows.UTF16PtrFromString(text)
	c, _ := windows.UTF16PtrFromString("Raikiri")
	windows.MessageBox(0, t, c, windows.MB_OK|windows.MB_ICONERROR)
}
