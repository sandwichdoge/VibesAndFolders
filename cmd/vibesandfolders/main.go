package main

import (
	"path/filepath"

	fyneapp "fyne.io/fyne/v2/app"

	"io.github.sandwichdoge.vibesandfolders/internal/app"
	"io.github.sandwichdoge.vibesandfolders/internal/ui"
)

func main() {
	myApp := fyneapp.NewWithID("io.github.sandwichdoge.vibesandfolders")

	logger := app.NewLogger(true)
	config := app.LoadConfig(myApp, logger)

	// Set default IndexDBPath if not configured
	if config.IndexDBPath == "" {
		config.IndexDBPath = filepath.Join(myApp.Storage().RootURI().Path(), "index.db")
		app.SaveConfig(myApp, config, logger)
	}

	validator := app.NewValidator()
	httpClient := app.NewHTTPClient(logger)

	aiService := app.NewOpenAIService(config, httpClient, logger)
	fileService := app.NewFileService(validator, logger)

	// Initialize IndexService
	indexService := app.NewIndexService(logger)
	if err := indexService.Initialize(config.IndexDBPath); err != nil {
		logger.Error("Failed to initialize index service: %v", err)
		// Continue without indexing
		indexService = nil
	}

	// Initialize DeepAnalysisService
	var deepAnalysisService *app.DeepAnalysisService
	if indexService != nil {
		deepAnalysisService = app.NewDeepAnalysisService(config, httpClient, indexService, logger)
	}

	orchestrator := app.NewOrchestrator(aiService, fileService, validator, logger, deepAnalysisService, indexService)

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

	// Close indexService on exit
	if indexService != nil {
		indexService.Close()
	}
}
