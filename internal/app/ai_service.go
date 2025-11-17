package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
)

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

// This matches the new system prompt. "json_object" mode requires a root object.
type AIResponseWrapper struct {
	Operations []FileOperation `json:"operations"`
}

// Exported function
func GetAISuggestions(structure, userPrompt, basePath string) ([]FileOperation, error) {

	systemPrompt := `You are a file organization assistant. You MUST output a valid JSON object.

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

	fullPrompt := fmt.Sprintf("Base directory: %s\n\nDirectory structure (relative paths):\n%s\n\nUser instructions: %s\n\nProvide JSON object (using relative paths):", basePath, structure, userPrompt)

	reqBody := OpenAIRequest{
		Model: GlobalConfig.Model,
		Messages: []Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: fullPrompt},
		},
		ResponseFormat: &ResponseFormat{Type: "json_object"}, // Request JSON output
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	// Use exported GlobalConfig
	req, err := http.NewRequest("POST", GlobalConfig.Endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	// Use exported GlobalConfig
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", GlobalConfig.APIKey))

	// OpenRouter-specific headers (optional but recommended)
	req.Header.Set("HTTP-Referer", "https://github.com/sandwichdoge/vibesandfolders")
	req.Header.Set("X-Title", "VibesAndFolders")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Debug logging
	fmt.Printf("=== API Response Debug ===\n")
	fmt.Printf("Status Code: %d\n", resp.StatusCode)
	fmt.Printf("Status: %s\n", resp.Status)
	fmt.Printf("Response Body (first 500 chars):\n%s\n", truncate(string(body), 500))
	fmt.Printf("========================\n")

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error: %s - Response: %s", resp.Status, truncate(string(body), 200))
	}

	var aiResp OpenAIResponse
	if err := json.Unmarshal(body, &aiResp); err != nil {
		return nil, fmt.Errorf("failed to parse API response: %v - Body: %s", err, truncate(string(body), 200))
	}

	if len(aiResp.Choices) == 0 {
		return nil, fmt.Errorf("no response from AI")
	}

	// Parse JSON response
	content := aiResp.Choices[0].Message.Content

	// Debug logging
	fmt.Printf("=== AI Content Debug ===\n")
	fmt.Printf("Raw content (first 500 chars):\n%s\n", truncate(content, 500))
	fmt.Printf("========================\n")

	// NOTE: Because we are using json_object mode, the markdown trimming
	// is likely no longer needed, but it's safe to keep.
	content = strings.TrimSpace(content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	fmt.Printf("=== Cleaned Content Debug ===\n")
	fmt.Printf("Cleaned content (first 500 chars):\n%s\n", truncate(content, 500))
	fmt.Printf("========================\n")

	var responseWrapper AIResponseWrapper
	if err := json.Unmarshal([]byte(content), &responseWrapper); err != nil {
		// This is the error point we were seeing before.
		return nil, fmt.Errorf("failed to parse AI response as JSON object: %v\nResponse: %s", err, truncate(content, 300))
	}

	// --- Now, extract the operations list from the wrapper ---
	operations := responseWrapper.Operations

	// Validate and CONVERT paths from relative to absolute
	for i := range operations {
		// Join the base path with the relative paths from the AI
		operations[i].From = filepath.Join(basePath, operations[i].From)
		operations[i].To = filepath.Join(basePath, operations[i].To)

		// Clean the resulting absolute path (e.g., to handle ".." or ".")
		operations[i].From = filepath.Clean(operations[i].From)
		operations[i].To = filepath.Clean(operations[i].To)
	}

	return operations, nil
}
