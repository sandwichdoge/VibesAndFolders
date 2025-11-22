package ui

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"io.github.sandwichdoge.vibesandfolders/internal/app"
)

type ConfigWindow struct {
	app    fyne.App
	config *app.Config
	logger *app.Logger
}

func NewConfigWindow(fyneApp fyne.App, config *app.Config, logger *app.Logger) *ConfigWindow {
	return &ConfigWindow{
		app:    fyneApp,
		config: config,
		logger: logger,
	}
}

func (cw *ConfigWindow) Show(onFirstRunSubmit func(), onFirstRunCancel func()) {
	configWin := cw.app.NewWindow("Configuration")
	configWin.Resize(fyne.NewSize(600, 250))

	endpointEntry := widget.NewEntry()
	endpointEntry.SetText(cw.config.Endpoint)
	endpointEntry.SetPlaceHolder("https://api.example.com/v1/chat/completions")

	apiKeyEntry := widget.NewPasswordEntry()
	apiKeyEntry.SetText(cw.config.APIKey)
	apiKeyEntry.SetPlaceHolder("sk-...")

	modelEntry := widget.NewEntry()
	modelEntry.SetText(cw.config.Model)
	modelEntry.SetPlaceHolder("gpt-4o")

	dbPathEntry := widget.NewEntry()
	dbPathEntry.SetText(cw.config.IndexDBPath)
	dbPathEntry.SetPlaceHolder("Path to index database (optional)")

	form := &widget.Form{
		Items: []*widget.FormItem{
			{Text: "Endpoint", Widget: endpointEntry},
			{Text: "API Key", Widget: apiKeyEntry},
			{Text: "Model", Widget: modelEntry},
			{Text: "Index DB Path", Widget: dbPathEntry},
		},
		OnSubmit: func() {
			if strings.TrimSpace(endpointEntry.Text) == "" {
				dialog.ShowError(app.ErrEmptyEndpoint, configWin)
				return
			}

			cw.config.Endpoint = endpointEntry.Text
			cw.config.APIKey = apiKeyEntry.Text
			cw.config.Model = modelEntry.Text
			cw.config.IndexDBPath = dbPathEntry.Text
			app.SaveConfig(cw.app, cw.config, cw.logger)

			dialog.ShowInformation("Saved", "Configuration has been saved.", configWin)
			configWin.Close()

			if onFirstRunSubmit != nil {
				onFirstRunSubmit()
			}
		},
		OnCancel: func() {
			configWin.Close()
			if onFirstRunCancel != nil {
				onFirstRunCancel()
			}
		},
	}

	configWin.SetContent(form)

	if onFirstRunSubmit != nil {
		configWin.ShowAndRun()
	} else {
		configWin.Show()
	}
}
