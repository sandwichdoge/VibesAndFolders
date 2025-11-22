package app

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
)


// IndexedFile represents a file record in the database
type IndexedFile struct {
	ID           int64
	FilePath     string
	Description  string
	FileType     string // "text", "image", "video", "audio", "other"
	FileSize     int64
	LastModified time.Time
	IndexedAt    time.Time
	UpdatedAt    time.Time
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
	UpdateFileIndex(filePath, description string, lastModified time.Time) error

	// Update file path in index (for moves/renames) without re-analyzing
	UpdateFilePath(oldPath, newPath string) error

	// Remove file from index (for deleted files)
	RemoveFile(filePath string) error

	// Get all indexed files in a directory
	GetIndexedFilesInDirectory(dirPath string) ([]IndexedFile, error)

	// Scan directory and identify changes
	ScanDirectoryChanges(dirPath string) (*DirectoryChanges, error)
}

// DirectoryChanges tracks what has changed in a directory
type DirectoryChanges struct {
	NewFiles     []string
	DeletedFiles []string
	ModifiedFiles []string
	UnchangedFiles []string
}

// DefaultIndexService implements IndexService
type DefaultIndexService struct {
	db     *sql.DB
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
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
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
	err := is.db.QueryRow(`
		SELECT id, file_path, description, file_type, file_size, last_modified, indexed_at, updated_at
		FROM indexed_files WHERE file_path = ?
	`, filePath).Scan(
		&file.ID, &file.FilePath, &file.Description,
		&file.FileType, &file.FileSize, &lastModUnix, &file.IndexedAt, &file.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	file.LastModified = time.Unix(lastModUnix, 0)
	return &file, nil
}

func (is *DefaultIndexService) IndexFile(filePath, description, fileType string, fileSize int64, lastModified time.Time) error {
	_, err := is.db.Exec(`
		INSERT INTO indexed_files (file_path, description, file_type, file_size, last_modified, indexed_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(file_path) DO UPDATE SET
			description = excluded.description,
			file_type = excluded.file_type,
			file_size = excluded.file_size,
			last_modified = excluded.last_modified,
			updated_at = excluded.updated_at
	`, filePath, description, fileType, fileSize, lastModified.Unix(), time.Now(), time.Now())
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
	fileInfo, err := os.Stat(newPath)
	if err != nil {
		return fmt.Errorf("failed to stat new file path: %w", err)
	}

	_, err = is.db.Exec(`
		UPDATE indexed_files
		SET file_path = ?, file_size = ?, last_modified = ?, updated_at = ?
		WHERE file_path = ?
	`, newPath, fileInfo.Size(), fileInfo.ModTime().Unix(), time.Now(), oldPath)
	return err
}

func (is *DefaultIndexService) RemoveFile(filePath string) error {
	_, err := is.db.Exec("DELETE FROM indexed_files WHERE file_path = ?", filePath)
	return err
}

func (is *DefaultIndexService) GetIndexedFilesInDirectory(dirPath string) ([]IndexedFile, error) {
	// Use LIKE to match all files under the directory
	pattern := dirPath + "%"
	rows, err := is.db.Query(`
		SELECT id, file_path, description, file_type, file_size, last_modified, indexed_at, updated_at
		FROM indexed_files WHERE file_path LIKE ?
	`, pattern)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []IndexedFile
	for rows.Next() {
		var file IndexedFile
		var lastModUnix int64
		err := rows.Scan(
			&file.ID, &file.FilePath, &file.Description,
			&file.FileType, &file.FileSize, &lastModUnix, &file.IndexedAt, &file.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		file.LastModified = time.Unix(lastModUnix, 0)
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
