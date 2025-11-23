# Ignore Patterns Documentation

## Overview

VibesAndFolders supports `.gitignore`-style patterns to exclude files and directories from analysis and indexing. This document explains how patterns work and their behavior.

## How It Works

### Two Levels of Filtering

1. **Indexing (Deep Analysis)**: Files matching ignore patterns are completely skipped during indexing - they won't be analyzed or stored in the database.

2. **Directory Analysis (LLM Context)**:
   - Ignored **directories** appear by name (e.g., `.git/`) to provide context, but their contents are omitted
   - Ignored **files** are completely omitted from the structure
   - No `[ignored]` labels or confusing markers - clean output

### Configuration

Ignore patterns are configured in **Settings → Configure → Ignore Patterns** tab. They are stored in your config file and applied automatically at startup.

## Pattern Syntax

### Basic Patterns

```bash
# Comments start with #
*.log           # Ignore all .log files anywhere
.DS_Store       # Ignore specific filename
Thumbs.db       # Ignore specific filename
```

### Directory Patterns (trailing slash)

```bash
.git/           # Ignore .git directory AND all its contents
node_modules/   # Ignore node_modules directory AND all its contents
build/          # Ignore build directory AND all its contents
```

**Important**: Directory patterns (ending with `/`) will:
- Skip the directory during indexing (won't analyze any files inside)
- Show the directory name in the structure sent to the LLM (for context)
- Omit all files and subdirectories within that directory from the structure

### Recursive Patterns (doublestar)

```bash
**/.git/        # Ignore .git directories at any depth
**/*.tmp        # Ignore .tmp files at any depth
**/build/       # Ignore build directories at any depth
```

### Wildcard Patterns

```bash
*.exe           # All executable files
*.dll           # All DLL files
temp.*          # All files starting with 'temp'
test_*          # All files starting with 'test_'
```

## Pattern Behavior Examples

### Example 1: Basic Directory Ignore

**Pattern**: `.git/`

**Result**:
- `.git/` directory **appears in the structure** (gives LLM context that it exists)
- Files like `.git/config`, `.git/HEAD` are NOT indexed
- Files like `.git/config` are NOT shown in directory structure
- The LLM sees `.git/` exists but sees none of its contents

**Example Output**:
```
project/
  .git/
  src/
    main.go (1234 bytes)
  README.md (567 bytes)
```

### Example 2: Wildcard File Pattern

**Pattern**: `*.log`

**Result**:
- Files like `debug.log`, `error.log`, `app.log` are skipped
- Works at any depth (e.g., `logs/debug.log` is also skipped)
- Directories named `*.log` are also matched by the glob

### Example 3: Nested Directory Ignore

**Pattern**: `**/node_modules/`

**Result**:
- Ignores `node_modules/` at root
- Also ignores `packages/app/node_modules/`
- Also ignores `src/vendor/node_modules/`

## Default Ignore Patterns

The following patterns are included by default:

```
# Version control
.git/
.svn/

# Dependencies
node_modules/
vendor/

# Build outputs
build/
dist/
*.exe
*.dll
*.so
*.dylib

# OS files
.DS_Store
Thumbs.db
Desktop.ini

# Temporary files
*.tmp
*.temp
*.log
*.cache
```

## FAQ

### Q: Will ignored directories appear in the LLM prompt?

**A: Yes, but only the directory name.** Ignored directories appear by name (e.g., `.git/`, `node_modules/`) so the LLM has context about what exists, but all files and subdirectories inside are omitted. This gives clean context without cluttering the prompt with thousands of irrelevant files.

### Q: Can I use `.git/**` pattern?

**A: You can, but `.git/` is simpler.**
- `.git/` - Ignores the `.git` directory at the root level
- `**/.git/` - Ignores any `.git` directory at any depth in the tree

The `**` pattern is for matching at any depth (e.g., `**/.git/` matches `.git` anywhere in nested directories).

### Q: Do patterns affect file operations?

**A: No**. Ignore patterns only affect:
1. What gets indexed for deep analysis
2. What appears in the directory structure sent to the LLM

When you execute file operations (move/rename), the patterns don't interfere.

### Q: Can I temporarily disable ignore patterns?

**A: Yes**. Go to Settings → Configure → Ignore Patterns and clear the text box, then save.

## Pattern Matching Library

VibesAndFolders uses [bmatcuk/doublestar/v4](https://github.com/bmatcuk/doublestar) for pattern matching, which provides:
- Full glob support with `*` and `?`
- Doublestar `**` for recursive matching
- High performance
- Compatibility with standard `.gitignore` patterns

## Technical Details

### Performance

- Ignored directories use `filepath.SkipDir` for efficient traversal
- The walker stops descending into ignored directories immediately
- Large ignored directories (like `node_modules/`) significantly speed up analysis

### Pattern Matching

Patterns are matched against paths relative to the root directory being analyzed:
- Paths use forward slashes (`/`) regardless of OS
- Directory patterns must end with `/`
- Patterns are case-sensitive on Linux/macOS, case-insensitive on Windows (following OS behavior)

## See Also

- [Main README](README.md)
- [Configuration Guide](internal/app/config.go)
- [Pattern Matcher Implementation](internal/app/ignore_patterns.go)
