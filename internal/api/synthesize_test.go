package api

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/fx/meadowlark/internal/model"
	"github.com/fx/meadowlark/internal/tts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// synthUpstream is a mock OpenAI-compatible TTS server that records the last
// /audio/speech request body and answers with a canned response.
type synthUpstream struct {
	*httptest.Server
	mu          sync.Mutex
	got         map[string]any
	status      int
	contentType string
	body        []byte
}

func newSynthUpstream(t *testing.T, body []byte) *synthUpstream {
	t.Helper()
	u := &synthUpstream{status: http.StatusOK, body: body}
	u.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var got map[string]any
		_ = json.NewDecoder(r.Body).Decode(&got)
		u.mu.Lock()
		u.got = got
		u.mu.Unlock()
		if u.contentType != "" {
			w.Header().Set("Content-Type", u.contentType)
		}
		w.WriteHeader(u.status)
		_, _ = w.Write(u.body)
	}))
	t.Cleanup(u.Close)
	return u
}

func (u *synthUpstream) request() map[string]any {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.got
}

// wavBytes builds a PCM WAV with an explicit data-size field and optional
// chunks between fmt and data.
func wavBytes(rate, width, channels int, dataSize uint32, extra []byte, pcm []byte) []byte {
	var b bytes.Buffer
	b.WriteString("RIFF")
	_ = binary.Write(&b, binary.LittleEndian, uint32(36+len(extra)+len(pcm)))
	b.WriteString("WAVEfmt ")
	_ = binary.Write(&b, binary.LittleEndian, uint32(16))
	_ = binary.Write(&b, binary.LittleEndian, uint16(1))
	_ = binary.Write(&b, binary.LittleEndian, uint16(channels))
	_ = binary.Write(&b, binary.LittleEndian, uint32(rate))
	_ = binary.Write(&b, binary.LittleEndian, uint32(rate*width*channels))
	_ = binary.Write(&b, binary.LittleEndian, uint16(width*channels))
	_ = binary.Write(&b, binary.LittleEndian, uint16(width*8))
	b.Write(extra)
	b.WriteString("data")
	_ = binary.Write(&b, binary.LittleEndian, dataSize)
	b.Write(pcm)
	return b.Bytes()
}

func testPCM(n int) []byte {
	pcm := make([]byte, n)
	for i := range pcm {
		pcm[i] = byte(i * 7)
	}
	return pcm
}

func newSynthTestServer(t *testing.T, eps ...model.Endpoint) *httptest.Server {
	t.Helper()
	_, ts := newEndpointTestServer(&endpointMockStore{endpoints: eps})
	t.Cleanup(ts.Close)
	return ts
}

func postSynthesize(t *testing.T, ts *httptest.Server, body any) *http.Response {
	t.Helper()
	var reader io.Reader
	if s, ok := body.(string); ok {
		reader = strings.NewReader(s)
	} else {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		reader = bytes.NewReader(b)
	}
	resp, err := http.Post(ts.URL+"/api/v1/synthesize", "application/json", reader)
	require.NoError(t, err)
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func synthBody(epID string) map[string]any {
	return map[string]any{"endpoint_id": epID, "model": "tts-1", "voice": "alloy", "text": "Hello there"}
}

// readWAV parses a synthesize response body.
func readWAV(t *testing.T, resp *http.Response) (*tts.AudioFormat, []byte, []byte) {
	t.Helper()
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	wr := tts.NewWAVReader(bytes.NewReader(raw))
	f, err := wr.ReadFormat()
	require.NoError(t, err)
	pcm, err := io.ReadAll(wr)
	require.NoError(t, err)
	return f, pcm, raw
}

func assertSynthError(t *testing.T, resp *http.Response, status int, code, msgContains string) {
	t.Helper()
	assert.Equal(t, status, resp.StatusCode)
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))
	var body map[string]apiError
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, code, body["error"].Code)
	assert.Contains(t, body["error"].Message, msgContains)
}

