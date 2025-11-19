package ui

import (
	"fmt"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"io.github.sandwichdoge.vibesandfolders/internal/app"
)

type MainWindow struct {
	app          fyne.App
	window       fyne.Window
	orchestrator *app.Orchestrator
	config       *app.Config

	dirEntry     *widget.Entry
	promptEntry  *widget.Entry
	depthSelect  *widget.Select
	cleanCheck   *widget.Check
	outputText   *widget.Entry
	statusLabel  *widget.Label
	progressBar  *widget.ProgressBarInfinite
	executeBtn   *widget.Button
	analyzeBtn   *widget.Button

	lastOutputContent  string
	currentOperations  []app.FileOperation
}

func NewMainWindow(fyneApp fyne.App, orchestrator *app.Orchestrator, config *app.Config) *MainWindow {
	mw := &MainWindow{
		app:          fyneApp,
		window:       fyneApp.NewWindow("VibesAndFolders - AI-Powered File Organizer"),
		orchestrator: orchestrator,
		config:       config,
	}

	mw.initializeComponents()
	mw.setupLayout()
	mw.setupMenu()

	return mw
}

func (mw *MainWindow) initializeComponents() {
	mw.dirEntry = widget.NewEntry()
	mw.dirEntry.SetPlaceHolder("Enter directory path (e.g., /home/user/Documents)")

	mw.promptEntry = widget.NewMultiLineEntry()
	mw.promptEntry.SetPlaceHolder("Enter your organization instructions (e.g., 'Organize by file type into folders')")
	mw.promptEntry.SetMinRowsVisible(3)

	mw.depthSelect = widget.NewSelect(
		[]string{"Unlimited", "1 (Root Only)", "2", "3", "4", "5"},
		func(s string) {},
	)
	mw.depthSelect.SetSelected("Unlimited")

	mw.cleanCheck = widget.NewCheck("Clean-up empty directories after execution", func(bool) {})
	mw.cleanCheck.SetChecked(true)

	mw.outputText = widget.NewMultiLineEntry()
	mw.outputText.SetPlaceHolder("Directory structure and AI suggestions will appear here...")
	mw.outputText.Wrapping = fyne.TextWrapWord
	mw.outputText.SetMinRowsVisible(15)
	mw.outputText.OnChanged = func(content string) {
		if content != mw.lastOutputContent {
			mw.outputText.SetText(mw.lastOutputContent)
		}
	}

	mw.statusLabel = widget.NewLabel("Ready")
	mw.progressBar = widget.NewProgressBarInfinite()
	mw.progressBar.Hide()

	mw.executeBtn = widget.NewButton("✓ Execute These Operations", mw.onExecute)
	mw.executeBtn.Hide()

	mw.analyzeBtn = widget.NewButton("Analyze & Get AI Suggestions", mw.onAnalyze)
}

func (mw *MainWindow) setupLayout() {
	browseBtn := widget.NewButton("Browse", func() {
		dialog.ShowFolderOpen(func(uri fyne.ListableURI, err error) {
			if err != nil || uri == nil {
				return
			}
			mw.dirEntry.SetText(uri.Path())
		}, mw.window)
	})

	dirInputRow := container.NewBorder(nil, nil, nil, browseBtn, mw.dirEntry)

	scanOptions := container.NewHBox(
		widget.NewLabel("Scan Depth:"),
		mw.depthSelect,
		widget.NewLabel("   "),
		mw.cleanCheck,
	)

	topInputs := container.NewVBox(
		widget.NewLabel("Directory Path:"),
		dirInputRow,
		widget.NewLabel("What to do with this directory:"),
		mw.promptEntry,
		scanOptions,
		mw.analyzeBtn,
		widget.NewSeparator(),
		widget.NewLabel("Output:"),
	)

	bottomStatus := container.NewVBox(
		mw.progressBar,
		mw.statusLabel,
		mw.executeBtn,
	)

	content := container.NewBorder(
		topInputs,
		bottomStatus,
		nil,
		nil,
		mw.outputText,
	)

	paddedContent := container.NewPadded(content)
	mw.window.SetContent(paddedContent)
	mw.window.Resize(fyne.NewSize(900, 700))
}

func (mw *MainWindow) setupMenu() {
	settingsMenu := fyne.NewMenu("Settings",
		fyne.NewMenuItem("Configure...", func() {
			configWindow := NewConfigWindow(mw.app, mw.config)
			configWindow.Show(nil, nil)
		}),
	)
	mainMenu := fyne.NewMainMenu(settingsMenu)
	mw.window.SetMainMenu(mainMenu)
}

func (mw *MainWindow) setOutputText(text string) {
	mw.lastOutputContent = text
	mw.outputText.SetText(text)

	lineCount := strings.Count(text, "\n")
	mw.outputText.CursorRow = lineCount + 1
	mw.outputText.Refresh()
}

func (mw *MainWindow) parseDepth() (int, error) {
	selectedDepthStr := mw.depthSelect.Selected
	if selectedDepthStr == "Unlimited" {
		return 0, nil
	}
	if selectedDepthStr == "1 (Root Only)" {
		return 1, nil
	}
	return strconv.Atoi(selectedDepthStr)
}

