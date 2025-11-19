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
	MaxTokens      int             `json:"max_tokens,omitempty"`
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
		MaxTokens:      8192,
	}

	headers := map[string]string{
		"Authorization": fmt.Sprintf("Bearer %s", s.config.APIKey),
		"HTTP-Referer":  "https://github.com/sandwichdoge/vibesandfolders",
		"X-Title":       "VibesAndFolders",
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
	return `You are a file organization assistant. Output only valid JSON.

Format:
{
  "operations": [
    {"from": "path/to/file.txt", "to": "new/path/file.txt"}
  ]
}

Rules:
- Use paths relative to the base directory
- "from" must reference existing files from the structure
- "to" must include the full destination path with filename
- Skip files that do not need to be changed, only output files that need changing
- Rename files when asked for
- Return empty array if no changes needed: {"operations": []}
- No explanations or markdown, only JSON`
}

func (s *OpenAIAIService) buildUserPrompt(basePath, structure, userPrompt string) string {
	return fmt.Sprintf("Base directory: %s\n\nDirectory structure:\n%s\n\nUser instructions: %s", basePath, structure, userPrompt)
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