func TestSynthesize_BufferedReturnsWAV(t *testing.T) {
	pcm := testPCM(960)
	up := newSynthUpstream(t, wavBytes(22050, 2, 1, uint32(len(pcm)), nil, pcm))
	ts := newSynthTestServer(t, model.Endpoint{ID: "ep", Name: "EP", BaseURL: up.URL, Enabled: true})

	body := synthBody("ep")
	body["speed"] = 1.25
	body["instructions"] = "whisper"
	resp := postSynthesize(t, ts, body)

	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "audio/wav", resp.Header.Get("Content-Type"))
	assert.Equal(t, "no-store", resp.Header.Get("Cache-Control"))
	assert.Equal(t, int64(44+len(pcm)), resp.ContentLength)

	f, gotPCM, raw := readWAV(t, resp)
	assert.Equal(t, tts.AudioFormat{Rate: 22050, Width: 2, Channels: 1}, *f)
	assert.Equal(t, pcm, gotPCM)
	assert.Equal(t, tts.WAVHeader(*f, uint32(len(pcm))), raw[:44])

	got := up.request()
	assert.Equal(t, "tts-1", got["model"])
	assert.Equal(t, "alloy", got["voice"])
	assert.Equal(t, "Hello there", got["input"])
	assert.Equal(t, "wav", got["response_format"])
	assert.Equal(t, 1.25, got["speed"])
	assert.Equal(t, "whisper", got["instructions"])
}

func TestSynthesize_TextIsSentLiterally(t *testing.T) {
	pcm := testPCM(64)
	up := newSynthUpstream(t, wavBytes(24000, 2, 1, uint32(len(pcm)), nil, pcm))
	ts := newSynthTestServer(t, model.Endpoint{ID: "ep", BaseURL: up.URL})

	body := synthBody("ep")
	body["text"] = `{"text":"hi","voice":"nova"}`
	resp := postSynthesize(t, ts, body)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, `{"text":"hi","voice":"nova"}`, up.request()["input"])
	assert.Equal(t, "alloy", up.request()["voice"])
}

func TestSynthesize_RebuildsStreamingWAVSize(t *testing.T) {
	for _, size := range []uint32{0, 0xFFFFFFFF} {
		pcm := testPCM(480)
		up := newSynthUpstream(t, wavBytes(24000, 2, 1, size, nil, pcm))
		ts := newSynthTestServer(t, model.Endpoint{ID: "ep", BaseURL: up.URL})

		resp := postSynthesize(t, ts, synthBody("ep"))
		require.Equal(t, http.StatusOK, resp.StatusCode)
		_, gotPCM, raw := readWAV(t, resp)
		assert.Equal(t, pcm, gotPCM)
		assert.Equal(t, uint32(len(pcm)), binary.LittleEndian.Uint32(raw[40:44]), "size %#x", size)
		assert.Equal(t, uint32(36+len(pcm)), binary.LittleEndian.Uint32(raw[4:8]))
	}
}

func TestSynthesize_StripsExtraChunks(t *testing.T) {
	pcm := testPCM(200)
	list := append([]byte("LIST"), 4, 0, 0, 0, 'I', 'N', 'F', 'O')
	up := newSynthUpstream(t, wavBytes(24000, 2, 1, uint32(len(pcm)), list, pcm))
	ts := newSynthTestServer(t, model.Endpoint{ID: "ep", BaseURL: up.URL})

	resp := postSynthesize(t, ts, synthBody("ep"))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_, gotPCM, raw := readWAV(t, resp)
	assert.Equal(t, pcm, gotPCM)
	assert.Len(t, raw, 44+len(pcm))
	assert.NotContains(t, string(raw), "LIST")
}

func TestSynthesize_StreamingEndpoint(t *testing.T) {
	pcm := testPCM(1000)
	up := newSynthUpstream(t, pcm)
	ts := newSynthTestServer(t, model.Endpoint{ID: "ep", BaseURL: up.URL, StreamingEnabled: true, StreamSampleRate: 16000})

	resp := postSynthesize(t, ts, synthBody("ep"))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	f, gotPCM, _ := readWAV(t, resp)
	assert.Equal(t, tts.AudioFormat{Rate: 16000, Width: 2, Channels: 1}, *f)
	assert.Equal(t, pcm, gotPCM)

	got := up.request()
	assert.Equal(t, true, got["stream"])
	assert.Equal(t, "pcm", got["response_format"])
	assert.Equal(t, "audio", got["stream_format"])
}

