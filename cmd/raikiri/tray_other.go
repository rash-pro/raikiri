//go:build !windows

package main

import (
	"context"
	"os"

	"raikiri/internal/app"
)

// The tray icon is only built for Windows, where the installer launches Raikiri
// without a console; elsewhere --tray just runs the server in the foreground.
func runTray(ctx context.Context, opts app.Options) {
	opts.Logger.Warn("--tray is only supported on Windows; running in the foreground")
	opts.Logger.Info("starting raikiri", "version", version)
	if err := app.Serve(ctx, opts); err != nil {
		opts.Logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}
