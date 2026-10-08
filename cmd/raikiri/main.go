package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"raikiri/internal/app"
)

var version = "dev"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Println("raikiri", version)
		return
	}

	// Bare flags (e.g. the installer's "--tray --data-dir ...") mean serve.
	cmd := "serve"
	if len(os.Args) > 1 && !strings.HasPrefix(os.Args[1], "-") {
		cmd = os.Args[1]
	}

	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	host := fs.String("host", getenv("RAIKIRI_HOST", "127.0.0.1"), "host to bind")
	port := fs.Int("port", getenvInt("RAIKIRI_PORT", 30001), "port to bind")
	dataDir := fs.String("data-dir", getenv("RAIKIRI_DATA_DIR", "./data"), "runtime data directory")
	tray := fs.Bool("tray", false, "run from a system tray icon and log to <data-dir>/raikiri.log (Windows)")
	if len(os.Args) > 1 && os.Args[1] == cmd {
		_ = fs.Parse(os.Args[2:])
	} else {
		_ = fs.Parse(os.Args[1:])
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	switch cmd {
	case "serve":
		opts := app.Options{Host: *host, Port: *port, DataDir: *dataDir, Version: version, Logger: logger}
		if *tray {
			runTray(ctx, opts)
			return
		}
		logger.Info("starting raikiri", "version", version)
		if err := app.Serve(ctx, opts); err != nil {
			logger.Error("server stopped", "error", err)
			os.Exit(1)
		}
	case "migrate":
		if err := app.Migrate(*dataDir); err != nil {
			logger.Error("migration failed", "error", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", cmd)
		os.Exit(2)
	}
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func getenvInt(key string, fallback int) int {
	var value int
	if _, err := fmt.Sscanf(os.Getenv(key), "%d", &value); err == nil {
		return value
	}
	return fallback
}
