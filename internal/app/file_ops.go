package app

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

// Backward compatibility functions for existing code

func CountFiles(rootPath string) (int, error) {
	service := NewFileService(NewValidator(), DefaultLogger)
	return service.CountFiles(rootPath)
}

func GetDirectoryStructure(rootPath string, maxDepth int) (string, error) {
	service := NewFileService(NewValidator(), DefaultLogger)
	return service.GetDirectoryStructure(rootPath, maxDepth)
}

func CleanEmptyDirectories(rootPath string) (int, error) {
	service := NewFileService(NewValidator(), DefaultLogger)
	return service.CleanEmptyDirectories(rootPath)
}

func ExecuteOperations(operations []FileOperation, basePath string, cleanEmpty bool, statusLabel *widget.Label, updateOutput func(string), window fyne.Window) {
	service := NewFileService(NewValidator(), DefaultLogger)

	var results strings.Builder
	successCount := 0
	failCount := 0

	fyne.Do(func() {
		statusLabel.SetText("Verifying integrity before execution...")
	})

	initialFileCount, countErr := service.CountFiles(basePath)
	if countErr != nil {
		fyne.Do(func() {
			dialog.ShowError(fmt.Errorf("integrity check failed (could not count files): %v", countErr), window)
			statusLabel.SetText("Execution Aborted")
		})
		return
	}

	for i, op := range operations {
		fyne.Do(func() {
			statusLabel.SetText(fmt.Sprintf("Executing %d/%d...", i+1, len(operations)))
		})

		validator := NewValidator()
		if err := validator.ValidateFileOperation(op); err != nil {
			if err == ErrSourceNotExist {
				results.WriteString(fmt.Sprintf("✗ [FAILED] %s → %s\n  Error: source file does not exist\n", op.From, op.To))
			} else if err == ErrDestinationExists {
				results.WriteString(fmt.Sprintf("✗ [FAILED] %s → %s\n  Error: destination already exists\n", op.From, op.To))
			} else {
				results.WriteString(fmt.Sprintf("✗ [FAILED] %s → %s\n  Error: %v\n", op.From, op.To, err))
			}
			failCount++
			continue
		}

		result := service.ExecuteOperation(op)
		if result.Success {
			results.WriteString(fmt.Sprintf("✓ [SUCCESS] %s → %s\n", op.From, op.To))
			successCount++
		} else {
			results.WriteString(fmt.Sprintf("✗ [FAILED] %s → %s\n  Error: %v\n", op.From, op.To, result.Error))
			failCount++
		}
	}

	cleanupMsg := ""
	if cleanEmpty {
		fyne.Do(func() {
			statusLabel.SetText("Cleaning up empty directories...")
		})
		removed, err := service.CleanEmptyDirectories(basePath)
		if err != nil {
			cleanupMsg = fmt.Sprintf("\n⚠ Cleanup Error: %v", err)
		} else {
			cleanupMsg = fmt.Sprintf("\n✨ Cleaned up %d empty directories.", removed)
		}
		results.WriteString(cleanupMsg)
	}

	finalFileCount, countErrAfter := service.CountFiles(basePath)
	verificationMsg := ""
	verificationSuccess := false

	if countErrAfter != nil {
		verificationMsg = fmt.Sprintf("\n⚠ VERIFICATION ERROR: Could not count files after execution: %v", countErrAfter)
	} else {
		if initialFileCount == finalFileCount {
			verificationMsg = fmt.Sprintf("\n🛡 VERIFICATION PASSED: File count maintained (%d files).", finalFileCount)
			verificationSuccess = true
		} else {
			diff := finalFileCount - initialFileCount
			verificationMsg = fmt.Sprintf("\n🛑 VERIFICATION WARNING: File count changed! Started with %d, ended with %d (Diff: %+d).", initialFileCount, finalFileCount, diff)
			verificationSuccess = false
		}
	}
	results.WriteString("\n" + verificationMsg)

	finalStatus := fmt.Sprintf("Completed: %d successful, %d failed", successCount, failCount)

	fyne.Do(func() {
		statusLabel.SetText(finalStatus)

		newContent := fmt.Sprintf("=== Execution Results ===\n%s", results.String())
		updateOutput(newContent)

		if failCount > 0 || !verificationSuccess {
			title := "Execution Warnings"
			msg := finalStatus + "\n\n" + verificationMsg
			if failCount > 0 {
				msg += "\n\nSome operations failed."
			}
			msg += "\nCheck the output log for details."

			dialog.ShowInformation(title, msg, window)
		} else {
			dialog.ShowInformation("Success", "All operations executed successfully!\n"+verificationMsg, window)
		}
	})
}
