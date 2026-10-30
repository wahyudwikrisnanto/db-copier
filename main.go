package main

import (
	"embed"
	"log"
	"path/filepath"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"db-copier/internal/history"
	"db-copier/internal/store"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	fixSnapEnv()

	s, err := store.New()
	if err != nil {
		log.Fatal(err)
	}
	h, err := history.Open(filepath.Join(s.Dir(), "history"))
	if err != nil {
		log.Fatal(err)
	}
	app := NewApp(s, h)

	err = wails.Run(&options.App{
		Title:     "DB Copier",
		Width:     1100,
		Height:    760,
		MinWidth:  860,
		MinHeight: 560,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 9, G: 9, B: 11, A: 1},
		OnStartup:        app.startup,
		Bind: []interface{}{
			app,
		},
	})
	if err != nil {
		log.Fatal(err)
	}
}
