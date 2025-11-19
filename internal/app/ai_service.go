package app

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

const (
	defaultMaxTokens = 8192
)

type OpenAIService struct {
	config     *Config
	httpClient *HTTPClient
	logger     *Logger
}

func NewOpenAIService(config *Config, httpClient *HTTPClient, logger *Logger) *OpenAIService {
	return &OpenAIService{
		config:     config,
		httpClient: httpClient,
		logger:     logger,
	}
}

type OpenAIRequest struct {
	Model     string    `json:"model"`
	Messages  []Message `json:"messages"`
	MaxTokens int       `json:"max_tokens,omitempty"`
	Stream    bool      `json:"stream"` // Enable streaming
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// OpenAIStreamResponse matches the SSE data structure
type OpenAIStreamResponse struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

func (s *OpenAIService) GetSuggestions(structure, userPrompt, basePath string, onOperation OperationCallback) ([]FileOperation, error) {
	systemPrompt := s.buildSystemPrompt()
	fullPrompt := s.buildUserPrompt(basePath, structure, userPrompt)

	reqBody := OpenAIRequest{
		Model: s.config.Model,
		Messages: []Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: fullPrompt},
		},
		MaxTokens: defaultMaxTokens,
		Stream:    true,
	}

	headers := map[string]string{
		"Authorization": fmt.Sprintf("Bearer %s", s.config.APIKey),
		"HTTP-Referer":  "https://github.com/sandwichdoge/vibesandfolders",
		"X-Title":       "VibesAndFolders",
	}

	streamBody, err := s.httpClient.PostStream(s.config.Endpoint, headers, reqBody)
	if err != nil {
		return nil, err
	}
	defer streamBody.Close()

	return s.processStream(streamBody, basePath, onOperation)
}

// processStream reads the SSE stream, accumulates tokens, and parses JSON lines
func (s *OpenAIService) processStream(r io.Reader, basePath string, onOperation OperationCallback) ([]FileOperation, error) {
	scanner := bufio.NewScanner(r)
	var operations []FileOperation
	var buffer bytes.Buffer // Accumulates content fragments

	// To handle cases where the AI might split a JSON line across multiple tokens
	// we accumulate text in 'buffer' and only parse when we see a newline.

	for scanner.Scan() {
		line := scanner.Text()

		// SSE format usually starts with "data: "
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")
		data = strings.TrimSpace(data)

		if data == "[DONE]" {
			break
		}

		var streamResp OpenAIStreamResponse
		if err := json.Unmarshal([]byte(data), &streamResp); err != nil {
			// Only log debug, don't fail whole stream for one bad chunk
			s.logger.Debug("Failed to unmarshal stream chunk: %v", err)
			continue
		}

		if len(streamResp.Choices) > 0 {
			content := streamResp.Choices[0].Delta.Content
			if content != "" {
				buffer.WriteString(content)

				// Check if we have a complete line (indicated by newline in the content)
				// Note: We loop because one chunk might contain multiple newlines (multiple ops)
				// or the newline might just finish the current op.
				currentStr := buffer.String()
				if strings.Contains(currentStr, "\n") {
					parts := strings.Split(currentStr, "\n")

					// Process all complete parts
					// The last part is either empty (if ended with \n) or incomplete (wait for next chunk)
					for i := 0; i < len(parts)-1; i++ {
						rawLine := strings.TrimSpace(parts[i])
						if rawLine != "" {
							if op, err := s.parseSingleOperation(rawLine, basePath); err == nil {
								operations = append(operations, op)
								if onOperation != nil {
									onOperation(op) // Trigger UI update
								}
							} else if err.Error() == "source and destination are identical" {
								// Silently ignore, do not log as error, do not send to UI
								continue
							} else {
								s.logger.Debug("Failed to parse JSON line: %s | Error: %v", rawLine, err)
							}
						}
					}

					// Keep the last part in the buffer
					buffer.Reset()
					buffer.WriteString(parts[len(parts)-1])
				}
			}
		}
	}

	// Process any remaining data in buffer (if AI forgot final newline)
	remaining := strings.TrimSpace(buffer.String())
	if remaining != "" {
		if op, err := s.parseSingleOperation(remaining, basePath); err == nil {
			operations = append(operations, op)
			if onOperation != nil {
				onOperation(op)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return operations, fmt.Errorf("stream reading error: %w", err)
	}

	return operations, nil
}

func (s *OpenAIService) parseSingleOperation(jsonLine, basePath string) (FileOperation, error) {
	// Clean up potential markdown artifacts if the AI ignored instructions
	jsonLine = strings.TrimPrefix(jsonLine, "```json")
	jsonLine = strings.TrimPrefix(jsonLine, "```")
	jsonLine = strings.TrimSuffix(jsonLine, "```")
	jsonLine = strings.TrimSpace(jsonLine)

	// Handle comma at end if AI treated it like a list
	jsonLine = strings.TrimSuffix(jsonLine, ",")

	var op FileOperation
	if err := json.Unmarshal([]byte(jsonLine), &op); err != nil {
		return op, err
	}

	// Sanitize paths
	op.From = filepath.Clean(filepath.Join(basePath, op.From))
	op.To = filepath.Clean(filepath.Join(basePath, op.To))

	if op.From == op.To {
		return op, fmt.Errorf("source and destination are identical")
	}

	return op, nil
}

func (s *OpenAIService) buildSystemPrompt() string {
	return `You are a file organization assistant.
You must output a stream of valid JSON objects.

Output Format Rules:
1. Output format: JSON Lines. Each line must be a standalone valid JSON object: {"from": "...", "to": "..."}
2. "from": path relative to base, must exist.
3. "to": destination path relative to base.
4. Only output files that need moving/renaming.

Example:
{"from": "IMG_1234.jpg", "to": "photos/vacation/IMG_1234.jpg"}
{"from": "document.pdf", "to": "documents/renamed_document.pdf"}
{"from": "old_folder/file.txt", "to": "new_folder/file.txt"}

Organization Principles:
5. When creating folders, use consistent naming that matches existing patterns in the directory.
6. Preserve existing well-organized structures. Avoid reorganizing what's already logically arranged.
7. May rename files in required.
`
}

func (s *OpenAIService) buildUserPrompt(basePath, structure, userPrompt string) string {
	return fmt.Sprintf("Base directory: %s\n\nDirectory structure:\n%s\n\nUser instructions: %s", basePath, structure, userPrompt)
}
