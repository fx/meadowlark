package tts

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fx/meadowlark/internal/model"
	"github.com/fx/meadowlark/internal/voice"
	"github.com/fx/meadowlark/internal/wyoming"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildTestWAV wraps buildWAV from wav_test.go, taking width (bytes) instead of bitsPerSample.
func buildTestWAV(rate, width, channels int, pcm []byte) []byte {
	return buildWAV(rate, width*8, channels, pcm)
}

// mockEndpointStore implements EndpointGetter and voice.EndpointLister.
type mockEndpointStore struct {
	endpoints map[string]*model.Endpoint
}

func (s *mockEndpointStore) GetEndpoint(_ context.Context, id string) (*model.Endpoint, error) {
	ep, ok := s.endpoints[id]
	if !ok {
		return nil, context.Canceled
	}
	return ep, nil
}

func (s *mockEndpointStore) ListEndpoints(_ context.Context) ([]model.Endpoint, error) {
	var result []model.Endpoint
	for _, ep := range s.endpoints {
		result = append(result, *ep)
	}
	return result, nil
}

// mockAliasStore implements voice.AliasLister.
type mockAliasStore struct {
	aliases []model.VoiceAlias
}

func (s *mockAliasStore) ListVoiceAliases(_ context.Context) ([]model.VoiceAlias, error) {
	return s.aliases, nil
}

// readAllEvents reads all Wyoming events from a buffer.
func readAllEvents(t *testing.T, data []byte) []*wyoming.Event {
	t.Helper()
	r := bufio.NewReader(bytes.NewReader(data))
	var events []*wyoming.Event
	for {
		ev, err := wyoming.ReadEvent(r)
		if err != nil {
			if err == io.EOF || err.Error() == "read header line: EOF" {
				break
			}
			t.Fatalf("unexpected read error: %v", err)
		}
		events = append(events, ev)
	}
	return events
}

func TestProxy_HandleSynthesize_Success(t *testing.T) {
	// Generate 4096 bytes of PCM data (will produce 2 full chunks).
	pcmData := make([]byte, 4096)
	for i := range pcmData {
		pcmData[i] = byte(i % 256)
	}
	wavData := buildTestWAV(24000, 2, 1, pcmData)

	// Mock TTS server.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/audio/speech", r.URL.Path)
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "audio/wav")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(wavData)
	}))
	defer server.Close()

	ep := &model.Endpoint{
		ID:                    "ep-1",
		Name:                  "TestEndpoint",
		BaseURL:               server.URL,
		APIKey:                "test-key",
		Models:                model.StringSlice{"tts-1"},
		DefaultResponseFormat: "wav",
		Enabled:               true,
	}

	epStore := &mockEndpointStore{endpoints: map[string]*model.Endpoint{"ep-1": ep}}
	aliasStore := &mockAliasStore{}
	resolver := voice.NewResolver(epStore, aliasStore)

	factory := func(e *model.Endpoint) *Client {
		return NewClient(e.BaseURL, e.APIKey, server.Client())
	}

	proxy := NewProxy(resolver, epStore, factory, slog.Default())

	var buf bytes.Buffer
	synth := &wyoming.Synthesize{Text: "Hello world", Voice: "alloy"}
	proxy.HandleSynthesize(context.Background(), synth, &buf)

	events := readAllEvents(t, buf.Bytes())
	require.Len(t, events, 4) // audio-start + 2 audio-chunks + audio-stop

	// Verify audio-start.
	assert.Equal(t, wyoming.TypeAudioStart, events[0].Type)
	audioStart, err := wyoming.AudioStartFromEvent(events[0])
	require.NoError(t, err)
	assert.Equal(t, 24000, audioStart.Rate)
	assert.Equal(t, 2, audioStart.Width)
	assert.Equal(t, 1, audioStart.Channels)

	// Verify audio-chunks.
	assert.Equal(t, wyoming.TypeAudioChunk, events[1].Type)
	chunk1, err := wyoming.AudioChunkFromEvent(events[1])
	require.NoError(t, err)
	assert.Len(t, chunk1.Audio, 2048)
	assert.Equal(t, 24000, chunk1.Rate)

	assert.Equal(t, wyoming.TypeAudioChunk, events[2].Type)
	chunk2, err := wyoming.AudioChunkFromEvent(events[2])
	require.NoError(t, err)
	assert.Len(t, chunk2.Audio, 2048)

	// Verify all PCM data was forwarded correctly.
	allPCM := append(chunk1.Audio, chunk2.Audio...)
	assert.Equal(t, pcmData, allPCM)

	// Verify audio-stop.
	assert.Equal(t, wyoming.TypeAudioStop, events[3].Type)
}

