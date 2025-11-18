package app

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

// CountFiles walks the directory tree and counts the total number of files (excluding directories).
func CountFiles(rootPath string) (int, error) {
	count := 0
	err := filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			count++
		}
		return nil
	})
	return count, err
}

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

// CleanEmptyDirectories recursively removes empty directories.
// It processes the tree depth-first (longest paths first) to ensure nested empty folders are removed.
func CleanEmptyDirectories(rootPath string) (int, error) {
	var dirs []string

	// 1. Collect all directories
	err := filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() && path != rootPath {
			dirs = append(dirs, path)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}

	// 2. Sort directories by length of path in descending order.
	// This ensures we try to delete "a/b/c" before "a/b".
	// If "c" is empty and deleted, "b" might become empty and can then be deleted.
	sort.Slice(dirs, func(i, j int) bool {
		return len(dirs[i]) > len(dirs[j])
	})

	removedCount := 0
	for _, dir := range dirs {
		// os.Remove only removes a directory if it is empty.
		// We don't need to check isEmpty manually; we just try to remove it.
		if err := os.Remove(dir); err == nil {
			removedCount++
		}
	}

	return removedCount, nil
}

func ExecuteOperations(operations []FileOperation, basePath string, cleanEmpty bool, statusLabel *widget.Label, updateOutput func(string), window fyne.Window) {
	var results strings.Builder
	successCount := 0
	failCount := 0

	// --- VERIFICATION: PRE-EXECUTION COUNT ---
	fyne.Do(func() {
		statusLabel.SetText("Verifying integrity before execution...")
	})

	initialFileCount, countErr := CountFiles(basePath)
	if countErr != nil {
		fyne.Do(func() {
			dialog.ShowError(fmt.Errorf("integrity check failed (could not count files): %v", countErr), window)
			statusLabel.SetText("Execution Aborted")
		})
		return
	}
	// -----------------------------------------

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

	// Clean up empty directories if requested
	cleanupMsg := ""
	if cleanEmpty {
		fyne.Do(func() {
			statusLabel.SetText("Cleaning up empty directories...")
		})
		removed, err := CleanEmptyDirectories(basePath)
		if err != nil {
			cleanupMsg = fmt.Sprintf("\n⚠ Cleanup Error: %v", err)
		} else {
			cleanupMsg = fmt.Sprintf("\n✨ Cleaned up %d empty directories.", removed)
		}
		results.WriteString(cleanupMsg)
	}

	// --- VERIFICATION: POST-EXECUTION COUNT ---
	finalFileCount, countErrAfter := CountFiles(basePath)
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
	// ------------------------------------------

	finalStatus := fmt.Sprintf("Completed: %d successful, %d failed", successCount, failCount)

	fyne.Do(func() {
		statusLabel.SetText(finalStatus)

		// Build the final text content (Execution Results only)
		newContent := fmt.Sprintf("=== Execution Results ===\n%s", results.String())

		// Update UI
		updateOutput(newContent)

		// Determine Dialog Type based on Failures AND Verification
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
