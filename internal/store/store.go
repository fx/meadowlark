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

	Migrate(ctx context.Context) error
	Close() error
}
