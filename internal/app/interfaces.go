package app

// Callback function type for streaming operations
type OperationCallback func(op FileOperation)

// AIService defines the contract for AI suggestion services
type AIService interface {
	// GetSuggestions now takes a callback to stream results
	GetSuggestions(structure, userPrompt, basePath string, onOperation OperationCallback) ([]FileOperation, error)
}

// FileService defines the contract for file operations
type FileService interface {
	GetDirectoryStructure(rootPath string, maxDepth int) (string, error)
	ExecuteOperations(operations []FileOperation, basePath string, cleanEmpty bool) (ExecutionResult, error)
	CountFiles(rootPath string) (int, error)
	CleanEmptyDirectories(rootPath string) (int, error)
}

// ExecutionResult and OperationResult remain unchanged...
type ExecutionResult struct {
	SuccessCount      int
	FailCount         int
	InitialFileCount  int
	FinalFileCount    int
	CleanedDirs       int
	Operations        []OperationResult
	VerificationError error
}

type OperationResult struct {
	Operation FileOperation
	Success   bool
	Error     error
}
