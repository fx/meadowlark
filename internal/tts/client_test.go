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
	audioData := []byte("RIFF\x00\x00\x00\x00WAVEfmt fake-audio-data")
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
		w.Write([]byte("RIFF\x00\x00\x00\x00WAVEfmt "))
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
		w.Write([]byte("RIFF\x00\x00\x00\x00WAVEfmt "))
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
		w.Write([]byte("RIFF\x00\x00\x00\x00WAVEfmt "))
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
		w.Write([]byte("RIFF\x00\x00\x00\x00WAVEfmt "))
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

func TestListModels_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/models", r.URL.Path)
		assert.Equal(t, "Bearer sk-test", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"id":"tts-1"},{"id":"tts-1-hd"}]}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "sk-test", nil)
	models, err := client.ListModels(context.Background())
	require.NoError(t, err)
	require.Len(t, models, 2)
	assert.Equal(t, "tts-1", models[0].ID)
	assert.Equal(t, "tts-1-hd", models[1].ID)
}

func TestListModels_404ReturnsEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "", nil)
	models, err := client.ListModels(context.Background())
	require.NoError(t, err)
	assert.Empty(t, models)
}

func TestListModels_NetworkErrorReturnsEmpty(t *testing.T) {
	client := NewClient("http://127.0.0.1:0", "", nil)
	models, err := client.ListModels(context.Background())
	require.NoError(t, err)
	assert.Empty(t, models)
}

func TestListModels_InvalidJSONReturnsEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`not json`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "", nil)
	models, err := client.ListModels(context.Background())
	require.NoError(t, err)
	assert.Empty(t, models)
}

func TestListModels_EmptyDataReturnsEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"data":null}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "", nil)
	models, err := client.ListModels(context.Background())
	require.NoError(t, err)
	assert.Empty(t, models)
}

func TestListVoices_OpenAIStyle(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/audio/voices", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"id":"alloy","name":"Alloy"},{"id":"nova","name":"Nova"}]}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "", nil)
	voices, err := client.ListVoices(context.Background())
	require.NoError(t, err)
	require.Len(t, voices, 2)
	assert.Equal(t, "alloy", voices[0].ID)
	assert.Equal(t, "Alloy", voices[0].Name)
	assert.Equal(t, "nova", voices[1].ID)
}

func TestListVoices_SpeachesStyle(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"voices":[{"voice_id":"jenny","name":"Jenny"},{"voice_id":"adam","name":"Adam"}]}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "", nil)
	voices, err := client.ListVoices(context.Background())
	require.NoError(t, err)
	require.Len(t, voices, 2)
	assert.Equal(t, "jenny", voices[0].ID)
	assert.Equal(t, "Jenny", voices[0].Name)
	assert.Equal(t, "adam", voices[1].ID)
}

func TestListVoices_GenericVoicesArray(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"voices":[{"id":"Vivian","name":"Vivian"},{"id":"Ryan","name":"Ryan"}]}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "", nil)
	voices, err := client.ListVoices(context.Background())
	require.NoError(t, err)
	require.Len(t, voices, 2)
	assert.Equal(t, "Vivian", voices[0].ID)
	assert.Equal(t, "Vivian", voices[0].Name)
	assert.Equal(t, "Ryan", voices[1].ID)
	assert.Equal(t, "Ryan", voices[1].Name)
}

func TestListVoices_StringArrayStyle(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"voices":["aiden","dylan","eric","vivian"]}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "", nil)
	voices, err := client.ListVoices(context.Background())
	require.NoError(t, err)
	require.Len(t, voices, 4)
	assert.Equal(t, "aiden", voices[0].ID)
	assert.Equal(t, "aiden", voices[0].Name)
	assert.Equal(t, "vivian", voices[3].ID)
	assert.Equal(t, "vivian", voices[3].Name)
}

func TestListVoices_404ReturnsEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "", nil)
	voices, err := client.ListVoices(context.Background())
	require.NoError(t, err)
	assert.Empty(t, voices)
}

func TestListVoices_NetworkErrorReturnsEmpty(t *testing.T) {
	client := NewClient("http://127.0.0.1:0", "", nil)
	voices, err := client.ListVoices(context.Background())
	require.NoError(t, err)
	assert.Empty(t, voices)
}

func TestListVoices_InvalidJSONReturnsEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`not json`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "", nil)
	voices, err := client.ListVoices(context.Background())
	require.NoError(t, err)
	assert.Empty(t, voices)
}

func TestListVoices_NoAuthHeaderWhenKeyEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Empty(t, r.Header.Get("Authorization"))
		w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "", nil)
	voices, err := client.ListVoices(context.Background())
	require.NoError(t, err)
	assert.Empty(t, voices)
}

func TestListVoices_AuthHeaderWhenKeySet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer sk-key", r.Header.Get("Authorization"))
		w.Write([]byte(`{"data":[{"id":"alloy","name":"Alloy"}]}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "sk-key", nil)
	voices, err := client.ListVoices(context.Background())
	require.NoError(t, err)
	require.Len(t, voices, 1)
}

func TestSynthesize_NonWAVResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"error":"model not found"}`))
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
	assert.Contains(t, err.Error(), "non-WAV response")
	assert.Contains(t, err.Error(), "model not found")
}

