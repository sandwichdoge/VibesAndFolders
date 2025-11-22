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
	systemPrompt := `You are a file analysis assistant. Analyze the provided file content and provide a concise, description (max 3 sentences) that captures the main purpose or content of the file. Be specific and informative.`

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

