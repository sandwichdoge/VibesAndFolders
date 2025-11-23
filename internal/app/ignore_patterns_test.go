package app

import (
	"testing"
)

func TestIgnorePatternMatcher_ShouldIgnore(t *testing.T) {
	tests := []struct {
		name     string
		patterns string
		path     string
		isDir    bool
		expected bool
	}{
		{
			name:     "ignore .git directory",
			patterns: ".git/",
			path:     ".git",
			isDir:    true,
			expected: true,
		},
		{
			name:     "ignore file in .git directory",
			patterns: ".git/",
			path:     ".git/config",
			isDir:    false,
			expected: true,
		},
		{
			name:     "ignore node_modules directory",
			patterns: "node_modules/",
			path:     "node_modules",
			isDir:    true,
			expected: true,
		},
		{
			name:     "ignore file with wildcard extension",
			patterns: "*.log",
			path:     "debug.log",
			isDir:    false,
			expected: true,
		},
		{
			name:     "do not ignore .log directory",
			patterns: "*.log",
			path:     "debug.log",
			isDir:    true,
			expected: true, // glob matches both files and directories
		},
		{
			name:     "ignore nested .git",
			patterns: ".git/",
			path:     "project/.git",
			isDir:    true,
			expected: false, // need **/.git/ for nested
		},
		{
			name:     "ignore with doublestar",
			patterns: "**/.git/",
			path:     "project/.git",
			isDir:    true,
			expected: true,
		},
		{
			name:     "do not ignore non-matching file",
			patterns: "*.log",
			path:     "readme.txt",
			isDir:    false,
			expected: false,
		},
		{
			name:     "ignore .DS_Store",
			patterns: ".DS_Store",
			path:     ".DS_Store",
			isDir:    false,
			expected: true,
		},
		{
			name:     "ignore multiple patterns - match first",
			patterns: "*.log\n*.tmp\n.git/",
			path:     "debug.log",
			isDir:    false,
			expected: true,
		},
		{
			name:     "ignore multiple patterns - match second",
			patterns: "*.log\n*.tmp\n.git/",
			path:     "temp.tmp",
			isDir:    false,
			expected: true,
		},
		{
			name:     "ignore multiple patterns - match third",
			patterns: "*.log\n*.tmp\n.git/",
			path:     ".git",
			isDir:    true,
			expected: true,
		},
		{
			name:     "skip comments",
			patterns: "# This is a comment\n*.log\n# Another comment",
			path:     "debug.log",
			isDir:    false,
			expected: true,
		},
		{
			name:     "skip empty lines",
			patterns: "*.log\n\n\n*.tmp",
			path:     "temp.tmp",
			isDir:    false,
			expected: true,
		},
		{
			name:     "ignore build directory",
			patterns: "build/",
			path:     "build",
			isDir:    true,
			expected: true,
		},
		{
			name:     "ignore nested files in build directory",
			patterns: "build/",
			path:     "build/output.exe",
			isDir:    false,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher := NewIgnorePatternMatcher(tt.patterns, nil)
			result := matcher.ShouldIgnore(tt.path, tt.isDir)
			if result != tt.expected {
				t.Errorf("ShouldIgnore(%q, %v) = %v, want %v", tt.path, tt.isDir, result, tt.expected)
			}
		})
	}
}

func TestIgnorePatternMatcher_GetPatterns(t *testing.T) {
	patterns := `# Comment
*.log
.git/

node_modules/`

	matcher := NewIgnorePatternMatcher(patterns, nil)
	result := matcher.GetPatterns()

	expected := []string{"*.log", ".git/", "node_modules/"}
	if len(result) != len(expected) {
		t.Errorf("GetPatterns() returned %d patterns, want %d", len(result), len(expected))
	}

	for i, pattern := range expected {
		if result[i] != pattern {
			t.Errorf("GetPatterns()[%d] = %q, want %q", i, result[i], pattern)
		}
	}
}

func TestIgnorePatternMatcher_EmptyPatterns(t *testing.T) {
	matcher := NewIgnorePatternMatcher("", nil)
	result := matcher.ShouldIgnore("anything.txt", false)
	if result {
		t.Error("Empty patterns should not ignore anything")
	}
}
