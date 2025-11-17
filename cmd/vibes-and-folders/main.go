package main

import (
	"fmt"
	"strconv" // <-- ADDED IMPORT
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	adminapp "io.github.sandwichdoge.vibesandfolders/internal/app"
)

// showConfigWindow creates and displays a new window for editing configuration.
// This function remains in main.go as it's purely UI-related.
func showConfigWindow(a fyne.App, onFirstRunSubmit func(), onFirstRunCancel func()) {
	configWin := a.NewWindow("Configuration")
	configWin.Resize(fyne.NewSize(600, 200))

	endpointEntry := widget.NewEntry()
	// Use the aliased package
	endpointEntry.SetText(adminapp.GlobalConfig.Endpoint)
	endpointEntry.SetPlaceHolder("https://api.example.com/v1/chat/completions")

	apiKeyEntry := widget.NewPasswordEntry() // Use PasswordEntry for sensitive data
	apiKeyEntry.SetText(adminapp.GlobalConfig.APIKey)
	apiKeyEntry.SetPlaceHolder("sk-...")

	modelEntry := widget.NewEntry()
	modelEntry.SetText(adminapp.GlobalConfig.Model)
	modelEntry.SetPlaceHolder("gpt-4o")

	form := &widget.Form{
		Items: []*widget.FormItem{
			{Text: "Endpoint", Widget: endpointEntry},
			{Text: "API Key", Widget: apiKeyEntry},
			{Text: "Model", Widget: modelEntry},
		},
		OnSubmit: func() {
			if strings.TrimSpace(endpointEntry.Text) == "" {
				dialog.ShowError(fmt.Errorf("endpoint field cannot be empty"), configWin)
				return // Stop submission
			}

			// Save the new values to the exported config
			adminapp.GlobalConfig.Endpoint = endpointEntry.Text
			adminapp.GlobalConfig.APIKey = apiKeyEntry.Text
			adminapp.GlobalConfig.Model = modelEntry.Text
			// Use the aliased package
			adminapp.SaveConfig(a)

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
		// This is the first run. Start the app loop
		// showing ONLY the config window.
		configWin.ShowAndRun()
	} else {
		// This is not the first run (called from settings menu).
		// The app loop is already running, just show the window.
		configWin.Show()
	}
}

func main() {
	myApp := app.NewWithID("io.github.sandwichdoge.vibesandfolders")
	myWindow := myApp.NewWindow("VibesAndFolders - AI-Powered File Organizer")

	adminapp.LoadConfig(myApp)

	myWindow.Resize(fyne.NewSize(900, 700))

	// Create the main menu
	settingsMenu := fyne.NewMenu("Settings",
		fyne.NewMenuItem("Configure...", func() {
			// App is already running, so we don't need first-run logic
			showConfigWindow(myApp, nil, nil)
		}),
	)
	mainMenu := fyne.NewMainMenu(settingsMenu)
	myWindow.SetMainMenu(mainMenu)

	// Input fields
	dirEntry := widget.NewEntry()
	dirEntry.SetPlaceHolder("Enter directory path (e.g., /home/user/Documents)")

	promptEntry := widget.NewMultiLineEntry()
	promptEntry.SetPlaceHolder("Enter your organization instructions (e.g., 'Organize by file type into folders')")
	promptEntry.SetMinRowsVisible(3)

	// --- NEW WIDGET ---
	depthSelect := widget.NewSelect(
		[]string{"Unlimited", "1 (Root Only)", "2", "3", "4", "5"},
		func(s string) {}, // No action needed on select
	)
	depthSelect.SetSelected("Unlimited")
	// --- END NEW WIDGET ---

	// Output area - make it taller
	outputText := widget.NewMultiLineEntry()
	outputText.SetPlaceHolder("Directory structure and AI suggestions will appear here...")
	outputText.Wrapping = fyne.TextWrapWord
	outputText.SetMinRowsVisible(15) // Still good for setting an initial size

	// Make the output text read-only by reverting any user changes
	var lastOutputContent string
	outputText.OnChanged = func(content string) {
		if content != lastOutputContent {
			outputText.SetText(lastOutputContent)
		}
	}

	// Status and progress
	statusLabel := widget.NewLabel("Ready")
	progressBar := widget.NewProgressBarInfinite()
	progressBar.Hide()

	// Execute button (initially hidden)
	var executeBtn *widget.Button
	// Use the aliased package
	var currentOperations []adminapp.FileOperation

	executeBtn = widget.NewButton("✓ Execute These Operations", func() {
		executeBtn.Hide()
		// Use the aliased package
		go adminapp.ExecuteOperations(currentOperations, dirEntry.Text, statusLabel, outputText, myWindow)
	})
	executeBtn.Hide()

	// Browse button
	browseBtn := widget.NewButton("Browse", func() {
		dialog.ShowFolderOpen(func(uri fyne.ListableURI, err error) {
			if err != nil || uri == nil {
				return
			}
			dirEntry.SetText(uri.Path())
		}, myWindow)
	})

	// Helper function to update output text (preserves the read-only state)
	setOutputText := func(text string) {
		lastOutputContent = text
		outputText.SetText(text)
	}

	// Analyze button
	var analyzeBtn *widget.Button
	analyzeBtn = widget.NewButton("Analyze & Get AI Suggestions", func() {
		if adminapp.GlobalConfig.Endpoint == "" || adminapp.GlobalConfig.APIKey == "" || adminapp.GlobalConfig.APIKey == adminapp.DefaultAPIKey {
			dialog.ShowError(fmt.Errorf("please configure your AI Endpoint and API Key in the 'Settings -> Configure...' menu first"), myWindow)
			return
		}

		dirPath := dirEntry.Text
		userPrompt := promptEntry.Text

		if dirPath == "" {
			dialog.ShowError(fmt.Errorf("please enter a directory path"), myWindow)
			return
		}

		if userPrompt == "" {
			dialog.ShowError(fmt.Errorf("please enter organization instructions"), myWindow)
			return
		}

		// --- PARSE DEPTH ---
		selectedDepthStr := depthSelect.Selected
		var maxDepth int
		if selectedDepthStr == "Unlimited" {
			maxDepth = 0 // Use 0 to signify unlimited
		} else if selectedDepthStr == "1 (Root Only)" {
			maxDepth = 1
		} else {
			// It's "2", "3", etc.
			parsedDepth, err := strconv.Atoi(selectedDepthStr)
			if err != nil {
				// This shouldn't happen with a Select, but good to check.
				dialog.ShowError(fmt.Errorf("invalid depth selected: %v", err), myWindow)
				return
			}
			maxDepth = parsedDepth
		}
		// --- END PARSE DEPTH ---

		// Show progress and disable controls (Main Thread)
		progressBar.Show()
		analyzeBtn.Disable()
		executeBtn.Hide()
		statusLabel.SetText("Scanning directory...")
		setOutputText("")

		go func() {
			// Get directory structure (Background Thread)
			// --- UPDATED FUNCTION CALL ---
			structure, err := adminapp.GetDirectoryStructure(dirPath, maxDepth)
			// --- END UPDATED CALL ---
			if err != nil {
				fyne.Do(func() {
					progressBar.Hide()
					analyzeBtn.Enable()
					dialog.ShowError(fmt.Errorf("failed to scan directory: %v", err), myWindow)
					statusLabel.SetText("Error scanning directory")
				})
				return // Exit goroutine
			}

			// Update UI before long AI call (Main Thread)
			fyne.Do(func() {
				statusLabel.SetText("Requesting analysis from AI, be patient...")
				setOutputText(fmt.Sprintf("Directory Structure:\n%s\n\nWaiting for AI response...", structure))
			})

			// Call AI (Background Thread)
			operations, aiErr := adminapp.GetAISuggestions(structure, userPrompt, dirPath)

			fyne.Do(func() {
				// Hide progress and re-enable controls
				progressBar.Hide()
				analyzeBtn.Enable()

				if aiErr != nil {
					dialog.ShowError(fmt.Errorf("failed to get AI suggestions: %v", aiErr), myWindow)
					statusLabel.SetText("Error getting AI suggestions")
					return
				}

				if len(operations) == 0 {
					setOutputText(fmt.Sprintf("Directory Structure:\n%s\n\nNo reorganization needed or AI returned no operations.", structure))
					statusLabel.SetText("No changes suggested")
					return
				}

				// Display suggestions and show execute button
				var commandsText strings.Builder
				for _, op := range operations {
					commandsText.WriteString(fmt.Sprintf("%s → %s\n", op.From, op.To))
				}

				setOutputText(fmt.Sprintf("Directory Structure:\n%s\n\n=== AI Suggested Operations (%d) ===\n%s", structure, len(operations), commandsText.String()))
				statusLabel.SetText(fmt.Sprintf("Ready to execute %d operations", len(operations)))

				// Show execute button for user confirmation
				currentOperations = operations
				executeBtn.Show()
			})
			// --- End Main Thread UI update ---
		}() // --- End goroutine ---
	})

	// Top: Inputs
	dirInputRow := container.NewBorder(nil, nil, nil, browseBtn, dirEntry)
	topInputs := container.NewVBox(
		widget.NewLabel("Directory Path:"),
		dirInputRow,
		widget.NewLabel("What to do with this directory:"),
		promptEntry,
		// --- ADDED DEPTH ROW ---
		container.NewBorder(nil, nil, widget.NewLabel("Scan Depth:"), nil, depthSelect),
		// --- END ADDED ROW ---
		analyzeBtn,
		widget.NewSeparator(),
		widget.NewLabel("Output:"), // Label for the output box
	)

	// Center: Output (wrapped in a scroll container)
	scrollableOutput := container.NewScroll(outputText)

	// Bottom: Status and Actions
	bottomStatus := container.NewVBox(
		progressBar,
		statusLabel,
		executeBtn,
	)

	// Main Layout: Use Border layout for flexible resizing
	content := container.NewBorder(
		topInputs,        // Top
		bottomStatus,     // Bottom
		nil,              // Left
		nil,              // Right
		scrollableOutput, // Center (will expand to fill remaining space)
	)

	paddedContent := container.NewPadded(content)

	myWindow.SetContent(paddedContent)

	if adminapp.GlobalConfig.APIKey == adminapp.DefaultAPIKey || adminapp.GlobalConfig.Endpoint == "" {
		showConfigWindow(myApp,
			func() {
				myWindow.Show()
			},
			func() {
				// OnCancel: Quit app.
				myApp.Quit()
			},
		)
	} else {
		// Config is already set, just show the main window.
		myWindow.ShowAndRun()
	}
}