func TestSynthesize_StreamingDefaultRate(t *testing.T) {
	up := newSynthUpstream(t, testPCM(100))
	ts := newSynthTestServer(t, model.Endpoint{ID: "ep", BaseURL: up.URL, StreamingEnabled: true})

	resp := postSynthesize(t, ts, synthBody("ep"))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	f, _, _ := readWAV(t, resp)
	assert.Equal(t, 24000, f.Rate)
}

func TestSynthesize_TrimsPartialFrame(t *testing.T) {
	pcm := testPCM(101)
	up := newSynthUpstream(t, pcm)
	ts := newSynthTestServer(t, model.Endpoint{ID: "ep", BaseURL: up.URL, StreamingEnabled: true})

	resp := postSynthesize(t, ts, synthBody("ep"))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_, gotPCM, _ := readWAV(t, resp)
	assert.Equal(t, pcm[:100], gotPCM)
}

func TestSynthesize_UpstreamFailures(t *testing.T) {
	t.Run("event stream", func(t *testing.T) {
		up := newSynthUpstream(t, []byte("data: {}\n\n"))
		up.contentType = "text/event-stream"
		ts := newSynthTestServer(t, model.Endpoint{ID: "ep", BaseURL: up.URL, StreamingEnabled: true})
		assertSynthError(t, postSynthesize(t, ts, synthBody("ep")), http.StatusBadGateway, "upstream_error", "event stream")
	})

	t.Run("upstream error status and body are reported", func(t *testing.T) {
		up := newSynthUpstream(t, []byte(`unknown voice alloy`))
		up.status = http.StatusBadRequest
		ts := newSynthTestServer(t, model.Endpoint{ID: "ep", BaseURL: up.URL})
		assertSynthError(t, postSynthesize(t, ts, synthBody("ep")), http.StatusBadGateway, "upstream_error", "synthesis failed: tts api call: tts: API error 400: unknown voice alloy")
	})

	t.Run("JSON instead of WAV", func(t *testing.T) {
		up := newSynthUpstream(t, []byte(`{"ok":true}`))
		ts := newSynthTestServer(t, model.Endpoint{ID: "ep", BaseURL: up.URL})
		assertSynthError(t, postSynthesize(t, ts, synthBody("ep")), http.StatusBadGateway, "upstream_error", "non-WAV")
	})

	t.Run("no audio", func(t *testing.T) {
		up := newSynthUpstream(t, nil)
		ts := newSynthTestServer(t, model.Endpoint{ID: "ep", BaseURL: up.URL, StreamingEnabled: true})
		assertSynthError(t, postSynthesize(t, ts, synthBody("ep")), http.StatusBadGateway, "upstream_error", "endpoint returned no audio")
	})

	t.Run("only a partial frame", func(t *testing.T) {
		up := newSynthUpstream(t, []byte{1})
		ts := newSynthTestServer(t, model.Endpoint{ID: "ep", BaseURL: up.URL, StreamingEnabled: true})
		assertSynthError(t, postSynthesize(t, ts, synthBody("ep")), http.StatusBadGateway, "upstream_error", "endpoint returned no audio")
	})

	t.Run("over the audio cap", func(t *testing.T) {
		old := maxSynthesizeAudioBytes
		maxSynthesizeAudioBytes = 1 << 20
		t.Cleanup(func() { maxSynthesizeAudioBytes = old })

		up := newSynthUpstream(t, make([]byte, 1<<20+2))
		ts := newSynthTestServer(t, model.Endpoint{ID: "ep", BaseURL: up.URL, StreamingEnabled: true})
		assertSynthError(t, postSynthesize(t, ts, synthBody("ep")), http.StatusBadGateway, "upstream_error", "audio exceeds the 1 MiB limit")
	})

	t.Run("exactly the audio cap", func(t *testing.T) {
		old := maxSynthesizeAudioBytes
		maxSynthesizeAudioBytes = 1 << 20
		t.Cleanup(func() { maxSynthesizeAudioBytes = old })

		up := newSynthUpstream(t, make([]byte, 1<<20))
		ts := newSynthTestServer(t, model.Endpoint{ID: "ep", BaseURL: up.URL, StreamingEnabled: true})
		assert.Equal(t, http.StatusOK, postSynthesize(t, ts, synthBody("ep")).StatusCode)
	})

	t.Run("unsupported response format", func(t *testing.T) {
		up := newSynthUpstream(t, nil)
		ts := newSynthTestServer(t, model.Endpoint{ID: "ep", BaseURL: up.URL, DefaultResponseFormat: "mp3"})
		assertSynthError(t, postSynthesize(t, ts, synthBody("ep")), http.StatusBadGateway, "upstream_error", "unsupported response format")
	})

	t.Run("unparseable WAV header", func(t *testing.T) {
		up := newSynthUpstream(t, []byte("RIFF\x00\x00\x00\x00JUNK"))
		ts := newSynthTestServer(t, model.Endpoint{ID: "ep", BaseURL: up.URL})
		assertSynthError(t, postSynthesize(t, ts, synthBody("ep")), http.StatusBadGateway, "upstream_error", "parse wav header")
	})

	t.Run("invalid audio format", func(t *testing.T) {
		up := newSynthUpstream(t, wavBytes(24000, 2, 0, 4, nil, []byte{1, 2, 3, 4}))
		ts := newSynthTestServer(t, model.Endpoint{ID: "ep", BaseURL: up.URL})
		assertSynthError(t, postSynthesize(t, ts, synthBody("ep")), http.StatusBadGateway, "upstream_error", "invalid audio format")
	})

	t.Run("upstream unreachable", func(t *testing.T) {
		up := newSynthUpstream(t, nil)
		up.Close()
		ts := newSynthTestServer(t, model.Endpoint{ID: "ep", BaseURL: up.URL})
		assertSynthError(t, postSynthesize(t, ts, synthBody("ep")), http.StatusBadGateway, "upstream_error", "synthesis failed: ")
	})
}

