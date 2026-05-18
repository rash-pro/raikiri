package raikiri

import (
	"embed"
	"io/fs"
)

//go:embed src/dashboard/* src/overlays/chat/* src/overlays/alerts/* src/overlays/audio/* src/shared/*
var assets embed.FS

func sub(path string) fs.FS {
	dir, err := fs.Sub(assets, path)
	if err != nil {
		panic(err)
	}
	return dir
}

func DashboardAssets() fs.FS { return sub("src/dashboard") }
func ChatAssets() fs.FS      { return sub("src/overlays/chat") }
func AlertsAssets() fs.FS    { return sub("src/overlays/alerts") }
func AudioAssets() fs.FS     { return sub("src/overlays/audio") }
func SharedAssets() fs.FS    { return sub("src/shared") }
