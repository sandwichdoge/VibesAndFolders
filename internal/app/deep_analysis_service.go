package app

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	maxTextFileSize  = 50 * 1024       // 50KB for text files
	maxImageFileSize = 5 * 1024 * 1024 // 5MB for images
)

// DeepAnalysisService handles multimodal file analysis
type DeepAnalysisService struct {
	config       *Config
	httpClient   *HTTPClient
	indexService IndexService
	logger       *Logger
}

func NewDeepAnalysisService(config *Config, httpClient *HTTPClient, indexService IndexService, logger *Logger) *DeepAnalysisService {
	return &DeepAnalysisService{
		config:       config,
		httpClient:   httpClient,
		indexService: indexService,
		logger:       logger,
	}
}

// AnalyzeFile analyzes a single file and returns a description
func (das *DeepAnalysisService) AnalyzeFile(filePath string) (string, error) {
	fileType := DetermineFileType(filePath)

	switch fileType {
	case "text":
		return das.analyzeTextFile(filePath)
	case "image":
		return das.analyzeImageFile(filePath)
	default:
		return das.analyzeGenericFile(filePath)
	}
}

// analyzeTextFile reads and analyzes text content
func (das *DeepAnalysisService) analyzeTextFile(filePath string) (string, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return "", err
	}

	// Skip very large text files
	if info.Size() > maxTextFileSize {
		return fmt.Sprintf("Large text file (%d bytes)", info.Size()), nil
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}

	// Use LLM to analyze the text content
	description, err := das.analyzeContentWithLLM(string(content), "text", filepath.Base(filePath))
	if err != nil {
		das.logger.Debug("Failed to analyze text file %s: %v", filePath, err)
		// Fallback to basic description
		return fmt.Sprintf("Text file: %s", filepath.Base(filePath)), nil
	}

	return description, nil
}

// analyzeImageFile analyzes image using multimodal LLM
func (das *DeepAnalysisService) analyzeImageFile(filePath string) (string, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return "", err
	}

	// Skip very large images
	if info.Size() > maxImageFileSize {
		return fmt.Sprintf("Large image file (%d bytes)", info.Size()), nil
	}

	// Read and encode image to base64
	imageData, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}

	base64Image := base64.StdEncoding.EncodeToString(imageData)

	// Determine MIME type
	mimeType := das.getMimeType(filePath)

	// Use multimodal LLM to analyze the image
	description, err := das.analyzeImageWithLLM(base64Image, mimeType, filepath.Base(filePath))
	if err != nil {
		das.logger.Debug("Failed to analyze image file %s: %v", filePath, err)
		// Fallback to basic description
		return fmt.Sprintf("Image file: %s", filepath.Base(filePath)), nil
	}

	return description, nil
}

// analyzeGenericFile provides basic file information
func (das *DeepAnalysisService) analyzeGenericFile(filePath string) (string, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return "", err
	}

	fileType := DetermineFileType(filePath)
	return fmt.Sprintf("%s file: %s (%d bytes)", fileType, filepath.Base(filePath), info.Size()), nil
}

// analyzeContentWithLLM sends text content to LLM for analysis
func (das *DeepAnalysisService) analyzeContentWithLLM(content, contentType, fileName string) (string, error) {
	systemPrompt := `You are a file analysis assistant. Analyze the provided file content and provide a concise, description (max 100 lines) that captures the main purpose or content of the file. Be specific and informative.`

	userPrompt := fmt.Sprintf("File name: %s\nContent type: %s\n\nContent:\n%s\n\nProvide a brief description:", fileName, contentType, das.truncateContent(content, 2000))

	reqBody := OpenAIRequest{
		Model: das.config.Model,
		Messages: []Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		MaxTokens: 100,
		Stream:    false,
	}

	headers := map[string]string{
		"Authorization": fmt.Sprintf("Bearer %s", das.config.APIKey),
		"HTTP-Referer":  "https://github.com/sandwichdoge/vibesandfolders",
		"X-Title":       "VibesAndFolders",
	}

	body, err := das.httpClient.Post(das.config.Endpoint, headers, reqBody)
	if err != nil {
		return "", err
	}

	var response struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return "", err
	}

	if len(response.Choices) > 0 {
		return response.Choices[0].Message.Content, nil
	}

	return "", fmt.Errorf("no response from LLM")
}

// analyzeImageWithLLM sends image to multimodal LLM for analysis
func (das *DeepAnalysisService) analyzeImageWithLLM(base64Image, mimeType, fileName string) (string, error) {
	systemPrompt := `You are an image analysis assistant. Analyze the provided image and provide a concise, single-line description (max 100 characters) that captures what the image shows. Be specific and descriptive.`

	// Create multimodal message with image
	userText := fmt.Sprintf("Analyze this image (filename: %s) and provide only a brief description:", fileName)
	reqBody := map[string]interface{}{
		"model": das.config.Model,
		"messages": []map[string]interface{}{
			{
				"role":    "system",
				"content": systemPrompt,
			},
			{
				"role": "user",
				"content": []map[string]interface{}{
					{
						"type": "text",
						"text": userText,
					},
					{
						"type": "image_url",
						"image_url": map[string]string{
							"url": fmt.Sprintf("data:%s;base64,%s", mimeType, base64Image),
						},
					},
				},
			},
		},
		"max_tokens": 100,
	}

	headers := map[string]string{
		"Authorization": fmt.Sprintf("Bearer %s", das.config.APIKey),
		"HTTP-Referer":  "https://github.com/sandwichdoge/vibesandfolders",
		"X-Title":       "VibesAndFolders",
	}

	body, err := das.httpClient.Post(das.config.Endpoint, headers, reqBody)
	if err != nil {
		return "", err
	}

	var response struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return "", err
	}

	if len(response.Choices) > 0 {
		return response.Choices[0].Message.Content, nil
	}

	return "", fmt.Errorf("no response from LLM")
}

