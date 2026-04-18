package tts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
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
		truncated := truncateBody(string(errBody), 500)
		slog.Error("tts: non-2xx response from TTS endpoint",
			"status_code", resp.StatusCode,
			"content_type", resp.Header.Get("Content-Type"),
			"response_body", truncated,
		)
		return nil, fmt.Errorf("tts: API error %d: %s", resp.StatusCode, truncated)
	}

	// Check for WAV magic bytes ("RIFF") to detect non-WAV error responses.
	// Read the first 4 bytes and then prepend them back to the stream.
	var header [4]byte
	n, err := io.ReadFull(resp.Body, header[:])
	if err != nil || n < 4 {
		resp.Body.Close()
		return nil, fmt.Errorf("tts: failed to read response header: %w", err)
	}
	if string(header[:]) != "RIFF" {
		// Not a WAV file -- likely a JSON error response. Read the rest for logging.
		rest, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		full := string(header[:n]) + string(rest)
		truncated := truncateBody(full, 500)
		slog.Error("tts: TTS endpoint returned non-WAV response",
			"status_code", resp.StatusCode,
			"content_type", resp.Header.Get("Content-Type"),
			"response_body", truncated,
		)
		return nil, fmt.Errorf("tts: endpoint returned non-WAV response: %s", truncated)
	}

	// Reconstruct the full stream with the header bytes prepended.
	combined := io.MultiReader(bytes.NewReader(header[:]), resp.Body)
	return &readCloser{Reader: combined, Closer: resp.Body}, nil
}

// readCloser wraps a Reader and a Closer into an io.ReadCloser.
type readCloser struct {
	io.Reader
	io.Closer
}

// truncateBody truncates a string to maxLen characters, appending "..." if truncated.
func truncateBody(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
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

	// Try generic voices array: {"voices": [{"id": "...", "name": "..."}]}
	var voicesResp struct {
		Voices []Voice `json:"voices"`
	}
	if err := json.Unmarshal(body, &voicesResp); err == nil && len(voicesResp.Voices) > 0 {
		hasID := false
		for _, v := range voicesResp.Voices {
			if v.ID != "" {
				hasID = true
				break
			}
		}
		if hasID {
			return voicesResp.Voices, nil
		}
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

	// Try plain string array: {"voices": ["voice1", "voice2"]}
	var stringResp struct {
		Voices []string `json:"voices"`
	}
	if err := json.Unmarshal(body, &stringResp); err == nil && len(stringResp.Voices) > 0 {
		voices := make([]Voice, len(stringResp.Voices))
		for i, v := range stringResp.Voices {
			voices[i] = Voice{ID: v, Name: v}
		}
		return voices, nil
	}

	return []Voice{}, nil
}
