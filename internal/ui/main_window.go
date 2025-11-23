package ui

import (
	"fmt"
	"os"
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
	httpClient   *app.HTTPClient

	dirEntry          *widget.Entry
	promptEntry       *widget.Entry
	depthSelect       *widget.Select
	cleanCheck        *widget.Check
	deepAnalysisCheck *widget.Check
	viewIndexBtn      *widget.Button
	deleteIndexBtn    *widget.Button
	indexDetailsBox   *fyne.Container
	outputText        *widget.Entry
	statusLabel       *widget.Label
	progressBar       *widget.ProgressBarInfinite
	executeBtn        *widget.Button
	analyzeBtn        *widget.Button
	rollbackBtn       *widget.Button
	bottomStatus      *fyne.Container

	lastOutputContent     string
	currentOperations     []app.FileOperation
	lastSuccessfulResults []app.OperationResult
}

func NewMainWindow(fyneApp fyne.App, orchestrator *app.Orchestrator, config *app.Config, logger *app.Logger, httpClient *app.HTTPClient) *MainWindow {
	mw := &MainWindow{
		app:          fyneApp,
		window:       fyneApp.NewWindow("VibesAndFolders - AI-Powered File Organizer"),
		orchestrator: orchestrator,
		config:       config,
		logger:       logger,
		httpClient:   httpClient,
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

	mw.depthSelect = widget.NewSelect([]string{"Unlimited", "1 (Root Only)", "2", "3", "4", "5"}, nil)
	mw.depthSelect.SetSelected("1 (Root Only)")

	mw.cleanCheck = widget.NewCheck("Clean-up empty directories after execution", nil)
	mw.cleanCheck.SetChecked(true)

	mw.viewIndexBtn = widget.NewButton("View Index", mw.onViewIndexDetails)
	mw.deleteIndexBtn = widget.NewButton("Clear Index", mw.onDeleteIndex)

	mw.indexDetailsBox = container.NewHBox(mw.viewIndexBtn, mw.deleteIndexBtn)
	mw.indexDetailsBox.Hidden = !mw.config.EnableDeepAnalysis

	mw.deepAnalysisCheck = widget.NewCheck("Enable Deep Analysis (PDFs, images, docs, sheets, slides content indexing)", func(checked bool) {
		mw.config.EnableDeepAnalysis = checked
		app.SaveConfig(mw.app, mw.config, mw.logger)
		mw.updateIndexDetailsVisibility()
	})
	mw.deepAnalysisCheck.SetChecked(mw.config.EnableDeepAnalysis)

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

	mw.rollbackBtn = widget.NewButton("↶ Undo Changes (Rollback)", mw.onRollback)
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

	topInputs := container.NewVBox(
		widget.NewLabel("Directory Path:"),
		container.NewBorder(nil, nil, nil, browseBtn, mw.dirEntry),
		widget.NewLabel("What to do with this directory:"),
		mw.promptEntry,
		container.NewVBox(
			container.NewHBox(widget.NewLabel("Scan Depth:"), mw.depthSelect),
			mw.cleanCheck,
			mw.deepAnalysisCheck,
			mw.indexDetailsBox,
		),
		mw.analyzeBtn,
		widget.NewSeparator(),
		widget.NewLabel("Output:"),
	)

	mw.bottomStatus = container.NewVBox(
		mw.progressBar,
		mw.statusLabel,
		mw.executeBtn,
		mw.rollbackBtn,
	)

	mw.window.SetContent(container.NewPadded(
		container.NewBorder(topInputs, mw.bottomStatus, nil, nil, mw.outputText),
	))
	mw.window.Resize(fyne.NewSize(defaultWindowWidth, defaultWindowHeight))
}

func (mw *MainWindow) setupMenu() {
	settingsMenu := fyne.NewMenu("Settings",
		fyne.NewMenuItem("Configure", func() {
			configWindow := NewConfigWindow(mw.app, mw.config, mw.logger, mw.httpClient)
			configWindow.Show(nil, nil)
		}),
		fyne.NewMenuItem("About", mw.showAboutDialog),
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

func (mw *MainWindow) refreshBottomStatus() {
	if mw.bottomStatus != nil {
		mw.bottomStatus.Refresh()
	}
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
	mw.refreshBottomStatus()
	mw.statusLabel.SetText("Analyzing directory...")

	mw.setOutputText("")
	var outputBuffer strings.Builder

	go func() {
		req := app.AnalysisRequest{
			DirectoryPath:      dirPath,
			UserPrompt:         userPrompt,
			MaxDepth:           maxDepth,
			EnableDeepAnalysis: mw.config.EnableDeepAnalysis,
		}

		structure, _ := mw.orchestrator.GetDirectoryStructure(dirPath, maxDepth)
		fyne.Do(func() {
			outputBuffer.WriteString(fmt.Sprintf("Directory Structure:\n%s\n\n=== AI Suggested Operations ===\n", structure))
			mw.setOutputText(outputBuffer.String())
			mw.statusLabel.SetText(fmt.Sprintf("Analyzing with %s...", mw.config.Model))
		})

		opCount := 0
		onOperation := func(op app.FileOperation) {
			fyne.Do(func() {
				opCount++
				fromRel := mw.getRelativePath(mw.dirEntry.Text, op.From)
				toRel := mw.getRelativePath(mw.dirEntry.Text, op.To)
				outputBuffer.WriteString(fmt.Sprintf("%s → %s\n", fromRel, toRel))
				mw.setOutputText(outputBuffer.String())
				mw.statusLabel.SetText(fmt.Sprintf("Found %d operations...", opCount))
			})
		}

		result := mw.orchestrator.AnalyzeDirectory(req, onOperation)

		fyne.Do(func() {
			mw.progressBar.Hide()
			mw.analyzeBtn.Enable()
			mw.refreshBottomStatus()

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
			mw.refreshBottomStatus()
		})
	}()
}

func (mw *MainWindow) onExecute() {
	mw.executeBtn.Hide()
	mw.rollbackBtn.Hide()
	mw.refreshBottomStatus()

	go func() {
		result := mw.orchestrator.ExecuteOrganization(app.ExecutionRequest{
			Operations: mw.currentOperations,
			BasePath:   mw.dirEntry.Text,
			CleanEmpty: mw.cleanCheck.Checked,
		})
		fyne.Do(func() { mw.displayExecutionResult(result, false) })
	}()
}

func (mw *MainWindow) onRollback() {
	mw.rollbackBtn.Hide()
	mw.progressBar.Show()
	mw.refreshBottomStatus()
	mw.statusLabel.SetText("Rolling back changes...")

	go func() {
		var inverseOps []app.FileOperation
		for i := len(mw.lastSuccessfulResults) - 1; i >= 0; i-- {
			result := mw.lastSuccessfulResults[i]
			inverseOps = append(inverseOps, app.FileOperation{
				From: result.Operation.To,
				To:   result.Operation.From,
			})
		}

		result := mw.orchestrator.ExecuteOrganization(app.ExecutionRequest{
			Operations: inverseOps,
			BasePath:   mw.dirEntry.Text,
			CleanEmpty: false,
		})

		dirsToRemove := make(map[string]bool)
		for i := len(mw.lastSuccessfulResults) - 1; i >= 0; i-- {
			for _, dir := range mw.lastSuccessfulResults[i].CreatedDirs {
				dirsToRemove[dir] = true
			}
		}

		var dirList []string
		for dir := range dirsToRemove {
			dirList = append(dirList, dir)
		}
		for i := 0; i < len(dirList); i++ {
			for j := i + 1; j < len(dirList); j++ {
				if len(dirList[j]) > len(dirList[i]) {
					dirList[i], dirList[j] = dirList[j], dirList[i]
				}
			}
		}

		removedCount := 0
		for _, dir := range dirList {
			if err := os.Remove(dir); err == nil {
				removedCount++
				mw.logger.Debug("Removed directory during rollback: %s", dir)
			}
		}

		if removedCount > 0 {
			result.CleanedDirs = removedCount
		}

		fyne.Do(func() {
			mw.progressBar.Hide()
			mw.refreshBottomStatus()
			mw.displayExecutionResult(result, true)
		})
	}()
}

func (mw *MainWindow) displayExecutionResult(result app.ExecutionResult, isRollback bool) {
	var resultsText strings.Builder
	basePath := mw.dirEntry.Text

	if !isRollback {
		mw.lastSuccessfulResults = []app.OperationResult{}
	}

	title := map[bool]string{false: "Execution Results", true: "Rollback Results"}[isRollback]

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
		mw.refreshBottomStatus()
	} else if isRollback && result.FailCount == 0 {
		// If rollback finished successfully, we return to the "Ready to Execute" state
		mw.executeBtn.Show()
		mw.refreshBottomStatus()
		mw.statusLabel.SetText("Rollback Complete. Ready to Execute original plan.")
	}

	if result.FailCount > 0 || !verificationSuccess {
		msg := finalStatus + "\n\n" + verificationMsg
		if result.FailCount > 0 {
			msg += "\n\nSome operations failed."
		}
		msg += "\nCheck the output log for details."
		dialog.ShowInformation("Execution Warnings", msg, mw.window)
	} else {
		msgTitle := map[bool]string{false: "Success", true: "Rollback Successful"}[isRollback]
		dialog.ShowInformation(msgTitle, "All operations processed successfully!\n"+verificationMsg, mw.window)
	}
}

func (mw *MainWindow) updateIndexDetailsVisibility() {
	mw.indexDetailsBox.Hidden = !mw.config.EnableDeepAnalysis
	mw.indexDetailsBox.Refresh()
}

func (mw *MainWindow) onViewIndexDetails() {
	if mw.dirEntry.Text == "" {
		dialog.ShowError(app.ErrEmptyDirectory, mw.window)
		return
	}

	if mw.orchestrator == nil {
		dialog.ShowError(fmt.Errorf("orchestrator not initialized"), mw.window)
		return
	}

	// Check if there are any indexed files before opening the window
	stats, err := mw.orchestrator.GetDirectoryIndexStats(mw.dirEntry.Text)
	if err != nil {
		dialog.ShowError(fmt.Errorf("failed to get index statistics: %w", err), mw.window)
		return
	}

	if stats["total"] == 0 {
		dialog.ShowInformation("No Index", "There are no indexed files for this directory yet.\n\nIndexing will happen automatically on the first analysis.", mw.window)
		return
	}

	// Open the detailed index window
	detailsWindow := NewIndexDetailsWindow(mw.app, mw.orchestrator, mw.logger, mw.dirEntry.Text)
	detailsWindow.Show()
}

func (mw *MainWindow) onDeleteIndex() {
	if mw.dirEntry.Text == "" {
		dialog.ShowError(app.ErrEmptyDirectory, mw.window)
		return
	}

	if mw.orchestrator == nil {
		dialog.ShowError(fmt.Errorf("orchestrator not initialized"), mw.window)
		return
	}

	// Get current stats to show in confirmation dialog
	stats, err := mw.orchestrator.GetDirectoryIndexStats(mw.dirEntry.Text)
	if err != nil {
		dialog.ShowError(fmt.Errorf("failed to get index statistics: %w", err), mw.window)
		return
	}

	totalIndexed := stats["total"]
	if totalIndexed == 0 {
		dialog.ShowInformation("No Index", "There are no indexed files for this directory.", mw.window)
		return
	}

	confirmMsg := fmt.Sprintf("Are you sure you want to delete the index for this directory?\n\nThis will remove %d indexed files from the database.\n\nPath: %s", totalIndexed, mw.dirEntry.Text)
	dialog.ShowConfirm("Confirm Delete Index", confirmMsg, func(confirmed bool) {
		if !confirmed {
			return
		}

		go func() {
			fyne.Do(func() {
				mw.progressBar.Show()
				mw.refreshBottomStatus()
				mw.statusLabel.SetText("Deleting index...")
			})

			deleted, err := mw.orchestrator.DeleteDirectoryIndex(mw.dirEntry.Text)

			fyne.Do(func() {
				mw.progressBar.Hide()
				mw.refreshBottomStatus()
				mw.statusLabel.SetText("Ready")
			})

			// Show dialog with custom callback to refresh window after close
			if err != nil {
				fyne.Do(func() {
					mw.statusLabel.SetText("Error deleting index")
					d := dialog.NewError(fmt.Errorf("failed to delete index: %w", err), mw.window)
					d.SetOnClosed(func() {
						mw.window.Canvas().Refresh(mw.window.Content())
					})
					d.Show()
				})
			} else {
				fyne.Do(func() {
					d := dialog.NewInformation("Index Deleted", fmt.Sprintf("Successfully deleted %d indexed files.", deleted), mw.window)
					d.SetOnClosed(func() {
						// Force full window refresh after dialog closes to prevent coordinate desync
						mw.window.Canvas().Refresh(mw.window.Content())
					})
					d.Show()
				})
			}
		}()
	}, mw.window)
}

func (mw *MainWindow) showAboutDialog() {
	version := mw.app.Metadata().Version
	if version == "" {
		version = "dev" // Fallback for development builds (go run)
	}

	aboutText := fmt.Sprintf(`VibesAndFolders
Version %s

An AI-powered desktop tool for organizing your files based on plain English instructions.

Author: sandwichdoge
GitHub: github.com/sandwichdoge/vibesandfolders`, version)

	dialog.ShowInformation("About VibesAndFolders", aboutText, mw.window)
}

func (mw *MainWindow) Show() {
	mw.window.Show()
}

func (mw *MainWindow) ShowAndRun() {
	mw.window.ShowAndRun()
}
