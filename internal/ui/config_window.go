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
	configWin.Resize(fyne.NewSize(800, 700))

	endpointEntry := widget.NewEntry()
	endpointEntry.SetText(cw.config.Endpoint)
	endpointEntry.SetPlaceHolder("https://api.example.com/v1/chat/completions")

	apiKeyEntry := widget.NewPasswordEntry()
	apiKeyEntry.SetText(cw.config.APIKey)
	apiKeyEntry.SetPlaceHolder("sk-...")

	modelEntry := widget.NewEntry()
	modelEntry.SetText(cw.config.Model)
	modelEntry.SetPlaceHolder("gpt-4o")

	systemPromptEntry := widget.NewMultiLineEntry()
	systemPromptEntry.SetText(cw.config.SystemPrompt)
	systemPromptEntry.SetPlaceHolder("Enter custom system prompt for the AI...")
	systemPromptEntry.Wrapping = fyne.TextWrapWord
	systemPromptEntry.SetMinRowsVisible(15)

	saveBtn := widget.NewButton("Submit", func() {
		if strings.TrimSpace(endpointEntry.Text) == "" {
			dialog.ShowError(app.ErrEmptyEndpoint, configWin)
			return
		}

		cw.config.Endpoint = endpointEntry.Text
		cw.config.APIKey = apiKeyEntry.Text
		cw.config.Model = modelEntry.Text
		cw.config.SystemPrompt = systemPromptEntry.Text
		app.SaveConfig(cw.app, cw.config, cw.logger)

		dialog.ShowInformation("Saved", "Configuration has been saved.", configWin)
		configWin.Close()

		if onFirstRunSubmit != nil {
			onFirstRunSubmit()
		}
	})
	saveBtn.Importance = widget.HighImportance

	cancelBtn := widget.NewButton("Cancel", func() {
		configWin.Close()
		if onFirstRunCancel != nil {
			onFirstRunCancel()
		}
	})

	// Create a custom layout with the system prompt taking up most of the space
	topForm := &widget.Form{
		Items: []*widget.FormItem{
			{Text: "Endpoint", Widget: endpointEntry},
			{Text: "API Key", Widget: apiKeyEntry},
			{Text: "Model", Widget: modelEntry},
		},
	}

	systemPromptLabel := widget.NewLabelWithStyle("System Prompt:", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	systemPromptScroll := container.NewScroll(systemPromptEntry)

	buttonBar := container.NewHBox(saveBtn, cancelBtn)

	content := container.NewBorder(
		container.NewVBox(topForm, systemPromptLabel),
		buttonBar,
		nil,
		nil,
		systemPromptScroll,
	)

	configWin.SetContent(content)

	if onFirstRunSubmit != nil {
		configWin.ShowAndRun()
	} else {
		configWin.Show()
	}
}
