package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type HTTPClient struct {
	client *http.Client
	logger *Logger
}

func NewHTTPClient(logger *Logger) *HTTPClient {
	return &HTTPClient{
		client: &http.Client{},
		logger: logger,
	}
}

// PostStream sends a request and returns the response body for streaming.
// The caller is responsible for closing the body.
func (c *HTTPClient) PostStream(url string, headers map[string]string, body interface{}) (io.ReadCloser, error) {
	jsonData, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream") // Signal we accept streams
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		// If not OK, try to read the error body
		bodyBytes, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("API error: %s - Body: %s", resp.Status, string(bodyBytes))
	}

	return resp.Body, nil
}

// Post sends a POST request and returns the full response body
func (c *HTTPClient) Post(url string, headers map[string]string, body interface{}) ([]byte, error) {
	jsonData, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error: %s - Body: %s", resp.Status, string(bodyBytes))
	}

	return bodyBytes, nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// VerifyMultimodalCapability tests if the LLM endpoint supports multimodal inputs
// by sending a small test request with a base64-encoded 1x1 pixel image
func (c *HTTPClient) VerifyMultimodalCapability(endpoint, apiKey, model string) (bool, error) {
	// Create a minimal 1x1 pixel PNG image (67 bytes base64-encoded)
	// This is a transparent 1x1 PNG pixel
	testImage := "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="

	// Create a minimal test request with multimodal content
	reqBody := map[string]interface{}{
		"model": model,
		"messages": []map[string]interface{}{
			{
				"role": "user",
				"content": []map[string]interface{}{
					{
						"type": "text",
						"text": "Hi",
					},
					{
						"type": "image_url",
						"image_url": map[string]string{
							"url": fmt.Sprintf("data:image/png;base64,%s", testImage),
						},
					},
				},
			},
		},
		"max_tokens": 5,
	}

	headers := map[string]string{
		"Authorization": fmt.Sprintf("Bearer %s", apiKey),
		"HTTP-Referer":  "https://github.com/sandwichdoge/vibesandfolders",
		"X-Title":       "VibesAndFolders",
	}

	// Try to send the multimodal request
	_, err := c.Post(endpoint, headers, reqBody)
	if err != nil {
		// Check if the error indicates lack of multimodal support
		errStr := err.Error()
		if strings.Contains(errStr, "multimodal") ||
			strings.Contains(errStr, "image") ||
			strings.Contains(errStr, "vision") ||
			strings.Contains(errStr, "content type") ||
			strings.Contains(errStr, "not supported") ||
			strings.Contains(errStr, "invalid") {
			return false, nil
		}
		// For other errors, return the error
		return false, err
	}

	// If the request succeeded, the model supports multimodal inputs
	return true, nil
}
