package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

// GetDirectoryStructure scans a directory up to a specified depth.
// maxDepth == 0 means unlimited depth.
func GetDirectoryStructure(rootPath string, maxDepth int) (string, error) {
	var builder strings.Builder
	err := filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(rootPath, path)
		if err != nil {
			return err
		}

		// Skip the root directory itself, as it's provided as context
		if relPath == "." {
			return nil
		}

		// Use forward slashes for consistency
		relPath = filepath.ToSlash(relPath)

		// Calculate current depth
		currentDepth := len(strings.Split(relPath, "/"))

		// If maxDepth is set (not 0) and we are too deep
		if maxDepth > 0 && currentDepth > maxDepth {
			// If this is a directory, skip descending into it
			if info.IsDir() {
				return filepath.SkipDir
			}
			// If it's a file, just skip this entry
			return nil
		}

		if info.IsDir() {
			// Don't show size for directories
			builder.WriteString(fmt.Sprintf("%s/\n", relPath))
		} else {
			builder.WriteString(fmt.Sprintf("%s (%d bytes)\n", relPath, info.Size()))
		}

		return nil
	})

	return builder.String(), err
}

func ExecuteOperations(operations []FileOperation, basePath string, statusLabel *widget.Label, outputText *widget.Entry, window fyne.Window) {
	var results strings.Builder
	successCount := 0
	failCount := 0

	for i, op := range operations {
		fyne.Do(func() {
			statusLabel.SetText(fmt.Sprintf("Executing %d/%d...", i+1, len(operations)))
		})

		// Verify source exists
		if _, err := os.Stat(op.From); os.IsNotExist(err) {
			results.WriteString(fmt.Sprintf("✗ [FAILED] %s → %s\n  Error: source file does not exist\n", op.From, op.To))
			failCount++
			continue
		}

		// Create destination directory if it doesn't exist
		destDir := filepath.Dir(op.To)
		if err := os.MkdirAll(destDir, 0755); err != nil {
			results.WriteString(fmt.Sprintf("✗ [FAILED] %s → %s\n  Error: could not create directory: %v\n", op.From, op.To, err))
			failCount++
			continue
		}

		// Check if destination already exists
		if _, err := os.Stat(op.To); err == nil {
			results.WriteString(fmt.Sprintf("✗ [FAILED] %s → %s\n  Error: destination already exists\n", op.From, op.To))
			failCount++
			continue
		}

		// Perform the move
		if err := os.Rename(op.From, op.To); err != nil {
			results.WriteString(fmt.Sprintf("✗ [FAILED] %s → %s\n  Error: %v\n", op.From, op.To, err))
			failCount++
		} else {
			results.WriteString(fmt.Sprintf("✓ [SUCCESS] %s → %s\n", op.From, op.To))
			successCount++
		}
	}

	finalStatus := fmt.Sprintf("Completed: %d successful, %d failed", successCount, failCount)

	// Get new structure (this is fast enough to run in the background thread)
	structure, _ := GetDirectoryStructure(basePath, 0) // Use 0 for unlimited depth

	fyne.Do(func() {
		statusLabel.SetText(finalStatus)

		// Update output with results
		// Store the content before setting it to maintain read-only behavior
		newContent := fmt.Sprintf("=== Execution Results ===\n%s\n\n=== Updated Directory Structure ===\n%s", results.String(), structure)
		outputText.SetText(newContent)

		// Since we can't access lastOutputContent from here, we trigger the OnChanged
		// handler to update it by setting the text again
		outputText.OnChanged(newContent)

		if failCount > 0 {
			dialog.ShowInformation("Execution Complete", finalStatus+"\n\nSome operations failed. Check the output for details.", window)
		} else {
			dialog.ShowInformation("Success", "All operations executed successfully!", window)
		}
	})
}
