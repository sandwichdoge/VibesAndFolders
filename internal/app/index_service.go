package app

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)


// IndexedFile represents a file record in the database
type IndexedFile struct {
	ID            int64
	FilePath      string
	Description   string
	FileType      string // "text", "image", "video", "audio", "other"
	FileSize      int64
	LastModified  time.Time
	IndexedAt     time.Time
	UpdatedAt     time.Time
	SymlinkTarget string // For symlinks, stores the target path
}

// IndexService handles file indexing and tracking
type IndexService interface {
	// Initialize the database
	Initialize(dbPath string) error
	Close() error

	// Check if a file is indexed and up-to-date
	IsFileIndexed(filePath string) (bool, error)
	NeedsReindexing(filePath string) (bool, error)

	// Get indexed file info
	GetIndexedFile(filePath string) (*IndexedFile, error)

	// Add or update file index
	IndexFile(filePath, description, fileType string, fileSize int64, lastModified time.Time) error
	IndexFileWithSymlink(filePath, description, fileType string, fileSize int64, lastModified time.Time, symlinkTarget string) error
	UpdateFileIndex(filePath, description string, lastModified time.Time) error

	// Update file path in index (for moves/renames) without re-analyzing
	UpdateFilePath(oldPath, newPath string) error
	UpdateFilePathWithSymlink(oldPath, newPath, newSymlinkTarget string) error

	// Remove file from index (for deleted files)
	RemoveFile(filePath string) error

	// Get all indexed files in a directory
	GetIndexedFilesInDirectory(dirPath string) ([]IndexedFile, error)

	// Scan directory and identify changes
	ScanDirectoryChanges(dirPath string) (*DirectoryChanges, error)

	// Transaction support for atomic operations
	BeginTransaction() error
	CommitTransaction() error
	RollbackTransaction() error

	// Create a snapshot of file paths for rollback
	CreateSnapshot(operations []FileOperation) (*IndexSnapshot, error)
	RestoreSnapshot(snapshot *IndexSnapshot) error

	// Validate and clean orphaned entries
	ValidateIndex() ([]string, error)
	RemoveOrphanedEntries(dirPath string) (int, error)
}

// DirectoryChanges tracks what has changed in a directory
type DirectoryChanges struct {
	NewFiles       []string
	DeletedFiles   []string
	ModifiedFiles  []string
	UnchangedFiles []string
}

// IndexSnapshot stores index state for rollback capability
type IndexSnapshot struct {
	Entries map[string]*IndexedFile // key is file path
}

// DefaultIndexService implements IndexService
type DefaultIndexService struct {
	db     *sql.DB
	tx     *sql.Tx
	logger *Logger
}

func NewIndexService(logger *Logger) *DefaultIndexService {
	return &DefaultIndexService{
		logger: logger,
	}
}

func (is *DefaultIndexService) Initialize(dbPath string) error {
	// Ensure the directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create database directory: %w", err)
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	is.db = db

	// Create the schema
	schema := `
	CREATE TABLE IF NOT EXISTS indexed_files (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		file_path TEXT UNIQUE NOT NULL,
		description TEXT,
		file_type TEXT NOT NULL,
		file_size INTEGER NOT NULL,
		last_modified INTEGER NOT NULL,
		indexed_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		symlink_target TEXT
	);

	CREATE INDEX IF NOT EXISTS idx_file_path ON indexed_files(file_path);
	CREATE INDEX IF NOT EXISTS idx_file_type ON indexed_files(file_type);
	CREATE INDEX IF NOT EXISTS idx_updated_at ON indexed_files(updated_at);
	`

	if _, err := db.Exec(schema); err != nil {
		return fmt.Errorf("failed to create schema: %w", err)
	}

	is.logger.Info("Index database initialized at %s", dbPath)
	return nil
}

func (is *DefaultIndexService) Close() error {
	if is.db != nil {
		return is.db.Close()
	}
	return nil
}

func (is *DefaultIndexService) IsFileIndexed(filePath string) (bool, error) {
	var count int
	err := is.db.QueryRow("SELECT COUNT(*) FROM indexed_files WHERE file_path = ?", filePath).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (is *DefaultIndexService) NeedsReindexing(filePath string) (bool, error) {
	indexed, err := is.IsFileIndexed(filePath)
	if err != nil {
		return false, err
	}
	if !indexed {
		return true, nil
	}

	// Get current file modification time
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return false, err
	}
	currentModTime := fileInfo.ModTime().Unix()

	// Get stored modification time
	var storedModTime int64
	err = is.db.QueryRow("SELECT last_modified FROM indexed_files WHERE file_path = ?", filePath).Scan(&storedModTime)
	if err != nil {
		return false, err
	}

	// If modification times differ, needs reindexing
	return currentModTime != storedModTime, nil
}

