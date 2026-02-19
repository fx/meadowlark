package tts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Model represents a model returned by the /v1/models endpoint.
type Model struct {
	ID string `json:"id"`
}

// Voice represents a voice returned by the /v1/audio/voices endpoint.
type Voice struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

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

// ListModels fetches available models from the /v1/models endpoint.
// Returns an empty slice (not an error) on 404 or request failure.
func (c *Client) ListModels(ctx context.Context) ([]Model, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/models", nil)
	if err != nil {
		return []Model{}, nil
	}
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return []Model{}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return []Model{}, nil
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return []Model{}, nil
	}

	// OpenAI-style response: {"data": [{"id": "model-id", ...}]}
	var listResp struct {
		Data []Model `json:"data"`
	}
	if err := json.Unmarshal(body, &listResp); err != nil {
		return []Model{}, nil
	}
	if listResp.Data == nil {
		return []Model{}, nil
	}
	return listResp.Data, nil
}

// ListVoices fetches available voices from the /v1/audio/voices endpoint.
// Handles both OpenAI-style ({"data": [...]}) and Speaches-style ({"voices": [...]}) responses.
// Returns an empty slice (not an error) on 404 or request failure.
func (c *Client) ListVoices(ctx context.Context) ([]Voice, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/audio/voices", nil)
	if err != nil {
		return []Voice{}, nil
	}
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return []Voice{}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return []Voice{}, nil
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return []Voice{}, nil
	}

	// Try OpenAI-style: {"data": [{"id": "voice-id"}]}
	var openAIResp struct {
		Data []Voice `json:"data"`
	}
	if err := json.Unmarshal(body, &openAIResp); err == nil && len(openAIResp.Data) > 0 {
		return openAIResp.Data, nil
	}

	// Try Speaches-style: {"voices": [{"voice_id": "...", "name": "..."}]}
	var speachesResp struct {
		Voices []struct {
			VoiceID string `json:"voice_id"`
			Name    string `json:"name"`
		} `json:"voices"`
	}
	if err := json.Unmarshal(body, &speachesResp); err == nil && len(speachesResp.Voices) > 0 {
		voices := make([]Voice, len(speachesResp.Voices))
		for i, v := range speachesResp.Voices {
			voices[i] = Voice{ID: v.VoiceID, Name: v.Name}
		}
		return voices, nil
	}

	return []Voice{}, nil
}
