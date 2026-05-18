package raikiri

import (
	"embed"
	"io/fs"
)

//go:embed src/dashboard/* src/overlays/chat/* src/overlays/alerts/* src/overlays/audio/* src/overlays/widgets/support-goal/* src/overlays/widgets/recent-events/* src/overlays/widgets/custom/* src/shared/*
var assets embed.FS

func sub(path string) fs.FS {
	dir, err := fs.Sub(assets, path)
	if err != nil {
		panic(err)
	}
	return dir
}

func DashboardAssets() fs.FS    { return sub("src/dashboard") }
func ChatAssets() fs.FS         { return sub("src/overlays/chat") }
func AlertsAssets() fs.FS       { return sub("src/overlays/alerts") }
func AudioAssets() fs.FS        { return sub("src/overlays/audio") }
func SupportGoalAssets() fs.FS  { return sub("src/overlays/widgets/support-goal") }
func RecentEventsAssets() fs.FS { return sub("src/overlays/widgets/recent-events") }
func CustomWidgetAssets() fs.FS { return sub("src/overlays/widgets/custom") }
func SharedAssets() fs.FS       { return sub("src/shared") }
