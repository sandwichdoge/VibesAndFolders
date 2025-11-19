package app

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

type OpenAIAIService struct {
	config     *Config
	httpClient *HTTPClient
	logger     *Logger
}

func NewOpenAIAIService(config *Config, httpClient *HTTPClient, logger *Logger) *OpenAIAIService {
	return &OpenAIAIService{
		config:     config,
		httpClient: httpClient,
		logger:     logger,
	}
}

type ResponseFormat struct {
	Type string `json:"type"`
}

type OpenAIRequest struct {
	Model          string          `json:"model"`
	Messages       []Message       `json:"messages"`
	ResponseFormat *ResponseFormat `json:"response_format,omitempty"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type OpenAIResponse struct {
	Choices []struct {
		Message Message `json:"message"`
	} `json:"choices"`
}

type AIResponseWrapper struct {
	Operations []FileOperation `json:"operations"`
}

func (s *OpenAIAIService) GetSuggestions(structure, userPrompt, basePath string) ([]FileOperation, error) {
	systemPrompt := s.buildSystemPrompt()
	fullPrompt := s.buildUserPrompt(basePath, structure, userPrompt)

	s.logger.Debug("System Prompt: %s", systemPrompt)
	s.logger.Debug("User Prompt: %s", fullPrompt)

	reqBody := OpenAIRequest{
		Model: s.config.Model,
		Messages: []Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: fullPrompt},
		},
		ResponseFormat: &ResponseFormat{Type: "json_object"},
	}

	headers := map[string]string{
		"Authorization":  fmt.Sprintf("Bearer %s", s.config.APIKey),
		"HTTP-Referer":   "https://github.com/sandwichdoge/vibesandfolders",
		"X-Title":        "VibesAndFolders",
	}

	responseBody, err := s.httpClient.PostJSON(s.config.Endpoint, headers, reqBody)
	if err != nil {
		return nil, err
	}

	var aiResp OpenAIResponse
	if err := json.Unmarshal(responseBody, &aiResp); err != nil {
		return nil, fmt.Errorf("failed to parse API response: %w - Body: %s", err, truncate(string(responseBody), 200))
	}

	if len(aiResp.Choices) == 0 {
		return nil, fmt.Errorf("no response from AI")
	}

	content := s.cleanContent(aiResp.Choices[0].Message.Content)
	s.logger.Debug("Cleaned AI Content: %s", truncate(content, 500))

	operations, err := s.parseOperations(content, basePath)
	if err != nil {
		return nil, err
	}

	return operations, nil
}

func (s *OpenAIAIService) buildSystemPrompt() string {
	return `You are a file organization assistant. You MUST output a valid JSON object.

This JSON object must contain a single key: "operations".
The value of "operations" MUST be a JSON array of file operation objects.

CRITICAL RULES:
1. Each object in the "operations" array must have "from" and "to" fields with paths RELATIVE to the base directory.
2. Ensure "from" paths point to existing files/folders from the structure.
3. Ensure "to" paths include the full RELATIVE destination path with filename.
4. If no operations are needed, you must return an empty array: {"operations": []}
5. Do NOT include any explanations, markdown, or other text outside the JSON object.

Example output format:
{
  "operations": [
    {"from": "file1.txt", "to": "Documents/file1.txt"},
    {"from": "old_images/file2.jpg", "to": "Images/file2.jpg"}
  ]
}`
}

func (s *OpenAIAIService) buildUserPrompt(basePath, structure, userPrompt string) string {
	return fmt.Sprintf("Base directory: %s\n\nDirectory structure (relative paths):\n%s\n\nUser instructions: %s\n\nProvide JSON object (using relative paths):", basePath, structure, userPrompt)
}

func (s *OpenAIAIService) cleanContent(content string) string {
	content = strings.TrimSpace(content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	return strings.TrimSpace(content)
}

func (s *OpenAIAIService) parseOperations(content string, basePath string) ([]FileOperation, error) {
	var responseWrapper AIResponseWrapper
	if err := json.Unmarshal([]byte(content), &responseWrapper); err != nil {
		return nil, fmt.Errorf("failed to parse AI response as JSON object: %w\nResponse: %s", err, truncate(content, 300))
	}

	operations := responseWrapper.Operations

	for i := range operations {
		operations[i].From = filepath.Clean(filepath.Join(basePath, operations[i].From))
		operations[i].To = filepath.Clean(filepath.Join(basePath, operations[i].To))
	}

	return operations, nil
}

// Backward compatibility function
func GetAISuggestions(structure, userPrompt, basePath string) ([]FileOperation, error) {
	service := NewOpenAIAIService(GlobalConfig, NewHTTPClient(DefaultLogger), DefaultLogger)
	return service.GetSuggestions(structure, userPrompt, basePath)
}
