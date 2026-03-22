package main

import (
	"catana/internal/ui"
	"log"
	"os"

	"gioui.org/app"
	"gioui.org/unit"
)

func main() {
	workspace, _ := os.Getwd()
	if len(os.Args) > 1 {
		workspace = os.Args[1]
	}

	catanaApp := ui.NewCatanaApp(workspace)

	go func() {
		w := new(app.Window)
		w.Option(
			app.Title("Catana"),
			app.Size(unit.Dp(1440), unit.Dp(900)),
			app.MinSize(unit.Dp(800), unit.Dp(600)),
		)

		if err := catanaApp.Run(w); err != nil {
			log.Fatal(err)
		}
		catanaApp.Stop()
		os.Exit(0)
	}()

	app.Main()
}