func (mw *MainWindow) onAnalyze() {
	if err := app.NewValidator().ValidateConfig(mw.config); err != nil {
		dialog.ShowError(err, mw.window)
		return
	}

	dirPath := mw.dirEntry.Text
	userPrompt := mw.promptEntry.Text

	if dirPath == "" {
		dialog.ShowError(app.ErrEmptyDirectory, mw.window)
		return
	}

	if userPrompt == "" {
		dialog.ShowError(app.ErrEmptyPrompt, mw.window)
		return
	}

	maxDepth, err := mw.parseDepth()
	if err != nil {
		dialog.ShowError(fmt.Errorf("%w: %v", app.ErrInvalidDepth, err), mw.window)
		return
	}

	mw.progressBar.Show()
	mw.analyzeBtn.Disable()
	mw.executeBtn.Hide()
	mw.statusLabel.SetText("Scanning directory...")
	mw.setOutputText("")

	go func() {
		req := app.AnalysisRequest{
			DirectoryPath: dirPath,
			UserPrompt:    userPrompt,
			MaxDepth:      maxDepth,
		}

		fyne.Do(func() {
			mw.statusLabel.SetText("Requesting analysis from AI, be patient...")
		})

		result := mw.orchestrator.AnalyzeDirectory(req)

		fyne.Do(func() {
			mw.progressBar.Hide()
			mw.analyzeBtn.Enable()

			if result.Error != nil {
				dialog.ShowError(result.Error, mw.window)
				mw.statusLabel.SetText("Error during analysis")
				return
			}

			if len(result.Operations) == 0 {
				mw.setOutputText(fmt.Sprintf("Directory Structure:\n%s\n\nNo reorganization needed or AI returned no operations.", result.Structure))
				mw.statusLabel.SetText("No changes suggested")
				return
			}

			var commandsText strings.Builder
			for _, op := range result.Operations {
				commandsText.WriteString(fmt.Sprintf("%s → %s\n", op.From, op.To))
			}

			mw.setOutputText(fmt.Sprintf("Directory Structure:\n%s\n\n=== AI Suggested Operations (%d) ===\n%s", result.Structure, len(result.Operations), commandsText.String()))
			mw.statusLabel.SetText(fmt.Sprintf("Ready to execute %d operations", len(result.Operations)))

			mw.currentOperations = result.Operations
			mw.executeBtn.Show()
		})
	}()
}

func (mw *MainWindow) onExecute() {
	mw.executeBtn.Hide()

	go func() {
		req := app.ExecutionRequest{
			Operations: mw.currentOperations,
			BasePath:   mw.dirEntry.Text,
			CleanEmpty: mw.cleanCheck.Checked,
		}

		result := mw.orchestrator.ExecuteOrganization(req)

		fyne.Do(func() {
			mw.displayExecutionResult(result)
		})
	}()
}

func (mw *MainWindow) displayExecutionResult(result app.ExecutionResult) {
	var resultsText strings.Builder

	for _, opResult := range result.Operations {
		if opResult.Success {
			resultsText.WriteString(fmt.Sprintf("✓ [SUCCESS] %s → %s\n", opResult.Operation.From, opResult.Operation.To))
		} else {
			resultsText.WriteString(fmt.Sprintf("✗ [FAILED] %s → %s\n  Error: %v\n", opResult.Operation.From, opResult.Operation.To, opResult.Error))
		}
	}

	if result.CleanedDirs > 0 {
		resultsText.WriteString(fmt.Sprintf("\n✨ Cleaned up %d empty directories.\n", result.CleanedDirs))
	}

	verificationMsg := ""
	verificationSuccess := false

	if result.VerificationError != nil {
		verificationMsg = fmt.Sprintf("\n⚠ VERIFICATION ERROR: %v", result.VerificationError)
	} else {
		if result.InitialFileCount == result.FinalFileCount {
			verificationMsg = fmt.Sprintf("\n🛡 VERIFICATION PASSED: File count maintained (%d files).", result.FinalFileCount)
			verificationSuccess = true
		} else {
			diff := result.FinalFileCount - result.InitialFileCount
			verificationMsg = fmt.Sprintf("\n🛑 VERIFICATION WARNING: File count changed! Started with %d, ended with %d (Diff: %+d).", result.InitialFileCount, result.FinalFileCount, diff)
		}
	}
	resultsText.WriteString(verificationMsg)

	finalStatus := fmt.Sprintf("Completed: %d successful, %d failed", result.SuccessCount, result.FailCount)
	mw.statusLabel.SetText(finalStatus)

	newContent := fmt.Sprintf("=== Execution Results ===\n%s", resultsText.String())
	mw.setOutputText(newContent)

	if result.FailCount > 0 || !verificationSuccess {
		title := "Execution Warnings"
		msg := finalStatus + "\n\n" + verificationMsg
		if result.FailCount > 0 {
			msg += "\n\nSome operations failed."
		}
		msg += "\nCheck the output log for details."
		dialog.ShowInformation(title, msg, mw.window)
	} else {
		dialog.ShowInformation("Success", "All operations executed successfully!\n"+verificationMsg, mw.window)
	}
}

func (mw *MainWindow) Show() {
	mw.window.Show()
}

func (mw *MainWindow) ShowAndRun() {
	mw.window.ShowAndRun()
}
