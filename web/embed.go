package web

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed ui/*
var assets embed.FS

func Handler() http.Handler { sub, _ := fs.Sub(assets, "ui"); return http.FileServer(http.FS(sub)) }
