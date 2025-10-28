package main

// C:\_Projects_Go\AcousticMerge\cmd\AcousticMerge\main.go
// Package: main
// Назначение: Точка входа CLI-утилиты AcousticMerge (цветной UI, 2 прогресс-бара: scan/merge).

import (
	"acousticmerge/internal/app"
	"acousticmerge/internal/ui"
)

func main() {
	cfg, showOnlyHelp := ui.ParseArgsAndSetup()
	if showOnlyHelp {
		return
	}
	app.Run(cfg, ui.API)
}
