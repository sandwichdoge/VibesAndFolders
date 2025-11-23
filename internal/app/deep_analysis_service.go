package app

import (
	"archive/zip"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gen2brain/go-fitz"
	"github.com/nguyenthenguyen/docx"
	"github.com/xuri/excelize/v2"
)

const (
	maxTextFileSize       = 50 * 1024        // 50KB for text files
	maxImageFileSize      = 5 * 1024 * 1024  // 5MB for images
	maxPDFFileSize        = 50 * 1024 * 1024 // 50MB for PDFs
	maxExcelFileSize      = 50 * 1024 * 1024 // 50MB for Excel files
	maxDocFileSize        = 50 * 1024 * 1024 // 50MB for Word documents
	maxPowerPointFileSize = 50 * 1024 * 1024 // 50MB for PowerPoint files
	maxExcelRows          = 100              // Max rows per sheet to process
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
	case "excel":
		return das.analyzeExcelFile(filePath)
	case "document":
		return das.analyzeDocFile(filePath)
	case "powerpoint":
		return das.analyzePowerPointFile(filePath)
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
		return "", fmt.Errorf("text file too large (%d bytes)", info.Size())
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}

	// Use LLM to analyze the text content
	description, err := das.analyzeContentWithLLM(string(content), "text", filepath.Base(filePath))
	if err != nil {
		das.logger.Debug("Failed to analyze text file %s: %v", filePath, err)
		return "", fmt.Errorf("text analysis failed: %w", err)
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
		return "", fmt.Errorf("image file too large (%d bytes)", info.Size())
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
		// Return error so the file won't be indexed
		// This allows it to be re-analyzed when a multimodal model is configured
		return "", fmt.Errorf("image analysis failed (model may not support vision): %w", err)
	}

	return description, nil
}

// analyzeDocFile extracts text from Word documents and analyzes them
func (das *DeepAnalysisService) analyzeDocFile(filePath string) (string, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return "", err
	}

	// Skip very large Word documents
	if info.Size() > maxDocFileSize {
		return "", fmt.Errorf("Word document too large (%d bytes)", info.Size())
	}

	ext := strings.ToLower(filepath.Ext(filePath))

	// Only .docx is supported (modern Word format)
	// .doc (legacy binary format) requires platform-specific tools
	if ext == ".doc" {
		das.logger.Debug("Legacy .doc format not supported, skipping: %s", filePath)
		return "", fmt.Errorf("legacy .doc format not supported")
	}

	// Open .docx file
	doc, err := docx.ReadDocxFile(filePath)
	if err != nil {
		das.logger.Debug("Failed to open Word document %s: %v", filePath, err)
		return "", fmt.Errorf("failed to open Word document: %w", err)
	}
	defer doc.Close()

	// Extract XML content
	xmlContent := doc.Editable().GetContent()

	// Extract plain text from XML
	text := das.extractTextFromDocxXML(xmlContent)

	if text == "" {
		return "", fmt.Errorf("Word document has no extractable text")
	}

	// Use LLM to analyze the Word document content
	description, err := das.analyzeContentWithLLM(text, "word", filepath.Base(filePath))
	if err != nil {
		das.logger.Debug("Failed to analyze Word document %s: %v", filePath, err)
		return "", fmt.Errorf("Word document analysis failed: %w", err)
	}

	return description, nil
}

// analyzeExcelFile extracts text from Excel sheets and analyzes them
func (das *DeepAnalysisService) analyzeExcelFile(filePath string) (string, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return "", err
	}

	// Skip very large Excel files
	if info.Size() > maxExcelFileSize {
		return "", fmt.Errorf("Excel file too large (%d bytes)", info.Size())
	}

	// Open Excel file
	f, err := excelize.OpenFile(filePath)
	if err != nil {
		das.logger.Debug("Failed to open Excel file %s: %v", filePath, err)
		return "", fmt.Errorf("failed to open Excel file: %w", err)
	}
	defer f.Close()

	// Get all sheet names
	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return "", fmt.Errorf("Excel file has no sheets")
	}

	// Extract content from all sheets
	var contentBuilder strings.Builder
	contentBuilder.WriteString(fmt.Sprintf("Excel file: %s\nSheets: %d\n\n", filepath.Base(filePath), len(sheets)))

	for _, sheetName := range sheets {
		contentBuilder.WriteString(fmt.Sprintf("Sheet: %s\n", sheetName))

		// Get all rows
		rows, err := f.GetRows(sheetName)
		if err != nil {
			das.logger.Debug("Failed to read sheet %s: %v", sheetName, err)
			continue
		}

		// Limit rows to prevent memory issues
		rowCount := len(rows)
		if rowCount > maxExcelRows {
			rowCount = maxExcelRows
		}

		// Extract cell values
		for i := 0; i < rowCount && i < len(rows); i++ {
			row := rows[i]
			for j, cell := range row {
				if cell != "" {
					contentBuilder.WriteString(fmt.Sprintf("%s ", cell))
				}
				if j > 20 { // Limit columns
					break
				}
			}
			contentBuilder.WriteString("\n")
		}
		contentBuilder.WriteString("\n")
	}

	content := contentBuilder.String()

	// Use LLM to analyze the Excel content
	description, err := das.analyzeContentWithLLM(content, "excel", filepath.Base(filePath))
	if err != nil {
		das.logger.Debug("Failed to analyze Excel file %s: %v", filePath, err)
		return "", fmt.Errorf("Excel analysis failed: %w", err)
	}

	return description, nil
}