func TestSynthesize_NonWAVResponseTruncated(t *testing.T) {
	// Response body longer than 500 chars should be truncated.
	longBody := make([]byte, 600)
	for i := range longBody {
		longBody[i] = 'a'
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(longBody)
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
	assert.Contains(t, err.Error(), "non-WAV response")
	assert.Contains(t, err.Error(), "...")
}

func TestSynthesize_WAVResponsePreservesBody(t *testing.T) {
	// Ensure that the WAV magic byte check properly reconstructs the full body.
	wavData := []byte("RIFF\x24\x00\x00\x00WAVEfmt \x10\x00\x00\x00\x01\x00\x01\x00\x80\x3e\x00\x00\x00\x7d\x00\x00\x02\x00\x10\x00data\x00\x00\x00\x00")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "audio/wav")
		w.WriteHeader(http.StatusOK)
		w.Write(wavData)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "", nil)
	body, err := client.Synthesize(context.Background(), &SynthesizeRequest{
		Model: "tts-1",
		Voice: "alloy",
		Input: "Hello",
	})
	require.NoError(t, err)
	defer body.Close()

	data, err := io.ReadAll(body)
	require.NoError(t, err)
	assert.Equal(t, wavData, data)
}

func TestSynthesizeStream_RequestBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/audio/speech", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var reqBody map[string]any
		err := json.NewDecoder(r.Body).Decode(&reqBody)
		require.NoError(t, err)

		assert.Equal(t, true, reqBody["stream"])
		assert.Equal(t, "pcm", reqBody["response_format"])
		assert.Equal(t, "tts-1", reqBody["model"])
		assert.Equal(t, "alloy", reqBody["voice"])
		assert.Equal(t, "Hello", reqBody["input"])

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("pcm-data"))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "", nil)
	body, err := client.SynthesizeStream(context.Background(), &StreamSynthesizeRequest{
		Model: "tts-1",
		Voice: "alloy",
		Input: "Hello",
	})
	require.NoError(t, err)
	defer body.Close()
}

func TestSynthesizeStream_AuthHeaderPresent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer sk-test-key", r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("pcm-data"))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "sk-test-key", nil)
	body, err := client.SynthesizeStream(context.Background(), &StreamSynthesizeRequest{
		Model: "tts-1",
		Voice: "alloy",
		Input: "Hello",
	})
	require.NoError(t, err)
	body.Close()
}

func TestSynthesizeStream_AuthHeaderAbsentWhenEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Empty(t, r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("pcm-data"))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "", nil)
	body, err := client.SynthesizeStream(context.Background(), &StreamSynthesizeRequest{
		Model: "tts-1",
		Voice: "alloy",
		Input: "Hello",
	})
	require.NoError(t, err)
	body.Close()
}

func TestSynthesizeStream_NonSuccessReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "out of memory"}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "", nil)
	body, err := client.SynthesizeStream(context.Background(), &StreamSynthesizeRequest{
		Model: "tts-1",
		Voice: "alloy",
		Input: "Hello",
	})
	assert.Nil(t, body)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
	assert.Contains(t, err.Error(), "out of memory")
}

func TestSynthesizeStream_IncrementalRead(t *testing.T) {
	chunks := [][]byte{
		{0x01, 0x02, 0x03, 0x04},
		{0x05, 0x06, 0x07, 0x08},
		{0x09, 0x0a, 0x0b, 0x0c},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		flusher, ok := w.(http.Flusher)
		require.True(t, ok)

		w.WriteHeader(http.StatusOK)
		for _, chunk := range chunks {
			w.Write(chunk)
			flusher.Flush()
		}
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "", nil)
	body, err := client.SynthesizeStream(context.Background(), &StreamSynthesizeRequest{
		Model: "tts-1",
		Voice: "alloy",
		Input: "Hello",
	})
	require.NoError(t, err)
	defer body.Close()

	data, err := io.ReadAll(body)
	require.NoError(t, err)

	var expected []byte
	for _, chunk := range chunks {
		expected = append(expected, chunk...)
	}
	assert.Equal(t, expected, data)
}

func TestSynthesizeStream_OptionalFields(t *testing.T) {
	t.Run("omitted when nil", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var reqBody map[string]any
			err := json.NewDecoder(r.Body).Decode(&reqBody)
			require.NoError(t, err)

			assert.NotContains(t, reqBody, "speed")
			assert.NotContains(t, reqBody, "instructions")

			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		client := NewClient(srv.URL, "", nil)
		body, err := client.SynthesizeStream(context.Background(), &StreamSynthesizeRequest{
			Model: "tts-1",
			Voice: "alloy",
			Input: "Hello",
		})
		require.NoError(t, err)
		body.Close()
	})

	t.Run("included when set", func(t *testing.T) {
		speed := 1.5
		instructions := "speak slowly"

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var reqBody map[string]any
			err := json.NewDecoder(r.Body).Decode(&reqBody)
			require.NoError(t, err)

			assert.Equal(t, 1.5, reqBody["speed"])
			assert.Equal(t, "speak slowly", reqBody["instructions"])

			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		client := NewClient(srv.URL, "", nil)
		body, err := client.SynthesizeStream(context.Background(), &StreamSynthesizeRequest{
			Model:        "tts-1",
			Voice:        "alloy",
			Input:        "Hello",
			Speed:        &speed,
			Instructions: &instructions,
		})
		require.NoError(t, err)
		body.Close()
	})
}

func TestSynthesizeStream_ContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	client := NewClient(srv.URL, "", nil)
	body, err := client.SynthesizeStream(ctx, &StreamSynthesizeRequest{
		Model: "tts-1",
		Voice: "alloy",
		Input: "Hello",
	})
	assert.Nil(t, body)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tts: send request:")
}

func TestTruncateBody(t *testing.T) {
	assert.Equal(t, "short", truncateBody("short", 500))
	assert.Equal(t, "abc...", truncateBody("abcdef", 3))
	assert.Equal(t, "", truncateBody("", 10))
}