// IndexDirectory scans and indexes all files in a directory
func (das *DeepAnalysisService) IndexDirectory(dirPath string, onProgress func(current, total int, fileName string)) error {
	// First, scan for changes
	changes, err := das.indexService.ScanDirectoryChanges(dirPath)
	if err != nil {
		return fmt.Errorf("failed to scan directory changes: %w", err)
	}

	// Calculate total files to process
	totalFiles := len(changes.NewFiles) + len(changes.ModifiedFiles)
	if totalFiles == 0 {
		das.logger.Info("No files need indexing in %s", dirPath)
		return nil
	}

	das.logger.Info("Indexing directory: %s (%d new, %d modified, %d deleted)",
		dirPath, len(changes.NewFiles), len(changes.ModifiedFiles), len(changes.DeletedFiles))

	currentFile := 0

	// Process new files
	for _, filePath := range changes.NewFiles {
		currentFile++
		if onProgress != nil {
			onProgress(currentFile, totalFiles, filePath)
		}

		if err := das.indexFile(filePath); err != nil {
			das.logger.Error("Failed to index new file %s: %v", filePath, err)
		}
	}

	// Process modified files
	for _, filePath := range changes.ModifiedFiles {
		currentFile++
		if onProgress != nil {
			onProgress(currentFile, totalFiles, filePath)
		}

		if err := das.indexFile(filePath); err != nil {
			das.logger.Error("Failed to reindex modified file %s: %v", filePath, err)
		}
	}

	// Remove deleted files from index
	for _, filePath := range changes.DeletedFiles {
		if err := das.indexService.RemoveFile(filePath); err != nil {
			das.logger.Error("Failed to remove deleted file from index %s: %v", filePath, err)
		}
	}

	das.logger.Info("Directory indexing complete for %s", dirPath)
	return nil
}

// indexFile indexes a single file
func (das *DeepAnalysisService) indexFile(filePath string) error {
	// Get file info
	info, err := os.Stat(filePath)
	if err != nil {
		return err
	}

	// Determine file type
	fileType := DetermineFileType(filePath)

	// Analyze file to get description
	description, err := das.AnalyzeFile(filePath)
	if err != nil {
		das.logger.Debug("Failed to analyze file %s, using basic description: %v", filePath, err)
		description = fmt.Sprintf("%s file", fileType)
	}

	// Store in index with modification time
	if err := das.indexService.IndexFile(filePath, description, fileType, info.Size(), info.ModTime()); err != nil {
		return fmt.Errorf("failed to store file in index: %w", err)
	}

	das.logger.Debug("Indexed: %s - %s", filePath, description)
	return nil
}

// UpdateIndexAfterOperations updates the index smartly after file operations
// It only updates paths for known files and indexes new files
func (das *DeepAnalysisService) UpdateIndexAfterOperations(operations []FileOperation) error {
	for _, op := range operations {
		// Check if the old path was indexed
		indexed, err := das.indexService.IsFileIndexed(op.From)
		if err != nil {
			das.logger.Debug("Error checking if file is indexed %s: %v", op.From, err)
			continue
		}

		if indexed {
			// File was already indexed, just update the path
			if err := das.indexService.UpdateFilePath(op.From, op.To); err != nil {
				das.logger.Error("Failed to update file path in index %s -> %s: %v", op.From, op.To, err)
			} else {
				das.logger.Debug("Updated index path: %s -> %s", op.From, op.To)
			}
		} else {
			// File wasn't indexed before, index it now at the new location
			if err := das.indexFile(op.To); err != nil {
				das.logger.Error("Failed to index new file %s: %v", op.To, err)
			} else {
				das.logger.Debug("Indexed new file: %s", op.To)
			}
		}
	}
	return nil
}

// truncateContent truncates content to a maximum length
func (das *DeepAnalysisService) truncateContent(content string, maxLen int) string {
	if len(content) <= maxLen {
		return content
	}
	return content[:maxLen] + "..."
}

// getMimeType returns MIME type for common image formats
func (das *DeepAnalysisService) getMimeType(filePath string) string {
	ext := filepath.Ext(filePath)
	switch ext {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".bmp":
		return "image/bmp"
	case ".webp":
		return "image/webp"
	default:
		return "image/jpeg"
	}
}

// GetDirectoryIndexStats returns statistics about indexed files in a directory
func (das *DeepAnalysisService) GetDirectoryIndexStats(dirPath string) (map[string]int, error) {
	indexedFiles, err := das.indexService.GetIndexedFilesInDirectory(dirPath)
	if err != nil {
		return nil, err
	}

	stats := make(map[string]int)
	stats["total"] = len(indexedFiles)

	for _, file := range indexedFiles {
		stats[file.FileType]++
	}

	return stats, nil
}
