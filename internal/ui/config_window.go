package ui

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"io.github.sandwichdoge.vibesandfolders/internal/app"
)

type ConfigWindow struct {
	app        fyne.App
	config     *app.Config
	logger     *app.Logger
	httpClient *app.HTTPClient
}

func NewConfigWindow(fyneApp fyne.App, config *app.Config, logger *app.Logger, httpClient *app.HTTPClient) *ConfigWindow {
	return &ConfigWindow{
		app:        fyneApp,
		config:     config,
		logger:     logger,
		httpClient: httpClient,
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

	// Determine the Model label based on Deep Analysis setting
	modelLabel := "Model"
	if cw.config.EnableDeepAnalysis {
		modelLabel = "Model (multimodal)"
	}

	// Create a verification status label
	verifyStatusLabel := widget.NewLabel("")
	verifyStatusLabel.Hide()

	// Create "Verify Multimodal" button (declare first, set callback after)
	var verifyBtn *widget.Button
	verifyBtn = widget.NewButton("Verify Multimodal Support", func() {
		if strings.TrimSpace(endpointEntry.Text) == "" {
			dialog.ShowError(app.ErrEmptyEndpoint, configWin)
			return
		}
		if strings.TrimSpace(modelEntry.Text) == "" {
			dialog.ShowInformation("Info", "Please enter a model name first.", configWin)
			return
		}

		verifyBtn.Disable()
		verifyStatusLabel.SetText("Testing...")
		verifyStatusLabel.Show()

		// Run verification in a goroutine to avoid blocking UI
		go func() {
			isMultimodal, err := cw.httpClient.VerifyMultimodalCapability(
				endpointEntry.Text,
				apiKeyEntry.Text,
				modelEntry.Text,
			)

			// Update UI on main thread using fyne.Do()
			fyne.Do(func() {
				verifyBtn.Enable()

				if err != nil {
					verifyStatusLabel.SetText("❌ Verification failed: " + err.Error())
					cw.logger.Error("Multimodal verification error: %v", err)
				} else if isMultimodal {
					verifyStatusLabel.SetText("✓ Model supports multimodal inputs")
				} else {
					verifyStatusLabel.SetText("✗ Model does not support multimodal inputs")
				}
			})
		}()
	})

	// Create a container for the model entry with the verify button
	modelContainer := container.NewBorder(nil, nil, nil, verifyBtn, modelEntry)

	form := &widget.Form{
		Items: []*widget.FormItem{
			{Text: "Endpoint", Widget: endpointEntry},
			{Text: "API Key", Widget: apiKeyEntry},
			{Text: modelLabel, Widget: modelContainer},
			{Text: "", Widget: verifyStatusLabel},
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