func TestProxy_HandleSynthesize_SmallChunk(t *testing.T) {
	// PCM data smaller than chunkSize.
	pcmData := make([]byte, 100)
	for i := range pcmData {
		pcmData[i] = byte(i)
	}
	wavData := buildTestWAV(16000, 2, 1, pcmData)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(wavData)
	}))
	defer server.Close()

	ep := &model.Endpoint{
		ID:      "ep-1",
		Name:    "Test",
		BaseURL: server.URL,
		APIKey:  "key",
		Models:  model.StringSlice{"model-1"},
		Enabled: true,
	}

	epStore := &mockEndpointStore{endpoints: map[string]*model.Endpoint{"ep-1": ep}}
	aliasStore := &mockAliasStore{}
	resolver := voice.NewResolver(epStore, aliasStore)
	factory := func(e *model.Endpoint) *Client { return NewClient(e.BaseURL, e.APIKey, server.Client()) }

	proxy := NewProxy(resolver, epStore, factory, slog.Default())

	var buf bytes.Buffer
	proxy.HandleSynthesize(context.Background(), &wyoming.Synthesize{Text: "Hi", Voice: "alloy"}, &buf)

	events := readAllEvents(t, buf.Bytes())
	require.Len(t, events, 3) // audio-start + 1 audio-chunk + audio-stop

	assert.Equal(t, wyoming.TypeAudioStart, events[0].Type)
	assert.Equal(t, wyoming.TypeAudioChunk, events[1].Type)
	chunk, _ := wyoming.AudioChunkFromEvent(events[1])
	assert.Equal(t, pcmData, chunk.Audio)
	assert.Equal(t, wyoming.TypeAudioStop, events[2].Type)
}

func TestProxy_HandleSynthesize_APIError(t *testing.T) {
	// TTS server returns an error.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal server error"))
	}))
	defer server.Close()

	ep := &model.Endpoint{
		ID:      "ep-1",
		Name:    "Test",
		BaseURL: server.URL,
		APIKey:  "key",
		Models:  model.StringSlice{"model-1"},
		Enabled: true,
	}

	epStore := &mockEndpointStore{endpoints: map[string]*model.Endpoint{"ep-1": ep}}
	aliasStore := &mockAliasStore{}
	resolver := voice.NewResolver(epStore, aliasStore)
	factory := func(e *model.Endpoint) *Client { return NewClient(e.BaseURL, e.APIKey, server.Client()) }

	proxy := NewProxy(resolver, epStore, factory, slog.Default())

	var buf bytes.Buffer
	proxy.HandleSynthesize(context.Background(), &wyoming.Synthesize{Text: "Hello", Voice: "alloy"}, &buf)

	events := readAllEvents(t, buf.Bytes())
	require.Len(t, events, 1)
	assert.Equal(t, wyoming.TypeError, events[0].Type)
	errEv, _ := wyoming.ErrorFromEvent(events[0])
	assert.Contains(t, errEv.Text, "tts api call")
	assert.Equal(t, "tts-error", errEv.Code)
}

func TestProxy_HandleSynthesize_VoiceAlias(t *testing.T) {
	speed := 1.5
	instructions := "speak cheerfully"
	pcmData := make([]byte, 512)
	wavData := buildTestWAV(24000, 2, 1, pcmData)

	var capturedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(wavData)
	}))
	defer server.Close()

	ep := &model.Endpoint{
		ID:      "ep-1",
		Name:    "TestEndpoint",
		BaseURL: server.URL,
		APIKey:  "key",
		Models:  model.StringSlice{"tts-1"},
		Enabled: true,
	}

	alias := model.VoiceAlias{
		ID:           "alias-1",
		Name:         "friendly",
		EndpointID:   "ep-1",
		Model:        "tts-1",
		Voice:        "nova",
		Speed:        &speed,
		Instructions: &instructions,
		Enabled:      true,
	}

	epStore := &mockEndpointStore{endpoints: map[string]*model.Endpoint{"ep-1": ep}}
	aliasStore := &mockAliasStore{aliases: []model.VoiceAlias{alias}}
	resolver := voice.NewResolver(epStore, aliasStore)
	factory := func(e *model.Endpoint) *Client { return NewClient(e.BaseURL, e.APIKey, server.Client()) }

	proxy := NewProxy(resolver, epStore, factory, slog.Default())

	var buf bytes.Buffer
	proxy.HandleSynthesize(context.Background(), &wyoming.Synthesize{Text: "Greetings", Voice: "friendly"}, &buf)

	events := readAllEvents(t, buf.Bytes())
	require.Len(t, events, 3) // audio-start + chunk + audio-stop
	assert.Equal(t, wyoming.TypeAudioStart, events[0].Type)

	// Verify the request sent to the TTS API used alias parameters.
	assert.Contains(t, string(capturedBody), `"voice":"nova"`)
	assert.Contains(t, string(capturedBody), `"model":"tts-1"`)
	assert.Contains(t, string(capturedBody), `"speed":1.5`)
	assert.Contains(t, string(capturedBody), `"instructions":"speak cheerfully"`)
}

