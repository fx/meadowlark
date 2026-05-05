// Package model defines the core data models.
package model

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"
)

// StringSlice is a []string that serializes to/from a JSON array in the database.
type StringSlice []string

// Scan implements sql.Scanner for reading JSON arrays from TEXT columns.
func (s *StringSlice) Scan(src any) error {
	if src == nil {
		*s = nil
		return nil
	}
	var data []byte
	switch v := src.(type) {
	case string:
		data = []byte(v)
	case []byte:
		data = v
	default:
		return fmt.Errorf("model: cannot scan %T into StringSlice", src)
	}
	var result []string
	if err := json.Unmarshal(data, &result); err != nil {
		return fmt.Errorf("model: unmarshal StringSlice: %w", err)
	}
	*s = result
	return nil
}

// Value implements driver.Valuer for writing JSON arrays to TEXT columns.
func (s StringSlice) Value() (driver.Value, error) {
	if s == nil {
		return "[]", nil
	}
	b, err := json.Marshal(s)
	if err != nil {
		return nil, fmt.Errorf("model: marshal StringSlice: %w", err)
	}
	return string(b), nil
}

// Endpoint represents an OpenAI-compatible TTS API endpoint.
type Endpoint struct {
	ID                    string      `json:"id"`
	Name                  string      `json:"name"`
	BaseURL               string      `json:"base_url"`
	APIKey                string      `json:"api_key,omitempty"`
	Models                StringSlice `json:"models"`
	DefaultModel          string      `json:"default_model"`
	DefaultVoice          string      `json:"default_voice"`
	DefaultSpeed          *float64    `json:"default_speed,omitempty"`
	DefaultInstructions   *string     `json:"default_instructions,omitempty"`
	DefaultResponseFormat string      `json:"default_response_format"`
	Enabled               bool        `json:"enabled"`
	StreamingEnabled      bool        `json:"streaming_enabled"`
	StreamSampleRate      int         `json:"stream_sample_rate"`
	CreatedAt             time.Time   `json:"created_at"`
	UpdatedAt             time.Time   `json:"updated_at"`
}

// EffectiveDefaultModel returns the model to use as the default for this
// endpoint: DefaultModel when set, otherwise the first entry in Models, or
// "" when neither is available.
func (e *Endpoint) EffectiveDefaultModel() string {
	if e.DefaultModel != "" {
		return e.DefaultModel
	}
	if len(e.Models) > 0 {
		return e.Models[0]
	}
	return ""
}

// VoiceAlias represents a friendly name mapping to a specific endpoint/model/voice combination.
type VoiceAlias struct {
	ID           string      `json:"id"`
	Name         string      `json:"name"`
	EndpointID   string      `json:"endpoint_id"`
	Model        string      `json:"model"`
	Voice        string      `json:"voice"`
	Speed        *float64    `json:"speed,omitempty"`
	Instructions *string     `json:"instructions,omitempty"`
	Languages    StringSlice `json:"languages"`
	Enabled      bool        `json:"enabled"`
	CreatedAt    time.Time   `json:"created_at"`
	UpdatedAt    time.Time   `json:"updated_at"`
}

// EndpointVoice represents a discovered voice on an endpoint with its enabled state.
// Discovered voices default to enabled=false; the operator opts each in via the UI.
type EndpointVoice struct {
	EndpointID string    `json:"endpoint_id"`
	VoiceID    string    `json:"voice_id"`
	Name       string    `json:"name"`
	Enabled    bool      `json:"enabled"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// ResolvedVoice is the result of voice resolution, combining alias/canonical lookup
// with all parameters needed to make a TTS API call.
type ResolvedVoice struct {
	Name         string      `json:"name"`
	EndpointID   string      `json:"endpoint_id"`
	Model        string      `json:"model"`
	Voice        string      `json:"voice"`
	Speed        *float64    `json:"speed,omitempty"`
	Instructions *string     `json:"instructions,omitempty"`
	Languages    StringSlice `json:"languages"`
	IsAlias      bool        `json:"is_alias"`
}
