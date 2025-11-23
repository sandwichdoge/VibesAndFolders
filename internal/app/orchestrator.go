package app

import (
	"fmt"
	"path/filepath"
	"strings"
)

type Orchestrator struct {
	aiService            AIService
	fileService          FileService
	validator            *Validator
	logger               *Logger
	indexOrchestrator    *IndexDirectoryOrchestrator
	indexService         IndexService
}

func NewOrchestrator(aiService AIService, fileService FileService, validator *Validator, logger *Logger, indexOrchestrator *IndexDirectoryOrchestrator, indexService IndexService) *Orchestrator {
	return &Orchestrator{
		aiService:         aiService,
		fileService:       fileService,
		validator:         validator,
		logger:            logger,
		indexOrchestrator: indexOrchestrator,
		indexService:      indexService,
	}
}

type AnalysisRequest struct {
	DirectoryPath      string
	UserPrompt         string
	MaxDepth           int
	EnableDeepAnalysis bool
}

type AnalysisResult struct {
	Structure  string
	Operations []FileOperation
	Error      error
}

type ExecutionRequest struct {
	Operations []FileOperation
	BasePath   string
	CleanEmpty bool
}

func (o *Orchestrator) ExecuteOrganization(req ExecutionRequest) ExecutionResult {
	o.logger.Info("Starting execution of %d operations", len(req.Operations))

	// Create index snapshot before execution if deep analysis is enabled
	var indexSnapshot *IndexSnapshot
	if o.indexOrchestrator != nil && o.indexService != nil {
		o.logger.Debug("Creating index snapshot before execution")
		snapshot, err := o.indexService.CreateSnapshot(req.Operations)
		if err != nil {
			o.logger.Error("Failed to create index snapshot: %v", err)
		} else {
			indexSnapshot = snapshot
			o.logger.Debug("Index snapshot created with %d entries", len(snapshot.Entries))
		}
	}

	result, err := o.fileService.ExecuteOperations(req.Operations, req.BasePath, req.CleanEmpty)
	if err != nil {
		o.logger.Error("Execution failed: %v", err)
	} else {
		o.logger.Info("Execution complete: %d successful, %d failed", result.SuccessCount, result.FailCount)
	}

	// Smartly update the index after execution (if deep analysis is enabled and there were successful operations)
	if result.SuccessCount > 0 && o.indexOrchestrator != nil && o.indexService != nil {
		o.logger.Info("Updating index after execution")

		// Start a transaction for atomic index updates
		if err := o.indexService.BeginTransaction(); err != nil {
			o.logger.Error("Failed to begin index transaction: %v", err)
		} else {
			// Extract only successful operations
			var successfulOps []FileOperation
			for _, opResult := range result.Operations {
				if opResult.Success {
					successfulOps = append(successfulOps, opResult.Operation)
				}
			}

			if err := o.indexOrchestrator.UpdateIndexAfterOperations(successfulOps); err != nil {
				o.logger.Error("Failed to update index after execution: %v", err)
				// Rollback the transaction
				if rbErr := o.indexService.RollbackTransaction(); rbErr != nil {
					o.logger.Error("Failed to rollback index transaction: %v", rbErr)
				}
				// Restore from snapshot if available
				if indexSnapshot != nil {
					o.logger.Info("Restoring index from snapshot due to update failure")
					if restoreErr := o.indexService.RestoreSnapshot(indexSnapshot); restoreErr != nil {
						o.logger.Error("Failed to restore index snapshot: %v", restoreErr)
					}
				}
			} else {
				// Commit the transaction
				if commitErr := o.indexService.CommitTransaction(); commitErr != nil {
					o.logger.Error("Failed to commit index transaction: %v", commitErr)
				} else {
					o.logger.Info("Index update complete")
				}
			}
		}
	}

	return result
}

