package app

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/gen2brain/go-fitz"
)

const (
	maxTextFileSize  = 50 * 1024        // 50KB for text files
	maxImageFileSize = 5 * 1024 * 1024  // 5MB for images
	maxPDFFileSize   = 50 * 1024 * 1024 // 50MB for PDFs
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
	case "pdf":
		return das.analyzePDFFile(filePath)
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

// analyzePDFFile converts PDF pages to images and analyzes them
func (das *DeepAnalysisService) analyzePDFFile(filePath string) (string, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return "", err
	}

	// Skip very large PDFs
	if info.Size() > maxPDFFileSize {
		return fmt.Sprintf("Large PDF file (%d bytes)", info.Size()), nil
	}

	// Open PDF using go-fitz (cross-platform, no external dependencies)
	doc, err := fitz.New(filePath)
	if err != nil {
		das.logger.Debug("Failed to open PDF with go-fitz: %v", err)
		return fmt.Sprintf("PDF file: %s", filepath.Base(filePath)), nil
	}
	defer doc.Close()

	totalPages := doc.NumPage()
	das.logger.Debug("Successfully opened PDF: %s (%d pages)", filePath, totalPages)

	// Process first 4 pages only
	maxPages := 4
	if totalPages < maxPages {
		maxPages = totalPages
	}

	// Convert pages to images and encode to base64
	var imageContents []map[string]interface{}
	for pageNum := 0; pageNum < maxPages; pageNum++ {
		// Render page to image at 150 DPI (default is 72)
		// go-fitz uses a zoom factor: 150 DPI = 150/72 = 2.08 zoom
		img, err := doc.Image(pageNum)
		if err != nil {
			das.logger.Debug("Failed to render page %d: %v", pageNum+1, err)
			continue
		}

		// Encode image to PNG in memory
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			das.logger.Debug("Failed to encode page %d to PNG: %v", pageNum+1, err)
			continue
		}

		imageData := buf.Bytes()

		// DEBUG: Save a copy of the converted image for inspection
		debugPath := filepath.Join(os.TempDir(), fmt.Sprintf("pdf-debug-%s-page%d.png", filepath.Base(filePath), pageNum+1))
		if err := os.WriteFile(debugPath, imageData, 0644); err == nil {
			das.logger.Debug("DEBUG: Saved converted PDF page to: %s", debugPath)
		}

		// Encode to base64 for API
		base64Image := base64.StdEncoding.EncodeToString(imageData)
		imageContents = append(imageContents, map[string]interface{}{
			"type": "image_url",
			"image_url": map[string]string{
				"url": fmt.Sprintf("data:image/png;base64,%s", base64Image),
			},
		})
	}

	if len(imageContents) == 0 {
		return fmt.Sprintf("PDF file: %s", filepath.Base(filePath)), nil
	}

	das.logger.Debug("Successfully converted %d pages from PDF: %s", len(imageContents), filePath)

	// Use multimodal LLM to analyze all pages together
	description, err := das.analyzePDFWithLLM(imageContents, filepath.Base(filePath), totalPages)
	if err != nil {
		das.logger.Debug("Failed to analyze PDF file %s: %v", filePath, err)
		return fmt.Sprintf("PDF file with %d pages: %s", totalPages, filepath.Base(filePath)), nil
	}

	return description, nil
}

