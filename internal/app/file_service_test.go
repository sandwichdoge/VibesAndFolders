package app

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func TestDetermineVerificationScope(t *testing.T) {
	// Create a mock file service for testing
	fs := &DefaultFileService{
		logger: NewLogger(false), // Silent logger for tests
	}

	tests := []struct {
		name       string
		basePath   string
		operations []FileOperation
		expected   []string
	}{
		{
			name:     "no operations - only basePath",
			basePath: "/home/user/project",
			operations: []FileOperation{},
			expected: []string{"/home/user/project"},
		},
		{
			name:     "operations within basePath only",
			basePath: "/home/user/project",
			operations: []FileOperation{
				{From: "/home/user/project/file1.txt", To: "/home/user/project/subdir/file1.txt"},
				{From: "/home/user/project/file2.txt", To: "/home/user/project/another/file2.txt"},
			},
			expected: []string{"/home/user/project"},
		},
		{
			name:     "move to parent directory - uses common ancestor",
			basePath: "/home/user/project/subfolder",
			operations: []FileOperation{
				{From: "/home/user/project/subfolder/file1.txt", To: "/home/user/project/file1.txt"},
				{From: "/home/user/project/subfolder/file2.txt", To: "/home/user/project/file2.txt"},
			},
			expected: []string{"/home/user/project"},
		},
		{
			name:     "move to grandparent directory - uses common ancestor",
			basePath: "/home/user/project/sub1/sub2",
			operations: []FileOperation{
				{From: "/home/user/project/sub1/sub2/file.txt", To: "/home/user/project/file.txt"},
			},
			expected: []string{"/home/user/project"},
		},
		{
			name:     "mixed operations - uses common ancestor",
			basePath: "/home/user/project/subfolder",
			operations: []FileOperation{
				{From: "/home/user/project/subfolder/file1.txt", To: "/home/user/project/subfolder/moved/file1.txt"},
				{From: "/home/user/project/subfolder/file2.txt", To: "/home/user/project/file2.txt"},
				{From: "/home/user/project/subfolder/file3.txt", To: "/home/user/project/another/file3.txt"},
			},
			expected: []string{"/home/user/project"},
		},
		{
			name:     "move to sibling directory - uses common ancestor",
			basePath: "/home/user/project/folder-a",
			operations: []FileOperation{
				{From: "/home/user/project/folder-a/file.txt", To: "/home/user/project/folder-b/file.txt"},
			},
			expected: []string{"/home/user/project"},
		},
		{
			name:     "multiple external destinations - uses common ancestor",
			basePath: "/home/user/project/source",
			operations: []FileOperation{
				{From: "/home/user/project/source/file1.txt", To: "/home/user/project/dest1/file1.txt"},
				{From: "/home/user/project/source/file2.txt", To: "/home/user/project/dest2/file2.txt"},
				{From: "/home/user/project/source/file3.txt", To: "/home/user/dest3/file3.txt"},
			},
			expected: []string{"/home/user"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := fs.determineVerificationScope(tt.operations, tt.basePath)

			// Sort both for comparison
			sort.Strings(result)
			sort.Strings(tt.expected)

			if len(result) != len(tt.expected) {
				t.Errorf("determineVerificationScope() returned %d paths, want %d\nGot: %v\nWant: %v",
					len(result), len(tt.expected), result, tt.expected)
				return
			}

			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("determineVerificationScope()[%d] = %q, want %q", i, result[i], tt.expected[i])
				}
			}
		})
	}
}