// analyzePowerPointFile extracts text from PowerPoint slides and analyzes them
func (das *DeepAnalysisService) analyzePowerPointFile(filePath string) (string, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return "", err
	}

	// Skip very large PowerPoint files
	if info.Size() > maxPowerPointFileSize {
		return "", fmt.Errorf("PowerPoint file too large (%d bytes)", info.Size())
	}

	ext := strings.ToLower(filepath.Ext(filePath))

	// Only .pptx is supported (modern PowerPoint format)
	// .ppt (legacy binary format) requires platform-specific tools
	if ext == ".ppt" {
		das.logger.Debug("Legacy .ppt format not supported, skipping: %s", filePath)
		return "", fmt.Errorf("legacy .ppt format not supported")
	}

	// Open .pptx file as a ZIP archive
	zipReader, err := zip.OpenReader(filePath)
	if err != nil {
		das.logger.Debug("Failed to open PowerPoint file as ZIP %s: %v", filePath, err)
		return "", fmt.Errorf("failed to open PowerPoint file: %w", err)
	}
	defer zipReader.Close()

	// Extract text from all slide XML files
	var allText []string
	slideCount := 0

	for _, file := range zipReader.File {
		// PowerPoint slides are in ppt/slides/slideN.xml
		if strings.HasPrefix(file.Name, "ppt/slides/slide") && strings.HasSuffix(file.Name, ".xml") {
			slideCount++

			rc, err := file.Open()
			if err != nil {
				das.logger.Debug("Failed to open slide file %s: %v", file.Name, err)
				continue
			}

			content, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				das.logger.Debug("Failed to read slide file %s: %v", file.Name, err)
				continue
			}

			// Extract text from this slide's XML
			slideText := das.extractTextFromPPTXML(string(content))
			if slideText != "" {
				allText = append(allText, slideText)
			}
		}
	}

	text := strings.Join(allText, " ")
	das.logger.Debug("Extracted text length for %s: %d characters from %d slides", filePath, len(text), slideCount)
	das.logger.Debug("First 200 chars of extracted text: %s", das.truncateContent(text, 200))

	if text == "" {
		das.logger.Debug("Extracted text is empty for: %s", filePath)
		return "", fmt.Errorf("PowerPoint presentation with %d slides has no extractable text", slideCount)
	}

	// Build content with metadata
	var contentBuilder strings.Builder
	contentBuilder.WriteString(fmt.Sprintf("PowerPoint presentation: %s\nSlides: %d\n\nContent:\n%s",
		filepath.Base(filePath), slideCount, text))

	content := contentBuilder.String()
	das.logger.Debug("Total content length being sent to LLM: %d characters", len(content))

	// Use LLM to analyze the PowerPoint content
	description, err := das.analyzeContentWithLLM(content, "powerpoint", filepath.Base(filePath))
	if err != nil {
		das.logger.Debug("Failed to analyze PowerPoint file %s: %v", filePath, err)
		return "", fmt.Errorf("PowerPoint analysis failed: %w", err)
	}

	return description, nil
}

