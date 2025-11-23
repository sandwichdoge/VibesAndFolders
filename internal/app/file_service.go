package app

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type DefaultFileService struct {
	validator      *Validator
	logger         *Logger
	ignoreMatcher  *IgnorePatternMatcher
}

func NewFileService(validator *Validator, logger *Logger) *DefaultFileService {
	return &DefaultFileService{
		validator:     validator,
		logger:        logger,
		ignoreMatcher: nil, // Will be set when needed
	}
}

// SetIgnorePatterns configures the ignore pattern matcher
func (fs *DefaultFileService) SetIgnorePatterns(patterns string) {
	if patterns == "" {
		fs.ignoreMatcher = nil
		return
	}
	fs.ignoreMatcher = NewIgnorePatternMatcher(patterns, fs.logger)
}

func (fs *DefaultFileService) CountFiles(rootPath string) (int, error) {
	count := 0
	err := filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Check if path should be ignored
		if fs.ignoreMatcher != nil && path != rootPath {
			relPath, err := filepath.Rel(rootPath, path)
			if err == nil && fs.ignoreMatcher.ShouldIgnore(relPath, info.IsDir()) {
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
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

		// Check if path should be ignored
		if fs.ignoreMatcher != nil && fs.ignoreMatcher.ShouldIgnore(relPath, info.IsDir()) {
			if info.IsDir() {
				// Show the ignored directory name (for context) but skip its contents
				builder.WriteString(fmt.Sprintf("%s/\n", relPath))
				return filepath.SkipDir
			}
			// Skip ignored files silently
			return nil
		}

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

// determineVerificationScope analyzes operations to determine which directories need verification.
// If operations move files outside basePath (e.g., to parent directory), those paths are included.
// Returns the common ancestor directory that encompasses all source and destination paths to avoid
// double-counting files in nested directories.
func (fs *DefaultFileService) determineVerificationScope(operations []FileOperation, basePath string) []string {
	// Normalize basePath for comparison
	basePath = filepath.Clean(basePath)

	// Track unique directories that need verification
	pathsMap := make(map[string]bool)
	pathsMap[basePath] = true

	for _, op := range operations {
		// Check both source and destination directories
		sourcePath := filepath.Clean(op.From)
		sourceDir := filepath.Dir(sourcePath)
		destPath := filepath.Clean(op.To)
		destDir := filepath.Dir(destPath)

		// Check if source is outside basePath
		relSourcePath, err := filepath.Rel(basePath, sourceDir)
		if err == nil && strings.HasPrefix(relSourcePath, "..") {
			pathsMap[sourceDir] = true
			fs.logger.Debug("Added verification path (external source): %s", sourceDir)
		}

		// Check if destination is outside basePath
		relDestPath, err := filepath.Rel(basePath, destDir)
		if err == nil && strings.HasPrefix(relDestPath, "..") {
			pathsMap[destDir] = true
			fs.logger.Debug("Added verification path (external destination): %s", destDir)
		}
	}

	// Convert map to slice
	paths := make([]string, 0, len(pathsMap))
	for path := range pathsMap {
		paths = append(paths, path)
	}

	// Sort for consistent ordering
	sort.Strings(paths)

	// If we have multiple paths, check for parent-child relationships
	// If one path contains another, use only the parent to avoid double-counting
	if len(paths) > 1 {
		result := fs.findCommonAncestor(paths)
		fs.logger.Info("Multi-path verification: using common ancestor %s to avoid double-counting", result)
		return []string{result}
	}

	return paths
}

// findCommonAncestor finds the common ancestor directory of all given paths
func (fs *DefaultFileService) findCommonAncestor(paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	if len(paths) == 1 {
		return paths[0]
	}

	// Start with the first path
	common := paths[0]

	// For each subsequent path, find the common ancestor with current common
	for i := 1; i < len(paths); i++ {
		common = fs.commonAncestorOfTwo(common, paths[i])
	}

	return common
}

// commonAncestorOfTwo finds the common ancestor of two paths
func (fs *DefaultFileService) commonAncestorOfTwo(path1, path2 string) string {
	// Clean both paths
	path1 = filepath.Clean(path1)
	path2 = filepath.Clean(path2)

	// Check if paths are absolute
	isAbs := filepath.IsAbs(path1) && filepath.IsAbs(path2)

	// Split into components
	parts1 := strings.Split(path1, string(filepath.Separator))
	parts2 := strings.Split(path2, string(filepath.Separator))

	// Find common prefix
	var commonParts []string
	minLen := len(parts1)
	if len(parts2) < minLen {
		minLen = len(parts2)
	}

	for i := 0; i < minLen; i++ {
		if parts1[i] == parts2[i] {
			commonParts = append(commonParts, parts1[i])
		} else {
			break
		}
	}

	// Join back into path
	if len(commonParts) == 0 {
		return string(filepath.Separator)
	}

	result := filepath.Join(commonParts...)

	// Restore leading slash for absolute paths if necessary
	if isAbs && !filepath.IsAbs(result) {
		result = string(filepath.Separator) + result
	}

	return result
}

func (fs *DefaultFileService) ExecuteOperations(operations []FileOperation, basePath string, cleanEmpty bool) (ExecutionResult, error) {
	result := ExecutionResult{
		Operations: make([]OperationResult, 0, len(operations)),
	}

	// Determine all paths that need verification (basePath + any external destinations)
	verificationPaths := fs.determineVerificationScope(operations, basePath)

	// Count files across all verification paths before execution
	initialCount := 0
	for _, path := range verificationPaths {
		count, err := fs.CountFiles(path)
		if err != nil {
			result.VerificationError = fmt.Errorf("integrity check failed for %s: %w", path, err)
			return result, result.VerificationError
		}
		initialCount += count
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

	// Count files across all verification paths after execution
	finalCount := 0
	for _, path := range verificationPaths {
		count, err := fs.CountFiles(path)
		if err != nil {
			result.VerificationError = fmt.Errorf("post-execution count failed for %s: %w", path, err)
		}
		finalCount += count
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