func TestProxy_HandleSynthesize_InputOverrides(t *testing.T) {
	pcmData := make([]byte, 512)
	wavData := buildTestWAV(24000, 2, 1, pcmData)

	var capturedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(wavData)
	}))
	defer server.Close()

	ep := &model.Endpoint{
		ID:      "ep-1",
		Name:    "TestEndpoint",
		BaseURL: server.URL,
		APIKey:  "key",
		Models:  model.StringSlice{"tts-1"},
		Enabled: true,
	}

	epStore := &mockEndpointStore{endpoints: map[string]*model.Endpoint{"ep-1": ep}}
	aliasStore := &mockAliasStore{}
	resolver := voice.NewResolver(epStore, aliasStore)
	factory := func(e *model.Endpoint) *Client { return NewClient(e.BaseURL, e.APIKey, server.Client()) }

	proxy := NewProxy(resolver, epStore, factory, slog.Default())

	var buf bytes.Buffer
	// Use tag format for input overrides.
	proxy.HandleSynthesize(context.Background(), &wyoming.Synthesize{
		Text:  "[speed: 2.0, instructions: whisper softly] Hello world",
		Voice: "alloy",
	}, &buf)

	events := readAllEvents(t, buf.Bytes())
	require.Len(t, events, 3)
	assert.Equal(t, wyoming.TypeAudioStart, events[0].Type)

	// Verify overrides were applied.
	assert.Contains(t, string(capturedBody), `"speed":2`)
	assert.Contains(t, string(capturedBody), `"instructions":"whisper softly"`)
	assert.Contains(t, string(capturedBody), `"input":"Hello world"`)
}

func TestProxy_HandleSynthesize_NoEndpoints(t *testing.T) {
	epStore := &mockEndpointStore{endpoints: map[string]*model.Endpoint{}}
	aliasStore := &mockAliasStore{}
	resolver := voice.NewResolver(epStore, aliasStore)
	factory := func(e *model.Endpoint) *Client { return NewClient(e.BaseURL, e.APIKey, nil) }

	proxy := NewProxy(resolver, epStore, factory, slog.Default())

	var buf bytes.Buffer
	proxy.HandleSynthesize(context.Background(), &wyoming.Synthesize{Text: "Hello", Voice: "alloy"}, &buf)

	events := readAllEvents(t, buf.Bytes())
	require.Len(t, events, 1)
	assert.Equal(t, wyoming.TypeError, events[0].Type)
	errEv, _ := wyoming.ErrorFromEvent(events[0])
	assert.Contains(t, errEv.Text, "resolve voice")
}

func TestProxy_HandleSynthesize_InvalidWAV(t *testing.T) {
	// TTS server returns invalid data (not a WAV file).
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("this is not a WAV file"))
	}))
	defer server.Close()

	ep := &model.Endpoint{
		ID:      "ep-1",
		Name:    "Test",
		BaseURL: server.URL,
		APIKey:  "key",
		Models:  model.StringSlice{"model-1"},
		Enabled: true,
	}

	epStore := &mockEndpointStore{endpoints: map[string]*model.Endpoint{"ep-1": ep}}
	aliasStore := &mockAliasStore{}
	resolver := voice.NewResolver(epStore, aliasStore)
	factory := func(e *model.Endpoint) *Client { return NewClient(e.BaseURL, e.APIKey, server.Client()) }

	proxy := NewProxy(resolver, epStore, factory, slog.Default())

	var buf bytes.Buffer
	proxy.HandleSynthesize(context.Background(), &wyoming.Synthesize{Text: "Hello", Voice: "alloy"}, &buf)

	events := readAllEvents(t, buf.Bytes())
	require.Len(t, events, 1)
	assert.Equal(t, wyoming.TypeError, events[0].Type)
	errEv, _ := wyoming.ErrorFromEvent(events[0])
	assert.Contains(t, errEv.Text, "parse wav header")
}

