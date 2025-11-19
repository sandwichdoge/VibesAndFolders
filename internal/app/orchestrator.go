package app

import (
	"fmt"
)

type Orchestrator struct {
	aiService   AIService
	fileService FileService
	validator   *Validator
	logger      *Logger
}

func NewOrchestrator(aiService AIService, fileService FileService, validator *Validator, logger *Logger) *Orchestrator {
	return &Orchestrator{
		aiService:   aiService,
		fileService: fileService,
		validator:   validator,
		logger:      logger,
	}
}

type AnalysisRequest struct {
	DirectoryPath string
	UserPrompt    string
	MaxDepth      int
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
	result, err := o.fileService.ExecuteOperations(req.Operations, req.BasePath, req.CleanEmpty)
	if err != nil {
		o.logger.Error("Execution failed: %v", err)
	} else {
		o.logger.Info("Execution complete: %d successful, %d failed", result.SuccessCount, result.FailCount)
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

	o.logger.Info("Scanning directory: %s (depth: %d)", req.DirectoryPath, req.MaxDepth)
	structure, err := o.fileService.GetDirectoryStructure(req.DirectoryPath, req.MaxDepth)
	if err != nil {
		result.Error = fmt.Errorf("failed to scan directory: %w", err)
		return result
	}
	result.Structure = structure

	o.logger.Info("Requesting AI suggestions (Streaming)")

	// Pass the callback here
	operations, err := o.aiService.GetSuggestions(structure, req.UserPrompt, req.DirectoryPath, onOperation)

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
