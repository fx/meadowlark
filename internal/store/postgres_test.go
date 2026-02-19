package store

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/fx/meadowlark/internal/model"
)

func dockerAvailable() bool {
	if err := exec.Command("docker", "info").Run(); err != nil {
		return false
	}
	return true
}

func newPgTestStore(t *testing.T) *PostgresStore {
	t.Helper()
	if !dockerAvailable() {
		t.Skip("docker not available, skipping PostgreSQL tests")
	}

	ctx := context.Background()
	pgContainer, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("meadowlark_test"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, pgContainer.Terminate(ctx))
	})

	dsn, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	s, err := NewPostgresStore(ctx, dsn)
	require.NoError(t, err)
	require.NoError(t, s.Migrate(ctx))
	t.Cleanup(func() { s.Close() })
	return s
}

func TestPostgresStore_ImplementsStore(t *testing.T) {
	var _ Store = (*PostgresStore)(nil)
}

func TestPostgresMigrate_Idempotent(t *testing.T) {
	s := newPgTestStore(t)
	ctx := context.Background()
	require.NoError(t, s.Migrate(ctx))
	require.NoError(t, s.Migrate(ctx))
}

func TestPostgresCreateEndpoint(t *testing.T) {
	s := newPgTestStore(t)
	ep := makeEndpoint("OpenAI")
	require.NoError(t, s.CreateEndpoint(context.Background(), ep))
	assert.NotEmpty(t, ep.ID)
	assert.False(t, ep.CreatedAt.IsZero())
}

func TestPostgresCreateEndpoint_WithID(t *testing.T) {
	s := newPgTestStore(t)
	ctx := context.Background()
	ep := makeEndpoint("OpenAI")
	ep.ID = "custom-pg-id-123"
	require.NoError(t, s.CreateEndpoint(ctx, ep))
	assert.Equal(t, "custom-pg-id-123", ep.ID)
	got, err := s.GetEndpoint(ctx, "custom-pg-id-123")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "OpenAI", got.Name)
}

func TestPostgresGetEndpoint(t *testing.T) {
	s := newPgTestStore(t)
	ctx := context.Background()
	ep := makeEndpoint("OpenAI")
	require.NoError(t, s.CreateEndpoint(ctx, ep))
	got, err := s.GetEndpoint(ctx, ep.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, ep.ID, got.ID)
	assert.Equal(t, "OpenAI", got.Name)
	assert.Equal(t, model.StringSlice{"tts-1", "gpt-4o-mini-tts"}, got.Models)
	assert.True(t, got.Enabled)
}

