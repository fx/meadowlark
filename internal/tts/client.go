package tts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// SynthesizeRequest contains the parameters for a TTS synthesis call.
type SynthesizeRequest struct {
	Model          string   `json:"model"`
	Voice          string   `json:"voice"`
	Input          string   `json:"input"`
	ResponseFormat string   `json:"response_format,omitempty"`
	Speed          *float64 `json:"speed,omitempty"`
	Instructions   *string  `json:"instructions,omitempty"`
}

// Client is an OpenAI-compatible TTS HTTP client.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewClient creates a new TTS client. If httpClient is nil, http.DefaultClient is used.
func NewClient(baseURL, apiKey string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{
		baseURL:    baseURL,
		apiKey:     apiKey,
		httpClient: httpClient,
	}
}

// Synthesize sends a TTS request and returns the response body as a stream.
// The caller is responsible for closing the returned ReadCloser.
func (c *Client) Synthesize(ctx context.Context, req *SynthesizeRequest) (io.ReadCloser, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("tts: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/audio/speech", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("tts: create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("tts: send request: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("tts: API error %d: %s", resp.StatusCode, string(errBody))
	}

	return resp.Body, nil
}
