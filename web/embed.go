package web

import "embed"

//go:embed dist/browser/*
var StaticFS embed.FS