func TestPostgresGetEndpoint_NotFound(t *testing.T) {
	s := newPgTestStore(t)
	got, err := s.GetEndpoint(context.Background(), "nonexistent")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestPostgresGetEndpoint_OptionalFields(t *testing.T) {
	s := newPgTestStore(t)
	ctx := context.Background()
	speed := 1.5
	instructions := "speak clearly"
	ep := makeEndpoint("WithDefaults")
	ep.DefaultSpeed = &speed
	ep.DefaultInstructions = &instructions
	require.NoError(t, s.CreateEndpoint(ctx, ep))
	got, err := s.GetEndpoint(ctx, ep.ID)
	require.NoError(t, err)
	require.NotNil(t, got.DefaultSpeed)
	assert.Equal(t, 1.5, *got.DefaultSpeed)
	require.NotNil(t, got.DefaultInstructions)
	assert.Equal(t, "speak clearly", *got.DefaultInstructions)
}

func TestPostgresListEndpoints(t *testing.T) {
	s := newPgTestStore(t)
	ctx := context.Background()
	list, err := s.ListEndpoints(ctx)
	require.NoError(t, err)
	assert.Empty(t, list)
	require.NoError(t, s.CreateEndpoint(ctx, makeEndpoint("Beta")))
	require.NoError(t, s.CreateEndpoint(ctx, makeEndpoint("Alpha")))
	list, err = s.ListEndpoints(ctx)
	require.NoError(t, err)
	assert.Len(t, list, 2)
	assert.Equal(t, "Alpha", list[0].Name)
	assert.Equal(t, "Beta", list[1].Name)
}

func TestPostgresUpdateEndpoint(t *testing.T) {
	s := newPgTestStore(t)
	ctx := context.Background()
	ep := makeEndpoint("Original")
	require.NoError(t, s.CreateEndpoint(ctx, ep))
	ep.Name = "Updated"
	ep.Enabled = false
	speed := 2.0
	ep.DefaultSpeed = &speed
	require.NoError(t, s.UpdateEndpoint(ctx, ep))
	got, err := s.GetEndpoint(ctx, ep.ID)
	require.NoError(t, err)
	assert.Equal(t, "Updated", got.Name)
	assert.False(t, got.Enabled)
	require.NotNil(t, got.DefaultSpeed)
	assert.Equal(t, 2.0, *got.DefaultSpeed)
}

func TestPostgresUpdateEndpoint_NotFound(t *testing.T) {
	s := newPgTestStore(t)
	ep := makeEndpoint("Ghost")
	ep.ID = "does-not-exist"
	err := s.UpdateEndpoint(context.Background(), ep)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestPostgresDeleteEndpoint(t *testing.T) {
	s := newPgTestStore(t)
	ctx := context.Background()
	ep := makeEndpoint("ToDelete")
	require.NoError(t, s.CreateEndpoint(ctx, ep))
	require.NoError(t, s.DeleteEndpoint(ctx, ep.ID))
	got, err := s.GetEndpoint(ctx, ep.ID)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestPostgresDeleteEndpoint_NotFound(t *testing.T) {
	s := newPgTestStore(t)
	err := s.DeleteEndpoint(context.Background(), "nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestPostgresCreateVoiceAlias(t *testing.T) {
	s := newPgTestStore(t)
	ctx := context.Background()
	ep := makeEndpoint("OpenAI")
	require.NoError(t, s.CreateEndpoint(ctx, ep))
	alias := &model.VoiceAlias{
		Name: "my-voice", EndpointID: ep.ID, Model: "tts-1", Voice: "alloy",
		Languages: model.StringSlice{"en", "es"}, Enabled: true,
	}
	require.NoError(t, s.CreateVoiceAlias(ctx, alias))
	assert.NotEmpty(t, alias.ID)
	assert.False(t, alias.CreatedAt.IsZero())
}

func TestPostgresCreateVoiceAlias_DefaultLanguages(t *testing.T) {
	s := newPgTestStore(t)
	ctx := context.Background()
	ep := makeEndpoint("OpenAI")
	require.NoError(t, s.CreateEndpoint(ctx, ep))
	alias := &model.VoiceAlias{Name: "defaults", EndpointID: ep.ID, Model: "tts-1", Voice: "alloy", Enabled: true}
	require.NoError(t, s.CreateVoiceAlias(ctx, alias))
	assert.Equal(t, model.StringSlice{"en"}, alias.Languages)
}

func TestPostgresGetVoiceAlias(t *testing.T) {
	s := newPgTestStore(t)
	ctx := context.Background()
	ep := makeEndpoint("OpenAI")
	require.NoError(t, s.CreateEndpoint(ctx, ep))
	speed := 0.8
	instructions := "whisper"
	alias := &model.VoiceAlias{
		Name: "whisper-voice", EndpointID: ep.ID, Model: "tts-1", Voice: "nova",
		Speed: &speed, Instructions: &instructions, Languages: model.StringSlice{"en"}, Enabled: true,
	}
	require.NoError(t, s.CreateVoiceAlias(ctx, alias))
	got, err := s.GetVoiceAlias(ctx, alias.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "whisper-voice", got.Name)
	assert.Equal(t, 0.8, *got.Speed)
	assert.Equal(t, "whisper", *got.Instructions)
}

func TestPostgresGetVoiceAlias_NotFound(t *testing.T) {
	s := newPgTestStore(t)
	got, err := s.GetVoiceAlias(context.Background(), "nonexistent")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestPostgresListVoiceAliases(t *testing.T) {
	s := newPgTestStore(t)
	ctx := context.Background()
	ep := makeEndpoint("OpenAI")
	require.NoError(t, s.CreateEndpoint(ctx, ep))
	list, err := s.ListVoiceAliases(ctx)
	require.NoError(t, err)
	assert.Empty(t, list)
	require.NoError(t, s.CreateVoiceAlias(ctx, &model.VoiceAlias{Name: "beta-voice", EndpointID: ep.ID, Model: "tts-1", Voice: "alloy", Enabled: true}))
	require.NoError(t, s.CreateVoiceAlias(ctx, &model.VoiceAlias{Name: "alpha-voice", EndpointID: ep.ID, Model: "tts-1", Voice: "nova", Enabled: true}))
	list, err = s.ListVoiceAliases(ctx)
	require.NoError(t, err)
	assert.Len(t, list, 2)
	assert.Equal(t, "alpha-voice", list[0].Name)
	assert.Equal(t, "beta-voice", list[1].Name)
}

func TestPostgresUpdateVoiceAlias(t *testing.T) {
	s := newPgTestStore(t)
	ctx := context.Background()
	ep := makeEndpoint("OpenAI")
	require.NoError(t, s.CreateEndpoint(ctx, ep))
	alias := &model.VoiceAlias{Name: "original", EndpointID: ep.ID, Model: "tts-1", Voice: "alloy", Languages: model.StringSlice{"en"}, Enabled: true}
	require.NoError(t, s.CreateVoiceAlias(ctx, alias))
	alias.Name = "updated"
	alias.Voice = "nova"
	speed := 1.5
	alias.Speed = &speed
	require.NoError(t, s.UpdateVoiceAlias(ctx, alias))
	got, err := s.GetVoiceAlias(ctx, alias.ID)
	require.NoError(t, err)
	assert.Equal(t, "updated", got.Name)
	assert.Equal(t, "nova", got.Voice)
	assert.Equal(t, 1.5, *got.Speed)
}

func TestPostgresUpdateVoiceAlias_NotFound(t *testing.T) {
	s := newPgTestStore(t)
	alias := &model.VoiceAlias{ID: "does-not-exist", Name: "x", EndpointID: "x", Model: "x", Voice: "x"}
	err := s.UpdateVoiceAlias(context.Background(), alias)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestPostgresDeleteVoiceAlias(t *testing.T) {
	s := newPgTestStore(t)
	ctx := context.Background()
	ep := makeEndpoint("OpenAI")
	require.NoError(t, s.CreateEndpoint(ctx, ep))
	alias := &model.VoiceAlias{Name: "todelete", EndpointID: ep.ID, Model: "tts-1", Voice: "alloy", Enabled: true}
	require.NoError(t, s.CreateVoiceAlias(ctx, alias))
	require.NoError(t, s.DeleteVoiceAlias(ctx, alias.ID))
	got, err := s.GetVoiceAlias(ctx, alias.ID)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestPostgresDeleteVoiceAlias_NotFound(t *testing.T) {
	s := newPgTestStore(t)
	err := s.DeleteVoiceAlias(context.Background(), "nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestPostgresCreateEndpoint_DuplicateName(t *testing.T) {
	s := newPgTestStore(t)
	ctx := context.Background()
	require.NoError(t, s.CreateEndpoint(ctx, makeEndpoint("Unique")))
	err := s.CreateEndpoint(ctx, makeEndpoint("Unique"))
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "unique") || strings.Contains(err.Error(), "duplicate"), "got: %v", err)
}

func TestPostgresCreateVoiceAlias_DuplicateName(t *testing.T) {
	s := newPgTestStore(t)
	ctx := context.Background()
	ep := makeEndpoint("OpenAI")
	require.NoError(t, s.CreateEndpoint(ctx, ep))
	require.NoError(t, s.CreateVoiceAlias(ctx, &model.VoiceAlias{Name: "same-name", EndpointID: ep.ID, Model: "tts-1", Voice: "alloy", Enabled: true}))
	err := s.CreateVoiceAlias(ctx, &model.VoiceAlias{Name: "same-name", EndpointID: ep.ID, Model: "tts-1", Voice: "nova", Enabled: true})
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "unique") || strings.Contains(err.Error(), "duplicate"), "got: %v", err)
}

func TestPostgresCreateVoiceAlias_InvalidEndpointFK(t *testing.T) {
	s := newPgTestStore(t)
	alias := &model.VoiceAlias{Name: "orphan", EndpointID: "nonexistent-endpoint-id", Model: "tts-1", Voice: "alloy", Enabled: true}
	err := s.CreateVoiceAlias(context.Background(), alias)
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "foreign key") || strings.Contains(err.Error(), "violates"), "got: %v", err)
}

func TestPostgresDeleteEndpoint_WithAliases(t *testing.T) {
	s := newPgTestStore(t)
	ctx := context.Background()
	ep := makeEndpoint("HasAliases")
	require.NoError(t, s.CreateEndpoint(ctx, ep))
	require.NoError(t, s.CreateVoiceAlias(ctx, &model.VoiceAlias{Name: "child", EndpointID: ep.ID, Model: "tts-1", Voice: "alloy", Enabled: true}))
	err := s.DeleteEndpoint(ctx, ep.ID)
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "foreign key") || strings.Contains(err.Error(), "violates"), "got: %v", err)
}

