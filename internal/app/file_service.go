package app

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type DefaultFileService struct {
	validator *Validator
	logger    *Logger
}

func NewFileService(validator *Validator, logger *Logger) *DefaultFileService {
	return &DefaultFileService{
		validator: validator,
		logger:    logger,
	}
}

func (fs *DefaultFileService) CountFiles(rootPath string) (int, error) {
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

func (fs *DefaultFileService) GetDirectoryStructure(rootPath string, maxDepth int) (string, error) {
	var builder strings.Builder
	err := filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(rootPath, path)
		if err != nil {
			return err
		}

		if relPath == "." {
			return nil
		}

		relPath = filepath.ToSlash(relPath)
		currentDepth := len(strings.Split(relPath, "/"))

		if maxDepth > 0 && currentDepth > maxDepth {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if info.IsDir() {
			builder.WriteString(fmt.Sprintf("%s/\n", relPath))
		} else {
			builder.WriteString(fmt.Sprintf("%s (%d bytes)\n", relPath, info.Size()))
		}

		return nil
	})

	return builder.String(), err
}

func (fs *DefaultFileService) CleanEmptyDirectories(rootPath string) (int, error) {
	var dirs []string

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

	sort.Slice(dirs, func(i, j int) bool {
		return len(dirs[i]) > len(dirs[j])
	})

	removedCount := 0
	for _, dir := range dirs {
		if err := os.Remove(dir); err == nil {
			removedCount++
			fs.logger.Debug("Removed empty directory: %s", dir)
		}
	}

	return removedCount, nil
}

func (fs *DefaultFileService) ExecuteOperations(operations []FileOperation, basePath string, cleanEmpty bool) (ExecutionResult, error) {
	result := ExecutionResult{
		Operations: make([]OperationResult, 0, len(operations)),
	}

	initialCount, err := fs.CountFiles(basePath)
	if err != nil {
		result.VerificationError = fmt.Errorf("integrity check failed: %w", err)
		return result, result.VerificationError
	}
	result.InitialFileCount = initialCount

	for _, op := range operations {
		opResult := fs.ExecuteOperation(op)
		result.Operations = append(result.Operations, opResult)

		if opResult.Success {
			result.SuccessCount++
		} else {
			result.FailCount++
		}
	}

	if cleanEmpty {
		cleaned, err := fs.CleanEmptyDirectories(basePath)
		if err != nil {
			fs.logger.Error("Failed to clean empty directories: %v", err)
		} else {
			result.CleanedDirs = cleaned
		}
	}

	finalCount, err := fs.CountFiles(basePath)
	if err != nil {
		result.VerificationError = fmt.Errorf("post-execution count failed: %w", err)
	}
	result.FinalFileCount = finalCount

	return result, nil
}

func (fs *DefaultFileService) ExecuteOperation(op FileOperation) OperationResult {
	result := OperationResult{
		Operation: op,
		Success:   false,
	}

	if err := fs.validator.ValidateFileOperation(op); err != nil {
		result.Error = err
		return result
	}

	destDir := filepath.Dir(op.To)

	// Track which directories we create
	var createdDirs []string

	// Check which directories need to be created
	currentPath := destDir
	for currentPath != "" && currentPath != "." && currentPath != "/" {
		if _, err := os.Stat(currentPath); os.IsNotExist(err) {
			// This directory doesn't exist, mark it for tracking
			createdDirs = append([]string{currentPath}, createdDirs...) // prepend to maintain order
		} else {
			// Directory exists, no need to check parent
			break
		}
		currentPath = filepath.Dir(currentPath)
		if currentPath == filepath.Dir(currentPath) {
			// Reached root
			break
		}
	}

	if err := os.MkdirAll(destDir, 0755); err != nil {
		result.Error = fmt.Errorf("%w: %v", ErrCannotCreateDir, err)
		return result
	}

	// Store the created directories in the result
	result.CreatedDirs = createdDirs

	// Check if source is a symlink using Lstat (doesn't follow symlinks)
	fileInfo, err := os.Lstat(op.From)
	if err != nil {
		result.Error = fmt.Errorf("failed to stat source: %v", err)
		return result
	}

	// Handle symlinks specially
	if fileInfo.Mode()&os.ModeSymlink != 0 {
		// Read the symlink target
		linkTarget, err := os.Readlink(op.From)
		if err != nil {
			result.Error = fmt.Errorf("failed to read symlink: %v", err)
			return result
		}

		// Store original symlink target for rollback
		result.SymlinkTarget = linkTarget

		// Determine the target path to use at the new location
		newTarget := linkTarget

		// If the target is relative, we need to adjust it for the new location
		if !filepath.IsAbs(linkTarget) {
			// Resolve the absolute path of what the symlink currently points to
			symlinkDir := filepath.Dir(op.From)
			absoluteTarget := filepath.Join(symlinkDir, linkTarget)
			absoluteTarget = filepath.Clean(absoluteTarget)

			// Calculate the new relative path from the destination
			newSymlinkDir := filepath.Dir(op.To)
			newRelTarget, err := filepath.Rel(newSymlinkDir, absoluteTarget)
			if err != nil {
				// If we can't create a relative path, fall back to absolute
				newTarget = absoluteTarget
				fs.logger.Debug("Converting symlink to absolute path: %s", absoluteTarget)
			} else {
				newTarget = newRelTarget
				fs.logger.Debug("Adjusted relative symlink path: %s -> %s", linkTarget, newTarget)
			}
		}

		// Remove the old symlink
		if err := os.Remove(op.From); err != nil {
			result.Error = fmt.Errorf("failed to remove original symlink: %v", err)
			return result
		}

		// Create the new symlink at destination with the adjusted target
		if err := os.Symlink(newTarget, op.To); err != nil {
			// Try to restore original symlink on failure
			restoreErr := os.Symlink(linkTarget, op.From)
			if restoreErr != nil {
				result.Error = fmt.Errorf("failed to create new symlink and restore original: %v (restore error: %v)", err, restoreErr)
			} else {
				result.Error = fmt.Errorf("failed to create new symlink: %v", err)
			}
			return result
		}

		result.Success = true
		if newTarget != linkTarget {
			fs.logger.Debug("Successfully moved symlink with adjusted target: %s -> %s (original target: %s, new target: %s)", op.From, op.To, linkTarget, newTarget)
		} else {
			fs.logger.Debug("Successfully moved symlink: %s -> %s (target: %s)", op.From, op.To, linkTarget)
		}
		return result
	}

	// For regular files and directories, use os.Rename
	if err := os.Rename(op.From, op.To); err != nil {
		result.Error = err
		return result
	}

	result.Success = true
	fs.logger.Debug("Successfully moved: %s -> %s", op.From, op.To)
	return result
}