// analyzePDFFile extracts text from PDF and analyzes it
func (das *DeepAnalysisService) analyzePDFFile(filePath string) (string, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return "", err
	}

	// Skip very large PDFs
	if info.Size() > maxPDFFileSize {
		return "", fmt.Errorf("PDF file too large (%d bytes)", info.Size())
	}

	// Open PDF using go-fitz
	doc, err := fitz.New(filePath)
	if err != nil {
		das.logger.Debug("Failed to open PDF with go-fitz: %v", err)
		return "", fmt.Errorf("failed to open PDF: %w", err)
	}
	defer doc.Close()

	totalPages := doc.NumPage()
	das.logger.Debug("Successfully opened PDF: %s (%d pages)", filePath, totalPages)

	// Extract text from all pages
	var textBuilder strings.Builder
	for pageNum := 0; pageNum < totalPages; pageNum++ {
		text, err := doc.Text(pageNum)
		if err != nil {
			das.logger.Debug("Failed to extract text from page %d: %v", pageNum+1, err)
			continue
		}
		if text != "" {
			textBuilder.WriteString(text)
			textBuilder.WriteString("\n")
		}
	}

	extractedText := strings.TrimSpace(textBuilder.String())

	if extractedText == "" {
		return "", fmt.Errorf("PDF file with %d pages has no extractable text", totalPages)
	}

	das.logger.Debug("Extracted %d characters of text from %d pages", len(extractedText), totalPages)

	// Build content with metadata
	content := fmt.Sprintf("PDF file: %s\nPages: %d\n\nContent:\n%s",
		filepath.Base(filePath), totalPages, extractedText)

	// Use LLM to analyze the PDF text content
	description, err := das.analyzeContentWithLLM(content, "pdf", filepath.Base(filePath))
	if err != nil {
		das.logger.Debug("Failed to analyze PDF file %s: %v", filePath, err)
		return "", fmt.Errorf("PDF analysis failed: %w", err)
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
	// Use appropriate system prompt based on content type
	systemPrompt := das.config.TextAnalysisPrompt
	if contentType == "pdf" {
		systemPrompt = das.config.PDFAnalysisPrompt
	}

	// Use larger truncation limit for structured documents (PowerPoint, Excel, Word, PDF)
	// to give LLM more context
	truncateLimit := 2000
	if contentType == "powerpoint" || contentType == "excel" || contentType == "word" || contentType == "pdf" {
		truncateLimit = 8000
	}

	truncatedContent := das.truncateContent(content, truncateLimit)
	das.logger.Debug("Sending %d characters to LLM for %s analysis (original: %d, limit: %d)",
		len(truncatedContent), contentType, len(content), truncateLimit)

	userPrompt := fmt.Sprintf("File name: %s\nContent type: %s\n\nContent:\n%s\n\nProvide a brief description:", fileName, contentType, truncatedContent)

	reqBody := OpenAIRequest{
		Model: das.config.Model,
		Messages: []Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		MaxTokens: 150,
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
	systemPrompt := das.config.ImageAnalysisPrompt

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
		"max_tokens":  200,
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

// extractTextFromDocxXML extracts plain text from Word document XML content
func (das *DeepAnalysisService) extractTextFromDocxXML(xmlContent string) string {
	// Extract text from <w:t> tags (Word text elements)
	re := regexp.MustCompile(`<w:t[^>]*>([^<]*)</w:t>`)
	matches := re.FindAllStringSubmatch(xmlContent, -1)

	var textParts []string
	for _, match := range matches {
		if len(match) > 1 && match[1] != "" {
			textParts = append(textParts, match[1])
		}
	}

	// Join with spaces and clean up
	text := strings.Join(textParts, " ")

	// Replace multiple spaces with single space
	text = regexp.MustCompile(`\s+`).ReplaceAllString(text, " ")

	return strings.TrimSpace(text)
}

// extractTextFromPPTXML extracts plain text from PowerPoint slide XML content
func (das *DeepAnalysisService) extractTextFromPPTXML(xmlContent string) string {
	// PowerPoint uses <a:t> tags (DrawingML text elements) for text content
	// These are inside <a:r> (text runs) within <a:p> (paragraphs)
	re := regexp.MustCompile(`<a:t[^>]*>([^<]*)</a:t>`)
	matches := re.FindAllStringSubmatch(xmlContent, -1)

	var textParts []string
	for _, match := range matches {
		if len(match) > 1 && match[1] != "" {
			textParts = append(textParts, match[1])
		}
	}

	// Join with spaces and clean up
	text := strings.Join(textParts, " ")

	// Replace multiple spaces with single space
	text = regexp.MustCompile(`\s+`).ReplaceAllString(text, " ")

	return strings.TrimSpace(text)
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
	case ".xls", ".xlsx":
		return "excel"
	case ".doc", ".docx":
		return "document"
	case ".ppt", ".pptx":
		return "powerpoint"
	default:
		return "other"
	}
}
