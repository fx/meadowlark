// Package store defines the database abstraction layer.
package store

import (
	"context"

	"github.com/fx/meadowlark/internal/model"
)

// Store is the persistence interface for Meadowlark configuration data.
type Store interface {
	ListEndpoints(ctx context.Context) ([]model.Endpoint, error)
	GetEndpoint(ctx context.Context, id string) (*model.Endpoint, error)
	CreateEndpoint(ctx context.Context, e *model.Endpoint) error
	UpdateEndpoint(ctx context.Context, e *model.Endpoint) error
	DeleteEndpoint(ctx context.Context, id string) error

	ListVoiceAliases(ctx context.Context) ([]model.VoiceAlias, error)
	GetVoiceAlias(ctx context.Context, id string) (*model.VoiceAlias, error)
	CreateVoiceAlias(ctx context.Context, a *model.VoiceAlias) error
	UpdateVoiceAlias(ctx context.Context, a *model.VoiceAlias) error
	DeleteVoiceAlias(ctx context.Context, id string) error

	ListEndpointVoices(ctx context.Context, endpointID string) ([]model.EndpointVoice, error)
	UpsertEndpointVoices(ctx context.Context, endpointID string, voices []model.EndpointVoice) error
	SetEndpointVoiceEnabled(ctx context.Context, endpointID, voiceID string, enabled bool) (*model.EndpointVoice, error)

	Migrate(ctx context.Context) error
	Close() error
}

// ErrEndpointVoiceNotFound is returned when an endpoint voice row is not found.
var ErrEndpointVoiceNotFound = endpointVoiceNotFoundError{}

type endpointVoiceNotFoundError struct{}

func (endpointVoiceNotFoundError) Error() string { return "store: endpoint voice not found" }
