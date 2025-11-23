package app

import (
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// IgnorePatternMatcher handles file/directory ignore patterns
type IgnorePatternMatcher struct {
	patterns []string
	logger   *Logger
}

// NewIgnorePatternMatcher creates a new pattern matcher from a multiline string
func NewIgnorePatternMatcher(patternsText string, logger *Logger) *IgnorePatternMatcher {
	matcher := &IgnorePatternMatcher{
		patterns: make([]string, 0),
		logger:   logger,
	}

	// Parse patterns line by line
	lines := strings.Split(patternsText, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		matcher.patterns = append(matcher.patterns, line)
	}

	if logger != nil {
		logger.Debug("Loaded %d ignore patterns", len(matcher.patterns))
	}

	return matcher
}

// ShouldIgnore checks if a path should be ignored based on the patterns
// path should be relative to the base directory and use forward slashes
func (m *IgnorePatternMatcher) ShouldIgnore(path string, isDir bool) bool {
	if len(m.patterns) == 0 {
		return false
	}

	// Normalize path to use forward slashes
	path = filepath.ToSlash(path)

	for _, pattern := range m.patterns {
		// Check if pattern is meant for directories only (ends with /)
		isDirPattern := strings.HasSuffix(pattern, "/")
		patternWithoutSlash := strings.TrimSuffix(pattern, "/")

		if isDirPattern {
			// Directory-specific pattern
			// Check if this directory matches at the root level
			if isDir && (path == patternWithoutSlash || strings.HasPrefix(path, patternWithoutSlash+"/")) {
				return true
			}

			// Check if this is a file inside an ignored directory (at root level)
			if !isDir && strings.HasPrefix(path, patternWithoutSlash+"/") {
				return true
			}

			// If pattern doesn't contain wildcards, also check if it matches the directory name at ANY level
			// For example, ".git/" should match both ".git" and "backup/.git"
			if !strings.Contains(patternWithoutSlash, "*") && !strings.Contains(patternWithoutSlash, "?") {
				pathParts := strings.Split(path, "/")
				for i, part := range pathParts {
					if part == patternWithoutSlash {
						// This directory matches the pattern
						if isDir && i == len(pathParts)-1 {
							// This is the directory itself
							return true
						}
						// This is a file inside the ignored directory (or the directory is an ancestor)
						return true
					}
				}
			}

			// Also try glob matching for directory patterns with wildcards
			matched, err := doublestar.Match(patternWithoutSlash, path)
			if err == nil && matched {
				return true
			}
		} else {
			// Regular pattern (not directory-specific)
			// Use doublestar for glob matching
			matched, err := doublestar.Match(pattern, path)
			if err != nil {
				if m.logger != nil {
					m.logger.Debug("Error matching pattern %s: %v", pattern, err)
				}
				continue
			}

			if matched {
				return true
			}
		}
	}

	return false
}

// GetPatterns returns the list of active patterns
func (m *IgnorePatternMatcher) GetPatterns() []string {
	return m.patterns
}
