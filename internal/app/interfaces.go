package app

// AIService defines the contract for AI suggestion services
type AIService interface {
	GetSuggestions(structure, userPrompt, basePath string) ([]FileOperation, error)
}

// FileService defines the contract for file operations
type FileService interface {
	GetDirectoryStructure(rootPath string, maxDepth int) (string, error)
	ExecuteOperations(operations []FileOperation, basePath string, cleanEmpty bool) (ExecutionResult, error)
	CountFiles(rootPath string) (int, error)
	CleanEmptyDirectories(rootPath string) (int, error)
}

// ExecutionResult holds the results of file operations
type ExecutionResult struct {
	SuccessCount      int
	FailCount         int
	InitialFileCount  int
	FinalFileCount    int
	CleanedDirs       int
	Operations        []OperationResult
	VerificationError error
}

// OperationResult holds the result of a single file operation
type OperationResult struct {
	Operation FileOperation
	Success   bool
	Error     error
}
