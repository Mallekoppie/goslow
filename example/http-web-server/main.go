package main

import (
	"embed"
	"github.com/Mallekoppie/goslow/platform"
)

//go:embed dist/*
var dist embed.FS

func main() {
	//platform.StartHttpServer(Routes)
	platform.StartHttpServerWithHtmlHosting(Routes, dist)
}
