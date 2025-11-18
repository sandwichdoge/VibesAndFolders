package main

import (
	"fmt"
	"strconv"
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

			// Save the new values to the global config
			adminapp.GlobalConfig.Endpoint = endpointEntry.Text
			adminapp.GlobalConfig.APIKey = apiKeyEntry.Text
			adminapp.GlobalConfig.Model = modelEntry.Text
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

	depthSelect := widget.NewSelect(
		[]string{"Unlimited", "1 (Root Only)", "2", "3", "4", "5"},
		func(s string) {}, // No action needed on select
	)
	depthSelect.SetSelected("Unlimited")

	// Checkbox for cleaning empty directories
	cleanCheck := widget.NewCheck("Clean-up empty directories after execution", func(bool) {})
	cleanCheck.SetChecked(true) // Default to true as it's usually desired

	// Output area
	outputText := widget.NewMultiLineEntry()
	outputText.SetPlaceHolder("Directory structure and AI suggestions will appear here...")
	outputText.Wrapping = fyne.TextWrapWord
	outputText.SetMinRowsVisible(15)

	// Make the output text read-only by reverting any user changes
	var lastOutputContent string
	outputText.OnChanged = func(content string) {
		if content != lastOutputContent {
			outputText.SetText(lastOutputContent)
		}
	}

	// Helper function to update output text (preserves the read-only state AND scrolls)
	setOutputText := func(text string) {
		lastOutputContent = text
		outputText.SetText(text)

		// --- AUTO-SCROLL LOGIC ---
		// Calculate the number of lines to set the cursor to the very end
		lineCount := strings.Count(text, "\n")
		// Move cursor to the last line
		outputText.CursorRow = lineCount + 1
		// Refresh creates the visual update to jump the scrollbar
		outputText.Refresh()
	}

	// Status and progress
	statusLabel := widget.NewLabel("Ready")
	progressBar := widget.NewProgressBarInfinite()
	progressBar.Hide()

	// Execute button
	var executeBtn *widget.Button
	var currentOperations []adminapp.FileOperation

	executeBtn = widget.NewButton("✓ Execute These Operations", func() {
		executeBtn.Hide()
		// Pass setOutputText instead of outputText
		go adminapp.ExecuteOperations(currentOperations, dirEntry.Text, cleanCheck.Checked, statusLabel, setOutputText, myWindow)
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

		// Parse depth
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

		// Show progress and disable controls (UI Thread)
		progressBar.Show()
		analyzeBtn.Disable()
		executeBtn.Hide()
		statusLabel.SetText("Scanning directory...")
		setOutputText("")

		go func() {
			// Get directory structure (Background Thread)
			structure, err := adminapp.GetDirectoryStructure(dirPath, maxDepth)
			if err != nil {
				fyne.Do(func() {
					progressBar.Hide()
					analyzeBtn.Enable()
					dialog.ShowError(fmt.Errorf("failed to scan directory: %v", err), myWindow)
					statusLabel.SetText("Error scanning directory")
				})
				return // Exit goroutine
			}

			// Update UI before long AI call (UI Thread)
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
			// --- End UI Thread update ---
		}() // --- End goroutine ---
	})

	// Top: Inputs
	dirInputRow := container.NewBorder(nil, nil, nil, browseBtn, dirEntry)

	// Group scan options together
	scanOptions := container.NewHBox(
		widget.NewLabel("Scan Depth:"),
		depthSelect,
		widget.NewLabel("   "), // spacer
		cleanCheck,
	)

	topInputs := container.NewVBox(
		widget.NewLabel("Directory Path:"),
		dirInputRow,
		widget.NewLabel("What to do with this directory:"),
		promptEntry,
		scanOptions,
		analyzeBtn,
		widget.NewSeparator(),
		widget.NewLabel("Output:"), // Label for the output box
	)

	// Bottom: Status and Actions
	bottomStatus := container.NewVBox(
		progressBar,
		statusLabel,
		executeBtn,
	)

	// Main Layout: Use Border layout for flexible resizing
	content := container.NewBorder(
		topInputs,    // Top
		bottomStatus, // Bottom
		nil,          // Left
		nil,          // Right
		outputText,   // Center (Directly use the entry, do not wrap in Scroll)
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