func TestProxy_HandleSynthesize_JSONInput(t *testing.T) {
	pcmData := make([]byte, 512)
	wavData := buildTestWAV(24000, 2, 1, pcmData)

	var capturedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(wavData)
	}))
	defer server.Close()

	ep := &model.Endpoint{
		ID:      "ep-1",
		Name:    "TestEndpoint",
		BaseURL: server.URL,
		APIKey:  "key",
		Models:  model.StringSlice{"tts-1"},
		Enabled: true,
	}

	epStore := &mockEndpointStore{endpoints: map[string]*model.Endpoint{"ep-1": ep}}
	aliasStore := &mockAliasStore{}
	resolver := voice.NewResolver(epStore, aliasStore)
	factory := func(e *model.Endpoint) *Client { return NewClient(e.BaseURL, e.APIKey, server.Client()) }

	proxy := NewProxy(resolver, epStore, factory, slog.Default())

	var buf bytes.Buffer
	proxy.HandleSynthesize(context.Background(), &wyoming.Synthesize{
		Text:  `{"input":"Hello from JSON","speed":0.8}`,
		Voice: "alloy",
	}, &buf)

	events := readAllEvents(t, buf.Bytes())
	require.Len(t, events, 3)

	assert.Contains(t, string(capturedBody), `"input":"Hello from JSON"`)
	assert.Contains(t, string(capturedBody), `"speed":0.8`)
}

func TestProxy_HandleSynthesize_NilLogger(t *testing.T) {
	// Ensure NewProxy works with nil logger.
	pcmData := make([]byte, 100)
	wavData := buildTestWAV(24000, 2, 1, pcmData)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(wavData)
	}))
	defer server.Close()

	ep := &model.Endpoint{
		ID:      "ep-1",
		Name:    "Test",
		BaseURL: server.URL,
		APIKey:  "key",
		Models:  model.StringSlice{"model-1"},
		Enabled: true,
	}

	epStore := &mockEndpointStore{endpoints: map[string]*model.Endpoint{"ep-1": ep}}
	aliasStore := &mockAliasStore{}
	resolver := voice.NewResolver(epStore, aliasStore)
	factory := func(e *model.Endpoint) *Client { return NewClient(e.BaseURL, e.APIKey, server.Client()) }

	proxy := NewProxy(resolver, epStore, factory, nil)

	var buf bytes.Buffer
	proxy.HandleSynthesize(context.Background(), &wyoming.Synthesize{Text: "Hi", Voice: "alloy"}, &buf)

	events := readAllEvents(t, buf.Bytes())
	require.Len(t, events, 3)
	assert.Equal(t, wyoming.TypeAudioStart, events[0].Type)
}

func TestProxy_HandleSynthesize_ExactChunkBoundary(t *testing.T) {
	// PCM data exactly equals chunkSize - should produce exactly 1 chunk.
	pcmData := make([]byte, chunkSize)
	for i := range pcmData {
		pcmData[i] = byte(i % 256)
	}
	wavData := buildTestWAV(24000, 2, 1, pcmData)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(wavData)
	}))
	defer server.Close()

	ep := &model.Endpoint{
		ID:      "ep-1",
		Name:    "Test",
		BaseURL: server.URL,
		APIKey:  "key",
		Models:  model.StringSlice{"model-1"},
		Enabled: true,
	}

	epStore := &mockEndpointStore{endpoints: map[string]*model.Endpoint{"ep-1": ep}}
	aliasStore := &mockAliasStore{}
	resolver := voice.NewResolver(epStore, aliasStore)
	factory := func(e *model.Endpoint) *Client { return NewClient(e.BaseURL, e.APIKey, server.Client()) }

	proxy := NewProxy(resolver, epStore, factory, slog.Default())

	var buf bytes.Buffer
	proxy.HandleSynthesize(context.Background(), &wyoming.Synthesize{Text: "Hi", Voice: "alloy"}, &buf)

	events := readAllEvents(t, buf.Bytes())
	require.Len(t, events, 3) // audio-start + 1 chunk + audio-stop
	chunk, _ := wyoming.AudioChunkFromEvent(events[1])
	assert.Len(t, chunk.Audio, chunkSize)
}
