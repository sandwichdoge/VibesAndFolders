package main

import (
	"fyne.io/fyne/v2/app"

	adminapp "io.github.sandwichdoge.vibesandfolders/internal/app"
	"io.github.sandwichdoge.vibesandfolders/internal/ui"
)

func main() {
	myApp := app.NewWithID("io.github.sandwichdoge.vibesandfolders")

	adminapp.LoadConfig(myApp)

	logger := adminapp.NewLogger(true)
	validator := adminapp.NewValidator()
	httpClient := adminapp.NewHTTPClient(logger)

	aiService := adminapp.NewOpenAIAIService(adminapp.GlobalConfig, httpClient, logger)
	fileService := adminapp.NewFileService(validator, logger)

	orchestrator := adminapp.NewOrchestrator(aiService, fileService, validator, logger)

	mainWindow := ui.NewMainWindow(myApp, orchestrator, adminapp.GlobalConfig)

	if adminapp.GlobalConfig.APIKey == adminapp.DefaultAPIKey || adminapp.GlobalConfig.Endpoint == "" {
		configWindow := ui.NewConfigWindow(myApp, adminapp.GlobalConfig)
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
