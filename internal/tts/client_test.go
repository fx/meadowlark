package tts

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSynthesize_Success(t *testing.T) {
	audioData := []byte("fake-audio-data")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/audio/speech", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		w.Header().Set("Content-Type", "audio/wav")
		w.WriteHeader(http.StatusOK)
		w.Write(audioData)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "", nil)
	body, err := client.Synthesize(context.Background(), &SynthesizeRequest{
		Model: "tts-1",
		Voice: "alloy",
		Input: "Hello world",
	})
	require.NoError(t, err)
	defer body.Close()

	data, err := io.ReadAll(body)
	require.NoError(t, err)
	assert.Equal(t, audioData, data)
}

func TestSynthesize_AuthorizationHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer sk-test-key", r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "sk-test-key", nil)
	body, err := client.Synthesize(context.Background(), &SynthesizeRequest{
		Model: "tts-1",
		Voice: "alloy",
		Input: "Hello",
	})
	require.NoError(t, err)
	body.Close()
}

func TestSynthesize_NoAuthHeaderWhenKeyEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Empty(t, r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "", nil)
	body, err := client.Synthesize(context.Background(), &SynthesizeRequest{
		Model: "tts-1",
		Voice: "alloy",
		Input: "Hello",
	})
	require.NoError(t, err)
	body.Close()
}

func TestSynthesize_OmitsNilFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody map[string]any
		err := json.NewDecoder(r.Body).Decode(&reqBody)
		require.NoError(t, err)

		assert.Equal(t, "tts-1", reqBody["model"])
		assert.Equal(t, "alloy", reqBody["voice"])
		assert.Equal(t, "Hello", reqBody["input"])
		assert.NotContains(t, reqBody, "speed")
		assert.NotContains(t, reqBody, "instructions")
		assert.NotContains(t, reqBody, "response_format")

		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "", nil)
	body, err := client.Synthesize(context.Background(), &SynthesizeRequest{
		Model: "tts-1",
		Voice: "alloy",
		Input: "Hello",
	})
	require.NoError(t, err)
	body.Close()
}

func TestSynthesize_IncludesOptionalFields(t *testing.T) {
	speed := 1.5
	instructions := "speak slowly"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody map[string]any
		err := json.NewDecoder(r.Body).Decode(&reqBody)
		require.NoError(t, err)

		assert.Equal(t, 1.5, reqBody["speed"])
		assert.Equal(t, "speak slowly", reqBody["instructions"])
		assert.Equal(t, "wav", reqBody["response_format"])

		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "", nil)
	body, err := client.Synthesize(context.Background(), &SynthesizeRequest{
		Model:          "tts-1",
		Voice:          "alloy",
		Input:          "Hello",
		ResponseFormat: "wav",
		Speed:          &speed,
		Instructions:   &instructions,
	})
	require.NoError(t, err)
	body.Close()
}

func TestSynthesize_4xxError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error": "invalid model"}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "", nil)
	body, err := client.Synthesize(context.Background(), &SynthesizeRequest{
		Model: "invalid",
		Voice: "alloy",
		Input: "Hello",
	})
	assert.Nil(t, body)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "400")
	assert.Contains(t, err.Error(), "invalid model")
}

func TestSynthesize_5xxError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal server error"))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "", nil)
	body, err := client.Synthesize(context.Background(), &SynthesizeRequest{
		Model: "tts-1",
		Voice: "alloy",
		Input: "Hello",
	})
	assert.Nil(t, body)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
	assert.Contains(t, err.Error(), "internal server error")
}

func TestSynthesize_ContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	client := NewClient(srv.URL, "", nil)
	body, err := client.Synthesize(ctx, &SynthesizeRequest{
		Model: "tts-1",
		Voice: "alloy",
		Input: "Hello",
	})
	assert.Nil(t, body)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tts: send request:")
}

func TestNewClient_DefaultHTTPClient(t *testing.T) {
	client := NewClient("http://localhost", "key", nil)
	assert.NotNil(t, client)
	assert.Equal(t, http.DefaultClient, client.httpClient)
}

func TestNewClient_CustomHTTPClient(t *testing.T) {
	custom := &http.Client{}
	client := NewClient("http://localhost", "key", custom)
	assert.Equal(t, custom, client.httpClient)
}