func (is *DefaultIndexService) GetIndexedFile(filePath string) (*IndexedFile, error) {
	var file IndexedFile
	var lastModUnix int64
	var symlinkTarget sql.NullString
	err := is.db.QueryRow(`
		SELECT id, file_path, description, file_type, file_size, last_modified, indexed_at, updated_at, symlink_target
		FROM indexed_files WHERE file_path = ?
	`, filePath).Scan(
		&file.ID, &file.FilePath, &file.Description,
		&file.FileType, &file.FileSize, &lastModUnix, &file.IndexedAt, &file.UpdatedAt, &symlinkTarget,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	file.LastModified = time.Unix(lastModUnix, 0)
	if symlinkTarget.Valid {
		file.SymlinkTarget = symlinkTarget.String
	}
	return &file, nil
}

func (is *DefaultIndexService) IndexFile(filePath, description, fileType string, fileSize int64, lastModified time.Time) error {
	return is.IndexFileWithSymlink(filePath, description, fileType, fileSize, lastModified, "")
}

func (is *DefaultIndexService) IndexFileWithSymlink(filePath, description, fileType string, fileSize int64, lastModified time.Time, symlinkTarget string) error {
	var symlinkTargetVal interface{}
	if symlinkTarget == "" {
		symlinkTargetVal = nil
	} else {
		symlinkTargetVal = symlinkTarget
	}

	_, err := is.db.Exec(`
		INSERT INTO indexed_files (file_path, description, file_type, file_size, last_modified, indexed_at, updated_at, symlink_target)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(file_path) DO UPDATE SET
			description = excluded.description,
			file_type = excluded.file_type,
			file_size = excluded.file_size,
			last_modified = excluded.last_modified,
			updated_at = excluded.updated_at,
			symlink_target = excluded.symlink_target
	`, filePath, description, fileType, fileSize, lastModified.Unix(), time.Now(), time.Now(), symlinkTargetVal)
	return err
}

func (is *DefaultIndexService) UpdateFileIndex(filePath, description string, lastModified time.Time) error {
	_, err := is.db.Exec(`
		UPDATE indexed_files
		SET description = ?, last_modified = ?, updated_at = ?
		WHERE file_path = ?
	`, description, lastModified.Unix(), time.Now(), filePath)
	return err
}

func (is *DefaultIndexService) UpdateFilePath(oldPath, newPath string) error {
	// Get the new file's modification time and size
	fileInfo, err := os.Lstat(newPath) // Use Lstat to handle symlinks
	if err != nil {
		return fmt.Errorf("failed to stat new file path: %w", err)
	}

	// Check if it's a symlink and read the target
	var symlinkTarget string
	if fileInfo.Mode()&os.ModeSymlink != 0 {
		symlinkTarget, err = os.Readlink(newPath)
		if err != nil {
			return fmt.Errorf("failed to read symlink: %w", err)
		}
	}

	return is.UpdateFilePathWithSymlink(oldPath, newPath, symlinkTarget)
}

func (is *DefaultIndexService) UpdateFilePathWithSymlink(oldPath, newPath, newSymlinkTarget string) error {
	// Get the new file's modification time and size
	fileInfo, err := os.Lstat(newPath)
	if err != nil {
		return fmt.Errorf("failed to stat new file path: %w", err)
	}

	var symlinkTargetVal interface{}
	if newSymlinkTarget == "" {
		symlinkTargetVal = nil
	} else {
		symlinkTargetVal = newSymlinkTarget
	}

	_, err = is.db.Exec(`
		UPDATE indexed_files
		SET file_path = ?, file_size = ?, last_modified = ?, updated_at = ?, symlink_target = ?
		WHERE file_path = ?
	`, newPath, fileInfo.Size(), fileInfo.ModTime().Unix(), time.Now(), symlinkTargetVal, oldPath)
	return err
}

func (is *DefaultIndexService) RemoveFile(filePath string) error {
	_, err := is.db.Exec("DELETE FROM indexed_files WHERE file_path = ?", filePath)
	return err
}

func (is *DefaultIndexService) GetIndexedFilesInDirectory(dirPath string) ([]IndexedFile, error) {
	// Use LIKE to match all files under the directory
	// Ensure dirPath ends with separator to avoid matching similar prefixes
	// e.g., "/home/user/doc" shouldn't match "/home/user/documents"
	pattern := filepath.Clean(dirPath)
	if !strings.HasSuffix(pattern, string(filepath.Separator)) {
		pattern += string(filepath.Separator)
	}
	pattern += "%"

	rows, err := is.db.Query(`
		SELECT id, file_path, description, file_type, file_size, last_modified, indexed_at, updated_at, symlink_target
		FROM indexed_files WHERE file_path LIKE ? OR file_path = ?
	`, pattern, filepath.Clean(dirPath))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []IndexedFile
	for rows.Next() {
		var file IndexedFile
		var lastModUnix int64
		var symlinkTarget sql.NullString
		err := rows.Scan(
			&file.ID, &file.FilePath, &file.Description,
			&file.FileType, &file.FileSize, &lastModUnix, &file.IndexedAt, &file.UpdatedAt, &symlinkTarget,
		)
		if err != nil {
			return nil, err
		}
		file.LastModified = time.Unix(lastModUnix, 0)
		if symlinkTarget.Valid {
			file.SymlinkTarget = symlinkTarget.String
		}
		files = append(files, file)
	}
	return files, rows.Err()
}

func (is *DefaultIndexService) ScanDirectoryChanges(dirPath string) (*DirectoryChanges, error) {
	changes := &DirectoryChanges{
		NewFiles:      make([]string, 0),
		DeletedFiles:  make([]string, 0),
		ModifiedFiles: make([]string, 0),
		UnchangedFiles: make([]string, 0),
	}

	// Get all indexed files in this directory
	indexedFiles, err := is.GetIndexedFilesInDirectory(dirPath)
	if err != nil {
		return nil, err
	}

	// Create a map of indexed file paths for quick lookup
	indexedMap := make(map[string]IndexedFile)
	for _, file := range indexedFiles {
		indexedMap[file.FilePath] = file
	}

	// Walk the directory to find current files
	currentFiles := make(map[string]bool)
	err = filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		currentFiles[path] = true

		// Check if file is indexed
		if _, exists := indexedMap[path]; exists {
			// File exists in index, check if modified
			needsReindex, err := is.NeedsReindexing(path)
			if err != nil {
				is.logger.Debug("Error checking if file needs reindexing: %v", err)
				return nil
			}
			if needsReindex {
				changes.ModifiedFiles = append(changes.ModifiedFiles, path)
			} else {
				changes.UnchangedFiles = append(changes.UnchangedFiles, path)
			}
		} else {
			// New file
			changes.NewFiles = append(changes.NewFiles, path)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	// Check for deleted files
	for path := range indexedMap {
		if !currentFiles[path] {
			changes.DeletedFiles = append(changes.DeletedFiles, path)
		}
	}

	return changes, nil
}

// BeginTransaction starts a database transaction
func (is *DefaultIndexService) BeginTransaction() error {
	if is.tx != nil {
		return fmt.Errorf("transaction already in progress")
	}
	tx, err := is.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	is.tx = tx
	is.logger.Debug("Index transaction started")
	return nil
}

// CommitTransaction commits the current transaction
func (is *DefaultIndexService) CommitTransaction() error {
	if is.tx == nil {
		return fmt.Errorf("no transaction in progress")
	}
	err := is.tx.Commit()
	is.tx = nil
	if err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	is.logger.Debug("Index transaction committed")
	return nil
}

// RollbackTransaction rolls back the current transaction
func (is *DefaultIndexService) RollbackTransaction() error {
	if is.tx == nil {
		return fmt.Errorf("no transaction in progress")
	}
	err := is.tx.Rollback()
	is.tx = nil
	if err != nil {
		return fmt.Errorf("failed to rollback transaction: %w", err)
	}
	is.logger.Debug("Index transaction rolled back")
	return nil
}

// CreateSnapshot creates a snapshot of affected files for rollback
func (is *DefaultIndexService) CreateSnapshot(operations []FileOperation) (*IndexSnapshot, error) {
	snapshot := &IndexSnapshot{
		Entries: make(map[string]*IndexedFile),
	}

	for _, op := range operations {
		// Store the current state of the source file
		file, err := is.GetIndexedFile(op.From)
		if err != nil {
			return nil, fmt.Errorf("failed to get indexed file %s: %w", op.From, err)
		}
		if file != nil {
			snapshot.Entries[op.From] = file
		}

		// Also check if destination exists (in case of overwrites)
		destFile, err := is.GetIndexedFile(op.To)
		if err != nil {
			return nil, fmt.Errorf("failed to get indexed file %s: %w", op.To, err)
		}
		if destFile != nil {
			snapshot.Entries[op.To] = destFile
		}
	}

	is.logger.Debug("Created index snapshot with %d entries", len(snapshot.Entries))
	return snapshot, nil
}

// RestoreSnapshot restores index state from a snapshot
func (is *DefaultIndexService) RestoreSnapshot(snapshot *IndexSnapshot) error {
	if snapshot == nil {
		return fmt.Errorf("snapshot is nil")
	}

	// Start a transaction for atomic restore
	if err := is.BeginTransaction(); err != nil {
		return err
	}

	for path, file := range snapshot.Entries {
		if file != nil {
			// Restore the file entry
			err := is.IndexFile(file.FilePath, file.Description, file.FileType, file.FileSize, file.LastModified)
			if err != nil {
				is.RollbackTransaction()
				return fmt.Errorf("failed to restore file %s: %w", path, err)
			}
		} else {
			// File didn't exist, remove it
			err := is.RemoveFile(path)
			if err != nil {
				is.RollbackTransaction()
				return fmt.Errorf("failed to remove file %s during restore: %w", path, err)
			}
		}
	}

	if err := is.CommitTransaction(); err != nil {
		return err
	}

	is.logger.Info("Restored index snapshot with %d entries", len(snapshot.Entries))
	return nil
}

// ValidateIndex checks for orphaned entries and returns their paths
func (is *DefaultIndexService) ValidateIndex() ([]string, error) {
	rows, err := is.db.Query("SELECT file_path FROM indexed_files")
	if err != nil {
		return nil, fmt.Errorf("failed to query indexed files: %w", err)
	}
	defer rows.Close()

	var orphaned []string
	for rows.Next() {
		var filePath string
		if err := rows.Scan(&filePath); err != nil {
			return nil, fmt.Errorf("failed to scan file path: %w", err)
		}

		// Check if file exists
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			orphaned = append(orphaned, filePath)
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	is.logger.Debug("Found %d orphaned index entries", len(orphaned))
	return orphaned, nil
}

// RemoveOrphanedEntries removes index entries for files that no longer exist
func (is *DefaultIndexService) RemoveOrphanedEntries(dirPath string) (int, error) {
	// Get all indexed files in directory
	indexedFiles, err := is.GetIndexedFilesInDirectory(dirPath)
	if err != nil {
		return 0, fmt.Errorf("failed to get indexed files: %w", err)
	}

	removed := 0
	for _, file := range indexedFiles {
		// Check if file exists
		if _, err := os.Stat(file.FilePath); os.IsNotExist(err) {
			if err := is.RemoveFile(file.FilePath); err != nil {
				is.logger.Error("Failed to remove orphaned entry %s: %v", file.FilePath, err)
			} else {
				removed++
				is.logger.Debug("Removed orphaned entry: %s", file.FilePath)
			}
		}
	}

	is.logger.Info("Removed %d orphaned entries from %s", removed, dirPath)
	return removed, nil
}

// DetermineFileType determines the type of file based on extension
func DetermineFileType(filePath string) string {
	ext := filepath.Ext(filePath)
	switch ext {
	case ".txt", ".md", ".json", ".xml", ".yaml", ".yml", ".toml", ".ini", ".cfg", ".conf":
		return "text"
	case ".go", ".py", ".js", ".ts", ".java", ".c", ".cpp", ".h", ".hpp", ".rs", ".rb", ".php", ".sh", ".bash":
		return "text"
	case ".jpg", ".jpeg", ".png", ".gif", ".bmp", ".svg", ".webp", ".ico":
		return "image"
	case ".mp4", ".avi", ".mkv", ".mov", ".wmv", ".flv", ".webm":
		return "video"
	case ".mp3", ".wav", ".flac", ".aac", ".ogg", ".wma", ".m4a":
		return "audio"
	case ".pdf", ".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx":
		return "document"
	default:
		return "other"
	}
}