func (o *Orchestrator) AnalyzeDirectory(req AnalysisRequest, onOperation OperationCallback) AnalysisResult {
	result := AnalysisResult{}

	if err := o.validator.ValidateDirectory(req.DirectoryPath); err != nil {
		result.Error = err
		return result
	}

	if err := o.validator.ValidatePrompt(req.UserPrompt); err != nil {
		result.Error = err
		return result
	}

	// Index the directory before analysis if deep analysis is enabled and there are files to index
	if req.EnableDeepAnalysis && o.indexOrchestrator != nil && o.indexService != nil {
		o.logger.Info("Checking if directory needs indexing: %s (depth: %d)", req.DirectoryPath, req.MaxDepth)

		// First, clean up any orphaned entries from previous operations
		removed, err := o.indexService.RemoveOrphanedEntries(req.DirectoryPath)
		if err != nil {
			o.logger.Error("Failed to remove orphaned entries: %v", err)
		} else if removed > 0 {
			o.logger.Info("Cleaned up %d orphaned index entries", removed)
		}

		changes, err := o.indexService.ScanDirectoryChanges(req.DirectoryPath, req.MaxDepth)
		if err != nil {
			o.logger.Error("Failed to scan directory changes: %v", err)
		} else {
			totalToIndex := len(changes.NewFiles) + len(changes.ModifiedFiles)
			if totalToIndex > 0 {
				o.logger.Info("Found %d files to index, starting indexing...", totalToIndex)
				if err := o.indexOrchestrator.IndexDirectory(req.DirectoryPath, req.MaxDepth, func(current, total int, fileName string) {
					o.logger.Debug("Indexing file %d/%d: %s", current, total, fileName)
				}); err != nil {
					o.logger.Error("Failed to index directory: %v", err)
				} else {
					o.logger.Info("Indexing complete")
				}
			} else {
				o.logger.Info("No files need indexing, using existing index")
			}
		}
	}

	o.logger.Info("Scanning directory: %s (depth: %d)", req.DirectoryPath, req.MaxDepth)
	structure, err := o.fileService.GetDirectoryStructure(req.DirectoryPath, req.MaxDepth)
	if err != nil {
		result.Error = fmt.Errorf("failed to scan directory: %w", err)
		return result
	}

	// Enrich structure with descriptions from index if deep analysis is enabled
	enrichedStructure := structure
	if req.EnableDeepAnalysis && o.indexOrchestrator != nil && o.indexService != nil {
		enrichedStructure, err = o.enrichStructureWithDescriptions(req.DirectoryPath, structure)
		if err != nil {
			o.logger.Error("Failed to enrich structure with descriptions: %v", err)
			// Fall back to basic structure
			enrichedStructure = structure
		} else {
			o.logger.Info("Structure enriched with AI descriptions")
		}
	}

	result.Structure = enrichedStructure

	o.logger.Info("Requesting AI suggestions (Streaming)")

	// Pass the callback here
	operations, err := o.aiService.GetSuggestions(enrichedStructure, req.UserPrompt, req.DirectoryPath, onOperation)

	if err != nil {
		result.Error = fmt.Errorf("failed to get AI suggestions: %w", err)
		return result
	}
	result.Operations = operations

	o.logger.Info("Analysis complete: %d operations suggested", len(operations))
	return result
}

func (o *Orchestrator) GetDirectoryStructure(path string, maxDepth int) (string, error) {
	return o.fileService.GetDirectoryStructure(path, maxDepth)
}

// GetDirectoryIndexStats returns statistics about indexed files in a directory
func (o *Orchestrator) GetDirectoryIndexStats(dirPath string) (map[string]int, error) {
	if o.indexOrchestrator == nil {
		return map[string]int{"total": 0}, nil
	}
	return o.indexOrchestrator.GetDirectoryIndexStats(dirPath)
}

// ScanDirectoryChanges scans for changes in a directory
func (o *Orchestrator) ScanDirectoryChanges(dirPath string, maxDepth int) (*DirectoryChanges, error) {
	if o.indexService == nil {
		return nil, fmt.Errorf("index service not available")
	}
	return o.indexService.ScanDirectoryChanges(dirPath, maxDepth)
}

// IndexDirectory indexes all files in a directory
func (o *Orchestrator) IndexDirectory(dirPath string, maxDepth int, onProgress func(current, total int, fileName string)) error {
	if o.indexOrchestrator == nil {
		return fmt.Errorf("index orchestrator not available")
	}
	return o.indexOrchestrator.IndexDirectory(dirPath, maxDepth, onProgress)
}

// DeleteDirectoryIndex deletes all indexed files for a directory
func (o *Orchestrator) DeleteDirectoryIndex(dirPath string) (int, error) {
	if o.indexService == nil {
		return 0, fmt.Errorf("index service not available")
	}
	return o.indexService.DeleteDirectoryIndex(dirPath)
}

// GetIndexedFiles returns all indexed files in a directory
func (o *Orchestrator) GetIndexedFiles(dirPath string) ([]IndexedFile, error) {
	if o.indexService == nil {
		return nil, fmt.Errorf("index service not available")
	}
	return o.indexService.GetIndexedFilesInDirectory(dirPath)
}

// enrichStructureWithDescriptions adds AI-generated descriptions to the directory structure
func (o *Orchestrator) enrichStructureWithDescriptions(dirPath, structure string) (string, error) {
	// Get all indexed files in this directory
	indexedFiles, err := o.indexService.GetIndexedFilesInDirectory(dirPath)
	if err != nil {
		return structure, err
	}

	// Create a map for quick lookup
	descriptionMap := make(map[string]string)
	for _, file := range indexedFiles {
		descriptionMap[file.FilePath] = file.Description
	}

	// Parse the structure line by line and add descriptions
	lines := strings.Split(structure, "\n")
	var enriched strings.Builder

	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			enriched.WriteString(line + "\n")
			continue
		}

		// Skip directory entries (they end with /)
		if strings.HasSuffix(strings.TrimSpace(line), "/") {
			enriched.WriteString(line + "\n")
			continue
		}

		// Extract the relative path from the line
		// Format is: "path/to/file.ext (123 bytes)"
		parts := strings.SplitN(line, " (", 2)
		if len(parts) < 2 {
			enriched.WriteString(line + "\n")
			continue
		}

		relPath := strings.TrimSpace(parts[0])
		sizeInfo := " (" + parts[1] // Keep the size info

		// Construct full path
		fullPath := filepath.Join(dirPath, relPath)
		fullPath = filepath.Clean(fullPath)

		// Check if we have a description for this file
		if desc, ok := descriptionMap[fullPath]; ok && desc != "" {
			// Add description before the size info
			enriched.WriteString(relPath + " [" + desc + "]" + sizeInfo + "\n")
		} else {
			// No description, keep original
			enriched.WriteString(line + "\n")
		}
	}

	return enriched.String(), nil
}