// The upstream declares a body longer than it sends, so reading the PCM fails
// with an unexpected EOF after the response has been accepted.
func TestSynthesize_ReadErrorIsUpstreamError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "1000")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(testPCM(10))
	}))
	defer srv.Close()
	ts := newSynthTestServer(t, model.Endpoint{ID: "ep", BaseURL: srv.URL, StreamingEnabled: true})
	assertSynthError(t, postSynthesize(t, ts, synthBody("ep")), http.StatusBadGateway, "upstream_error", "read audio")
}

func TestSynthesize_EndpointDefaults(t *testing.T) {
	speed := 0.8
	instr := "be cheerful"
	ep := model.Endpoint{ID: "ep", DefaultSpeed: &speed, DefaultInstructions: &instr}

	t.Run("omitted fields use endpoint defaults", func(t *testing.T) {
		up := newSynthUpstream(t, testPCM(10))
		ep := ep
		ep.BaseURL = up.URL
		ep.StreamingEnabled = true
		ts := newSynthTestServer(t, ep)
		require.Equal(t, http.StatusOK, postSynthesize(t, ts, synthBody("ep")).StatusCode)
		assert.Equal(t, 0.8, up.request()["speed"])
		assert.Equal(t, "be cheerful", up.request()["instructions"])
	})

	t.Run("supplied fields override", func(t *testing.T) {
		up := newSynthUpstream(t, testPCM(10))
		ep := ep
		ep.BaseURL = up.URL
		ep.StreamingEnabled = true
		ts := newSynthTestServer(t, ep)
		body := synthBody("ep")
		body["speed"] = 2.0
		body["instructions"] = "slowly"
		require.Equal(t, http.StatusOK, postSynthesize(t, ts, body).StatusCode)
		assert.Equal(t, 2.0, up.request()["speed"])
		assert.Equal(t, "slowly", up.request()["instructions"])
	})
}

