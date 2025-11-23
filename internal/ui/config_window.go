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
	configWin.Resize(fyne.NewSize(900, 650))

	// General Settings Tab
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

	// Organization Prompt Tab
	systemPromptEntry := widget.NewMultiLineEntry()
	systemPromptEntry.SetText(cw.config.SystemPrompt)
	systemPromptEntry.SetPlaceHolder("Enter system prompt for file organization...")
	systemPromptEntry.Wrapping = fyne.TextWrapWord
	systemPromptEntry.SetMinRowsVisible(20)

	// PDF Analysis Prompt Tab
	pdfPromptEntry := widget.NewMultiLineEntry()
	pdfPromptEntry.SetText(cw.config.PDFAnalysisPrompt)
	pdfPromptEntry.SetPlaceHolder("Enter system prompt for PDF analysis...")
	pdfPromptEntry.Wrapping = fyne.TextWrapWord
	pdfPromptEntry.SetMinRowsVisible(20)

	// Text Analysis Prompt Tab
	textPromptEntry := widget.NewMultiLineEntry()
	textPromptEntry.SetText(cw.config.TextAnalysisPrompt)
	textPromptEntry.SetPlaceHolder("Enter system prompt for text file analysis...")
	textPromptEntry.Wrapping = fyne.TextWrapWord
	textPromptEntry.SetMinRowsVisible(20)

	// Image Analysis Prompt Tab
	imagePromptEntry := widget.NewMultiLineEntry()
	imagePromptEntry.SetText(cw.config.ImageAnalysisPrompt)
	imagePromptEntry.SetPlaceHolder("Enter system prompt for image analysis...")
	imagePromptEntry.Wrapping = fyne.TextWrapWord
	imagePromptEntry.SetMinRowsVisible(20)

	// Ignore Patterns Tab
	ignorePatternsEntry := widget.NewMultiLineEntry()
	ignorePatternsEntry.SetText(cw.config.IgnorePatterns)
	ignorePatternsEntry.SetPlaceHolder("Enter ignore patterns (one per line, # for comments)...")
	ignorePatternsEntry.Wrapping = fyne.TextWrapWord
	ignorePatternsEntry.SetMinRowsVisible(20)

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

	saveBtn := widget.NewButton("Submit", func() {
		if strings.TrimSpace(endpointEntry.Text) == "" {
			dialog.ShowError(app.ErrEmptyEndpoint, configWin)
			return
		}

		cw.config.Endpoint = endpointEntry.Text
		cw.config.APIKey = apiKeyEntry.Text
		cw.config.Model = modelEntry.Text
		cw.config.SystemPrompt = systemPromptEntry.Text
		cw.config.PDFAnalysisPrompt = pdfPromptEntry.Text
		cw.config.TextAnalysisPrompt = textPromptEntry.Text
		cw.config.ImageAnalysisPrompt = imagePromptEntry.Text
		cw.config.IndexDBPath = dbPathEntry.Text
		cw.config.IgnorePatterns = ignorePatternsEntry.Text
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

	// Create General Settings tab
	generalForm := &widget.Form{
		Items: []*widget.FormItem{
			{Text: "Endpoint", Widget: endpointEntry},
			{Text: "API Key", Widget: apiKeyEntry},
			{Text: modelLabel, Widget: modelContainer},
			{Text: "", Widget: verifyStatusLabel},
			{Text: "Index DB Path", Widget: dbPathEntry},
		},
	}
	generalTab := container.NewBorder(generalForm, nil, nil, nil)

	// Create Organization Prompt tab
	orgPromptLabel := widget.NewLabelWithStyle("System Prompt for File Organization:", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	orgPromptScroll := container.NewScroll(systemPromptEntry)
	orgPromptTab := container.NewBorder(orgPromptLabel, nil, nil, nil, orgPromptScroll)

	// Create PDF Analysis Prompt tab
	pdfPromptLabel := widget.NewLabelWithStyle("System Prompt for PDF Analysis:", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	pdfPromptScroll := container.NewScroll(pdfPromptEntry)
	pdfPromptTab := container.NewBorder(pdfPromptLabel, nil, nil, nil, pdfPromptScroll)

	// Create Text Analysis Prompt tab
	textPromptLabel := widget.NewLabelWithStyle("System Prompt for Text/Document Analysis:", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	textPromptScroll := container.NewScroll(textPromptEntry)
	textPromptTab := container.NewBorder(textPromptLabel, nil, nil, nil, textPromptScroll)

	// Create Image Analysis Prompt tab
	imagePromptLabel := widget.NewLabelWithStyle("System Prompt for Image Analysis:", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	imagePromptScroll := container.NewScroll(imagePromptEntry)
	imagePromptTab := container.NewBorder(imagePromptLabel, nil, nil, nil, imagePromptScroll)

	// Create Ignore Patterns tab
	ignorePatternsLabel := widget.NewLabelWithStyle("Ignore Patterns (one per line, similar to .gitignore):", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	ignorePatternsScroll := container.NewScroll(ignorePatternsEntry)
	ignorePatternsTab := container.NewBorder(ignorePatternsLabel, nil, nil, nil, ignorePatternsScroll)

	// Create tabs
	tabs := container.NewAppTabs(
		container.NewTabItem("General", generalTab),
		container.NewTabItem("Organization Prompt", orgPromptTab),
		container.NewTabItem("PDF Analysis", pdfPromptTab),
		container.NewTabItem("Text Analysis", textPromptTab),
		container.NewTabItem("Image Analysis", imagePromptTab),
		container.NewTabItem("Ignore Patterns", ignorePatternsTab),
	)

	buttonBar := container.NewHBox(saveBtn, cancelBtn)

	content := container.NewBorder(
		nil,
		buttonBar,
		nil,
		nil,
		tabs,
	)

	configWin.SetContent(content)

	if onFirstRunSubmit != nil {
		configWin.ShowAndRun()
	} else {
		configWin.Show()
	}
}
