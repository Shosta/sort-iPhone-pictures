package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"

	"sort-iphone-pictures/internal/appicon"
	"sort-iphone-pictures/internal/desktop"
)

func main() {
	application := app.NewWithID("local.photosort.desktop")
	application.SetIcon(fyne.NewStaticResource("photosort.png", appicon.PNG))
	application.Settings().SetTheme(desktop.Theme())
	desktop.NewWindow(application).ShowAndRun()
}