func TestPostgresEndpoint_ModelsRoundTrip(t *testing.T) {
	s := newPgTestStore(t)
	ctx := context.Background()
	ep := makeEndpoint("JSONTest")
	ep.Models = model.StringSlice{"tts-1", "gpt-4o-mini-tts", "kokoro-v1"}
	require.NoError(t, s.CreateEndpoint(ctx, ep))
	got, err := s.GetEndpoint(ctx, ep.ID)
	require.NoError(t, err)
	assert.Equal(t, model.StringSlice{"tts-1", "gpt-4o-mini-tts", "kokoro-v1"}, got.Models)
}

func TestPostgresVoiceAlias_LanguagesRoundTrip(t *testing.T) {
	s := newPgTestStore(t)
	ctx := context.Background()
	ep := makeEndpoint("OpenAI")
	require.NoError(t, s.CreateEndpoint(ctx, ep))
	alias := &model.VoiceAlias{Name: "multilang", EndpointID: ep.ID, Model: "tts-1", Voice: "alloy", Languages: model.StringSlice{"en", "es", "fr", "de"}, Enabled: true}
	require.NoError(t, s.CreateVoiceAlias(ctx, alias))
	got, err := s.GetVoiceAlias(ctx, alias.ID)
	require.NoError(t, err)
	assert.Equal(t, model.StringSlice{"en", "es", "fr", "de"}, got.Languages)
}
