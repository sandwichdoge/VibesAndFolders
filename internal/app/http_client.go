package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

// PostJSON remains for non-streaming calls if needed (though we might retire it later)
func (c *HTTPClient) PostJSON(url string, headers map[string]string, body interface{}) ([]byte, error) {
	// ... existing implementation ...
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

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	c.logger.Debug("HTTP Response - Status: %d, Body: %s", resp.StatusCode, truncate(string(responseBody), 500))

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error: %s - Response: %s", resp.Status, truncate(string(responseBody), 200))
	}

	return responseBody, nil
}