func TestSynthesize_DisabledEndpointIsSynthesized(t *testing.T) {
	up := newSynthUpstream(t, testPCM(10))
	ts := newSynthTestServer(t, model.Endpoint{ID: "ep", BaseURL: up.URL, StreamingEnabled: true, Enabled: false})
	assert.Equal(t, http.StatusOK, postSynthesize(t, ts, synthBody("ep")).StatusCode)
}

func TestSynthesize_Validation(t *testing.T) {
	with := func(k string, v any) map[string]any {
		b := synthBody("ep")
		if v == nil {
			delete(b, k)
		} else {
			b[k] = v
		}
		return b
	}
	cases := []struct {
		name string
		body any
		msg  string
	}{
		{"invalid JSON", "{not json", "invalid JSON body"},
		{"oversize body", `{"text":"` + strings.Repeat("a", 70<<10) + `"}`, "invalid JSON body"},
		{"missing endpoint_id", with("endpoint_id", "  "), "endpoint_id is required"},
		{"missing model", with("model", nil), "model is required"},
		{"missing voice", with("voice", ""), "voice is required"},
		{"missing text", with("text", nil), "text is required"},
		{"whitespace text", with("text", " \n\t "), "text is required"},
		{"text too long", with("text", strings.Repeat("é", 4097)), "text must be at most 4096 characters"},
		{"speed too low", with("speed", 0.2), "speed must be between 0.25 and 4.0"},
		{"speed too high", with("speed", 4.1), "speed must be between 0.25 and 4.0"},
	}
	ts := newSynthTestServer(t, model.Endpoint{ID: "ep", BaseURL: "http://127.0.0.1:1"})
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertSynthError(t, postSynthesize(t, ts, tc.body), http.StatusBadRequest, "bad_request", tc.msg)
		})
	}
}

func TestSynthesize_TextAtLimitIsAccepted(t *testing.T) {
	up := newSynthUpstream(t, testPCM(10))
	ts := newSynthTestServer(t, model.Endpoint{ID: "ep", BaseURL: up.URL, StreamingEnabled: true})
	body := synthBody("ep")
	body["text"] = strings.Repeat("é", 4096)
	assert.Equal(t, http.StatusOK, postSynthesize(t, ts, body).StatusCode)
}

func TestSynthesize_EndpointNotFound(t *testing.T) {
	ts := newSynthTestServer(t)
	assertSynthError(t, postSynthesize(t, ts, synthBody("missing")), http.StatusBadRequest, "bad_request", "endpoint not found")
}

// failingEndpointStore fails every GetEndpoint call.
type failingEndpointStore struct{ endpointMockStore }

func (m *failingEndpointStore) GetEndpoint(_ context.Context, _ string) (*model.Endpoint, error) {
	return nil, errors.New("db down")
}

func TestSynthesize_StoreError(t *testing.T) {
	srv := &Server{store: &failingEndpointStore{}}
	ts := httptest.NewServer(srv.setupRoutes())
	defer ts.Close()
	assertSynthError(t, postSynthesize(t, ts, synthBody("ep")), http.StatusInternalServerError, "internal_error", "failed to get endpoint")
}

func TestSynthesize_WithoutDeadlineSupport(t *testing.T) {
	// httptest.ResponseRecorder does not support SetWriteDeadline; the handler
	// must still succeed.
	up := newSynthUpstream(t, testPCM(10))
	srv, ts := newEndpointTestServer(&endpointMockStore{endpoints: []model.Endpoint{{ID: "ep", BaseURL: up.URL, StreamingEnabled: true}}})
	ts.Close()

	b, err := json.Marshal(synthBody("ep"))
	require.NoError(t, err)
	rec := httptest.NewRecorder()
	srv.Synthesize(rec, httptest.NewRequest(http.MethodPost, "/api/v1/synthesize", bytes.NewReader(b)))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "audio/wav", rec.Header().Get("Content-Type"))
	assert.Equal(t, 54, rec.Body.Len())
}