func TestExecuteOperations_WithParentMoveVerification(t *testing.T) {
	// Create a temporary directory structure for testing
	tempDir := t.TempDir()

	// Create some existing files in parent directory (to simulate real scenario)
	existingFile := filepath.Join(tempDir, "existing.txt")
	if err := os.WriteFile(existingFile, []byte("existing"), 0644); err != nil {
		t.Fatalf("Failed to create existing file: %v", err)
	}

	// Create subfolder and files
	subfolder := filepath.Join(tempDir, "subfolder")
	if err := os.MkdirAll(subfolder, 0755); err != nil {
		t.Fatalf("Failed to create subfolder: %v", err)
	}

	// Create test files in subfolder
	file1 := filepath.Join(subfolder, "test1.txt")
	file2 := filepath.Join(subfolder, "test2.txt")
	file3 := filepath.Join(subfolder, "test3.txt")

	for _, file := range []string{file1, file2, file3} {
		if err := os.WriteFile(file, []byte("test content"), 0644); err != nil {
			t.Fatalf("Failed to create test file %s: %v", file, err)
		}
	}

	// Create file service
	validator := NewValidator()
	logger := NewLogger(false) // Silent logger for tests
	fs := NewFileService(validator, logger)

	// Create operations to move files from subfolder to parent
	operations := []FileOperation{
		{From: file1, To: filepath.Join(tempDir, "test1.txt")},
		{From: file2, To: filepath.Join(tempDir, "test2.txt")},
		{From: file3, To: filepath.Join(tempDir, "test3.txt")},
	}

	// Execute operations with subfolder as basePath
	result, err := fs.ExecuteOperations(operations, subfolder, false)

	if err != nil {
		t.Fatalf("ExecuteOperations() returned error: %v", err)
	}

	// Verification should pass because we count files in both subfolder and parent
	if result.VerificationError != nil {
		t.Errorf("Verification failed: %v", result.VerificationError)
	}

	// All operations should succeed
	if result.SuccessCount != 3 {
		t.Errorf("SuccessCount = %d, want 3", result.SuccessCount)
	}

	if result.FailCount != 0 {
		t.Errorf("FailCount = %d, want 0", result.FailCount)
	}

	// File counts should match (before and after should be equal)
	// The counts include both subfolder and parent directory
	if result.InitialFileCount != result.FinalFileCount {
		t.Errorf("File count changed: InitialFileCount = %d, FinalFileCount = %d (should be equal)",
			result.InitialFileCount, result.FinalFileCount)
	}

	// The initial count should be 4 total: 3 in subfolder + 1 existing in parent
	// Final count should also be 4: 0 in subfolder + 4 in parent (1 existing + 3 moved)
	expectedCount := 4
	if result.InitialFileCount != expectedCount {
		t.Errorf("InitialFileCount = %d, want %d", result.InitialFileCount, expectedCount)
	}
	if result.FinalFileCount != expectedCount {
		t.Errorf("FinalFileCount = %d, want %d", result.FinalFileCount, expectedCount)
	}

	// Verify files actually moved
	for _, file := range []string{file1, file2, file3} {
		if _, err := os.Stat(file); !os.IsNotExist(err) {
			t.Errorf("Source file still exists: %s", file)
		}
	}

	for _, file := range []string{"test1.txt", "test2.txt", "test3.txt"} {
		destPath := filepath.Join(tempDir, file)
		if _, err := os.Stat(destPath); os.IsNotExist(err) {
			t.Errorf("Destination file does not exist: %s", destPath)
		}
	}
}

func TestExecuteOperations_InternalMoveVerification(t *testing.T) {
	// Create a temporary directory structure for testing
	tempDir := t.TempDir()

	// Create subfolder and files
	subfolder := filepath.Join(tempDir, "subfolder")
	organized := filepath.Join(subfolder, "organized")
	if err := os.MkdirAll(subfolder, 0755); err != nil {
		t.Fatalf("Failed to create subfolder: %v", err)
	}

	// Create test files in subfolder root
	file1 := filepath.Join(subfolder, "file1.txt")
	file2 := filepath.Join(subfolder, "file2.txt")

	for _, file := range []string{file1, file2} {
		if err := os.WriteFile(file, []byte("test content"), 0644); err != nil {
			t.Fatalf("Failed to create test file %s: %v", file, err)
		}
	}

	// Create file service
	validator := NewValidator()
	logger := NewLogger(false)
	fs := NewFileService(validator, logger)

	// Create operations to move files within subfolder
	operations := []FileOperation{
		{From: file1, To: filepath.Join(organized, "file1.txt")},
		{From: file2, To: filepath.Join(organized, "file2.txt")},
	}

	// Execute operations with subfolder as basePath
	result, err := fs.ExecuteOperations(operations, subfolder, false)

	if err != nil {
		t.Fatalf("ExecuteOperations() returned error: %v", err)
	}

	// Verification should pass - files moved within basePath
	if result.VerificationError != nil {
		t.Errorf("Verification failed: %v", result.VerificationError)
	}

	// File counts should match
	if result.InitialFileCount != result.FinalFileCount {
		t.Errorf("File count mismatch: initial=%d, final=%d", result.InitialFileCount, result.FinalFileCount)
	}

	if result.InitialFileCount != 2 {
		t.Errorf("InitialFileCount = %d, want 2", result.InitialFileCount)
	}
}
