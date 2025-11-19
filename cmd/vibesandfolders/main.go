package main

import (
	fyneapp "fyne.io/fyne/v2/app"

	"io.github.sandwichdoge.vibesandfolders/internal/app"
	"io.github.sandwichdoge.vibesandfolders/internal/ui"
)

func main() {
	myApp := fyneapp.NewWithID("io.github.sandwichdoge.vibesandfolders")

	logger := app.NewLogger(true)
	config := app.LoadConfig(myApp, logger)

	validator := app.NewValidator()
	httpClient := app.NewHTTPClient(logger)

	aiService := app.NewOpenAIService(config, httpClient, logger)
	fileService := app.NewFileService(validator, logger)

	orchestrator := app.NewOrchestrator(aiService, fileService, validator, logger)

	mainWindow := ui.NewMainWindow(myApp, orchestrator, config, logger)

	if config.APIKey == app.DefaultAPIKey || config.Endpoint == "" {
		configWindow := ui.NewConfigWindow(myApp, config, logger)
		configWindow.Show(
			func() {
				mainWindow.Show()
			},
			func() {
				myApp.Quit()
			},
		)
	} else {
		mainWindow.ShowAndRun()
	}
}