// analyzePDFWithLLM sends multiple PDF page images to multimodal LLM for analysis
func (das *DeepAnalysisService) analyzePDFWithLLM(imageContents []map[string]interface{}, fileName string, totalPages int) (string, error) {
	systemPrompt := `You are a precise document analysis assistant. Your task is to analyze PDF page images and describe ONLY what you can actually see in them.

CRITICAL RULES:
- Only describe content that is clearly visible in the provided images
- If images are unclear, blurry, or unreadable, state that explicitly
- Do NOT make assumptions about content you cannot see
- Do NOT invent details that aren't present
- Focus on: document type, main topic, visible headings, key sections, and purpose
- Be factual and specific, citing visible elements (e.g., "shows a table with X columns", "contains section titled Y")
- Maximum 3 sentences

If the images are too low quality to read, respond with: "Unable to analyze - images are not clear enough to read text reliably."`

	// Build the user message with text followed by all images
	userText := fmt.Sprintf("Document filename: %s\nPages shown: %d of %d total\n\nDescribe ONLY what you can clearly see in these page images. Do not speculate or infer content you cannot directly observe:", fileName, len(imageContents), totalPages)

	// Create content array starting with text
	contentArray := []map[string]interface{}{
		{
			"type": "text",
			"text": userText,
		},
	}

	// Add all images
	contentArray = append(contentArray, imageContents...)

	reqBody := map[string]interface{}{
		"model": das.config.Model,
		"messages": []map[string]interface{}{
			{
				"role":    "system",
				"content": systemPrompt,
			},
			{
				"role":    "user",
				"content": contentArray,
			},
		},
		"max_tokens":  200,
		"temperature": 0.3, // Lower temperature for more factual, less creative responses
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
		summary := strings.TrimSpace(response.Choices[0].Message.Content)

		// Validate that the response is not empty or generic
		if summary == "" {
			return "", fmt.Errorf("LLM returned empty response")
		}

		// Check for common hallucination patterns - responses that are too generic or unrelated
		lowerSummary := strings.ToLower(summary)
		suspiciousPatterns := []string{
			"i cannot", "i can't", "i'm unable", "i don't have access",
			"as an ai", "as a language model", "i apologize",
		}
		for _, pattern := range suspiciousPatterns {
			if strings.Contains(lowerSummary, pattern) {
				das.logger.Debug("LLM response contains refusal pattern: %s", summary)
				return fmt.Sprintf("PDF document: %s (%d pages)", fileName, totalPages), nil
			}
		}

		// Warn if response seems unrelated to document analysis
		if !strings.Contains(lowerSummary, "document") && !strings.Contains(lowerSummary, "page") &&
			!strings.Contains(lowerSummary, "shows") && !strings.Contains(lowerSummary, "contains") &&
			!strings.Contains(lowerSummary, "pdf") && !strings.Contains(lowerSummary, "file") &&
			!strings.Contains(lowerSummary, "text") && len(summary) < 20 {
			das.logger.Debug("LLM response seems unrelated or too short: %s", summary)
			return fmt.Sprintf("PDF document: %s (%d pages)", fileName, totalPages), nil
		}

		// Add page count info if analyzing subset
		if totalPages > len(imageContents) {
			summary = fmt.Sprintf("%s [Analyzed %d of %d pages]", summary, len(imageContents), totalPages)
		}
		return summary, nil
	}

	return "", fmt.Errorf("no response from LLM")
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
	systemPrompt := `You are a precise image analysis assistant. Describe ONLY what you can actually see in the image.

RULES:
- Describe visible subjects, objects, scenes, and composition
- If the image contains text, mention it (e.g., "screenshot of code", "diagram with labels")
- Be specific and factual (e.g., "photo of a red car on a highway", not "transportation image")
- If unclear or corrupted, state "Image is unclear or corrupted"
- Do NOT invent details you cannot see
- Maximum 100 characters`

	// Create multimodal message with image
	userText := fmt.Sprintf("Image: %s\n\nDescribe only what is clearly visible:", fileName)
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
		"max_tokens":  100,
		"temperature": 0.3, // Lower temperature for more factual responses
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
		description := strings.TrimSpace(response.Choices[0].Message.Content)

		// Basic validation - reject obvious hallucinations
		if description == "" {
			return "", fmt.Errorf("LLM returned empty response")
		}

		return description, nil
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
	ext := strings.ToLower(filepath.Ext(filePath))
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
	case ".pdf":
		return "pdf"
	case ".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx":
		return "document"
	default:
		return "other"
	}
}
