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
	if err := os.MkdirAll(destDir, 0755); err != nil {
		result.Error = fmt.Errorf("%w: %v", ErrCannotCreateDir, err)
		return result
	}

	if err := os.Rename(op.From, op.To); err != nil {
		result.Error = err
		return result
	}

	result.Success = true
	fs.logger.Debug("Successfully moved: %s -> %s", op.From, op.To)
	return result
}
