package ui

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"io.github.sandwichdoge.vibesandfolders/internal/app"
)

const (
	defaultWindowWidth  = 900
	defaultWindowHeight = 700
	outputTextRows      = 15
	promptTextRows      = 3
)

type MainWindow struct {
	app          fyne.App
	window       fyne.Window
	orchestrator *app.Orchestrator
	config       *app.Config
	logger       *app.Logger

	dirEntry    *widget.Entry
	promptEntry *widget.Entry
	depthSelect *widget.Select
	cleanCheck  *widget.Check
	outputText  *widget.Entry
	statusLabel *widget.Label
	progressBar *widget.ProgressBarInfinite
	executeBtn  *widget.Button
	analyzeBtn  *widget.Button
	rollbackBtn *widget.Button

	lastOutputContent    string
	currentOperations    []app.FileOperation
	lastSuccessfulResults []app.OperationResult
}

func NewMainWindow(fyneApp fyne.App, orchestrator *app.Orchestrator, config *app.Config, logger *app.Logger) *MainWindow {
	mw := &MainWindow{
		app:          fyneApp,
		window:       fyneApp.NewWindow("VibesAndFolders - AI-Powered File Organizer"),
		orchestrator: orchestrator,
		config:       config,
		logger:       logger,
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
	mw.promptEntry.SetMinRowsVisible(promptTextRows)

	mw.depthSelect = widget.NewSelect(
		[]string{"Unlimited", "1 (Root Only)", "2", "3", "4", "5"},
		func(s string) {},
	)
	mw.depthSelect.SetSelected("1 (Root Only)")

	mw.cleanCheck = widget.NewCheck("Clean-up empty directories after execution", func(bool) {})
	mw.cleanCheck.SetChecked(true)

	mw.outputText = widget.NewMultiLineEntry()
	mw.outputText.SetPlaceHolder("Directory structure and AI suggestions will appear here...")
	mw.outputText.Wrapping = fyne.TextWrapWord
	mw.outputText.SetMinRowsVisible(outputTextRows)
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

	mw.rollbackBtn = widget.NewButton("⟲ Undo Changes (Rollback)", mw.onRollback)
	mw.rollbackBtn.Importance = widget.DangerImportance
	mw.rollbackBtn.Hide()

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
		widget.NewLabel("    "),
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
		mw.rollbackBtn,
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
	mw.window.Resize(fyne.NewSize(defaultWindowWidth, defaultWindowHeight))
}

func (mw *MainWindow) setupMenu() {
	settingsMenu := fyne.NewMenu("Settings",
		fyne.NewMenuItem("Configure...", func() {
			configWindow := NewConfigWindow(mw.app, mw.config, mw.logger)
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

func (mw *MainWindow) getRelativePath(basePath, fullPath string) string {
	relPath, err := filepath.Rel(basePath, fullPath)
	if err != nil {
		return fullPath
	}
	return relPath
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
	mw.rollbackBtn.Hide()
	mw.statusLabel.SetText("Streaming analysis...")

	// Initialize output with structure
	// We will append operations to this
	mw.setOutputText("")

	// We need a thread-safe buffer for the UI text to avoid flickering
	var outputBuffer strings.Builder

	go func() {
		req := app.AnalysisRequest{
			DirectoryPath: dirPath,
			UserPrompt:    userPrompt,
			MaxDepth:      maxDepth,
		}

		// 1. Get Structure first (fast)
		structure, _ := mw.orchestrator.GetDirectoryStructure(dirPath, maxDepth)

		fyne.Do(func() {
			outputBuffer.WriteString(fmt.Sprintf("Directory Structure:\n%s\n\n=== AI Suggested Operations ===\n", structure))
			mw.setOutputText(outputBuffer.String())
			mw.statusLabel.SetText(fmt.Sprintf("Thinking via %s...", mw.config.Model))
		})

		// 2. Define the callback
		opCount := 0
		onOperation := func(op app.FileOperation) {
			// This runs in the background thread, need fyne.Do for UI
			fyne.Do(func() {
				opCount++
				fromRel := mw.getRelativePath(mw.dirEntry.Text, op.From)
				toRel := mw.getRelativePath(mw.dirEntry.Text, op.To)

				line := fmt.Sprintf("%s → %s\n", fromRel, toRel)

				// Append to buffer and update UI
				outputBuffer.WriteString(line)
				mw.setOutputText(outputBuffer.String())

				mw.statusLabel.SetText(fmt.Sprintf("Found %d operations...", opCount))
			})
		}

		// 3. Call Orchestrator with callback
		result := mw.orchestrator.AnalyzeDirectory(req, onOperation)

		fyne.Do(func() {
			mw.progressBar.Hide()
			mw.analyzeBtn.Enable()

			if result.Error != nil {
				dialog.ShowError(result.Error, mw.window)
				mw.statusLabel.SetText("Error during analysis")
				return
			}

			if len(result.Operations) == 0 {
				mw.statusLabel.SetText("No changes suggested")
				return
			}

			mw.statusLabel.SetText(fmt.Sprintf("Ready to execute %d operations", len(result.Operations)))
			mw.currentOperations = result.Operations
			mw.executeBtn.Show()
		})
	}()
}

func (mw *MainWindow) onExecute() {
	mw.executeBtn.Hide()
	mw.rollbackBtn.Hide() // Hide during execution

	go func() {
		req := app.ExecutionRequest{
			Operations: mw.currentOperations,
			BasePath:   mw.dirEntry.Text,
			CleanEmpty: mw.cleanCheck.Checked,
		}

		result := mw.orchestrator.ExecuteOrganization(req)

		fyne.Do(func() {
			mw.displayExecutionResult(result, false)
		})
	}()
}

func (mw *MainWindow) onRollback() {
	mw.rollbackBtn.Hide()
	mw.progressBar.Show()
	mw.statusLabel.SetText("Rolling back changes...")

	go func() {
		// Create inverse operations
		// We must iterate backwards to handle chained moves correctly (A->B, B->C reversed is C->B, B->A)
		var inverseOps []app.FileOperation
		for i := len(mw.lastSuccessfulResults) - 1; i >= 0; i-- {
			result := mw.lastSuccessfulResults[i]
			inverseOps = append(inverseOps, app.FileOperation{
				From: result.Operation.To,   // Swap From
				To:   result.Operation.From, // Swap To
			})
		}

		req := app.ExecutionRequest{
			Operations: inverseOps,
			BasePath:   mw.dirEntry.Text,
			CleanEmpty: false, // Usually don't want to clean empty dirs during rollback, or make it optional. Safest is false to ensure structure restoration.
		}

		result := mw.orchestrator.ExecuteOrganization(req)

		fyne.Do(func() {
			mw.progressBar.Hide()
			mw.displayExecutionResult(result, true) // Pass flag indicating this IS a rollback
		})
	}()
}

func (mw *MainWindow) displayExecutionResult(result app.ExecutionResult, isRollback bool) {
	var resultsText strings.Builder
	basePath := mw.dirEntry.Text

	if !isRollback {
		mw.lastSuccessfulResults = []app.OperationResult{}
	}

	title := "Execution Results"
	if isRollback {
		title = "Rollback Results"
	}

	for _, opResult := range result.Operations {
		fromRel := mw.getRelativePath(basePath, opResult.Operation.From)
		toRel := mw.getRelativePath(basePath, opResult.Operation.To)
		if opResult.Success {
			resultsText.WriteString(fmt.Sprintf("✓ [SUCCESS] %s → %s\n", fromRel, toRel))
			if !isRollback {
				mw.lastSuccessfulResults = append(mw.lastSuccessfulResults, opResult)
			}
		} else {
			resultsText.WriteString(fmt.Sprintf("✗ [FAILED] %s → %s\n  Error: %v\n", fromRel, toRel, opResult.Error))
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

	newContent := fmt.Sprintf("=== %s ===\n%s", title, resultsText.String())
	mw.setOutputText(newContent)

	if !isRollback && len(mw.lastSuccessfulResults) > 0 {
		mw.rollbackBtn.Show()
	} else if isRollback && result.FailCount == 0 {
		// If rollback finished successfully, we return to the "Ready to Execute" state
		mw.executeBtn.Show()
		mw.statusLabel.SetText("Rollback Complete. Ready to Execute original plan.")
	}

	if result.FailCount > 0 || !verificationSuccess {
		msgTitle := "Execution Warnings"
		msg := finalStatus + "\n\n" + verificationMsg
		if result.FailCount > 0 {
			msg += "\n\nSome operations failed."
		}
		msg += "\nCheck the output log for details."
		dialog.ShowInformation(msgTitle, msg, mw.window)
	} else {
		// Only show popup success if it's a fresh execution.
		// For rollback, the UI update is sufficient usually, or we can show it too.
		msgTitle := "Success"
		if isRollback {
			msgTitle = "Rollback Successful"
		}
		dialog.ShowInformation(msgTitle, "All operations processed successfully!\n"+verificationMsg, mw.window)
	}
}

func (mw *MainWindow) Show() {
	mw.window.Show()
}

func (mw *MainWindow) ShowAndRun() {
	mw.window.ShowAndRun()
}
