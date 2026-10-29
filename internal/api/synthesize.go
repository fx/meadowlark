package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/fx/meadowlark/internal/model"
	"github.com/fx/meadowlark/internal/tts"
	"github.com/fx/meadowlark/internal/voice"
)

// maxSynthesizeBodyBytes caps the JSON request body of POST /synthesize.
const maxSynthesizeBodyBytes = 64 << 10

// maxSynthesizeTextRunes is the longest text, in code points, POST /synthesize
// accepts.
const maxSynthesizeTextRunes = 4096

// synthesizeTimeout bounds one synthesis request, upstream call included. The
// server's WriteTimeout is extended past it for this handler only.
const synthesizeTimeout = 5 * time.Minute

// maxSynthesizeAudioBytes caps the PCM a synthesis may buffer. It is a variable
// so tests can lower it.
var maxSynthesizeAudioBytes int64 = 64 << 20

// synthesizeRequest is the JSON body for POST /synthesize.
type synthesizeRequest struct {
	EndpointID   string   `json:"endpoint_id"`
	Model        string   `json:"model"`
	Voice        string   `json:"voice"`
	Text         string   `json:"text"`
	Speed        *float64 `json:"speed,omitempty"`
	Instructions *string  `json:"instructions,omitempty"`
}

// validate returns the message of the first rule req breaks, or "".
func (req *synthesizeRequest) validate() string {
	switch {
	case strings.TrimSpace(req.EndpointID) == "":
		return "endpoint_id is required"
	case strings.TrimSpace(req.Model) == "":
		return "model is required"
	case strings.TrimSpace(req.Voice) == "":
		return "voice is required"
	case strings.TrimSpace(req.Text) == "":
		return "text is required"
	case utf8.RuneCountInString(req.Text) > maxSynthesizeTextRunes:
		return fmt.Sprintf("text must be at most %d characters", maxSynthesizeTextRunes)
	case req.Speed != nil && (*req.Speed < 0.25 || *req.Speed > 4.0):
		return "speed must be between 0.25 and 4.0"
	}
	return ""
}

// Synthesize synthesizes text against an endpoint with explicit parameters and
// returns the audio as a WAV file. It is stateless: nothing is resolved by
// voice name and nothing is stored. The upstream request is opened exactly as
// the Wyoming proxy opens it, so streaming endpoints behave as in production.
func (s *Server) Synthesize(w http.ResponseWriter, r *http.Request) {
	var req synthesizeRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxSynthesizeBodyBytes)).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	if msg := req.validate(); msg != "" {
		respondError(w, http.StatusBadRequest, "bad_request", msg)
		return
	}

	ep, err := s.store.GetEndpoint(r.Context(), req.EndpointID)
	if err != nil {
		slog.Error("synthesize: get endpoint", "error", err, "endpoint_id", req.EndpointID)
		respondError(w, http.StatusInternalServerError, "internal_error", "failed to get endpoint")
		return
	}
	if ep == nil {
		respondError(w, http.StatusBadRequest, "bad_request", "endpoint not found")
		return
	}

	// A long text can take longer than the server-wide WriteTimeout.
	if err := http.NewResponseController(w).SetWriteDeadline(time.Now().Add(synthesizeTimeout + 10*time.Second)); err != nil {
		slog.Debug("synthesize: cannot extend write deadline", "error", err)
	}
	ctx, cancel := context.WithTimeout(r.Context(), synthesizeTimeout)
	defer cancel()

	// Omitted speed and instructions fall back to the endpoint's defaults, as
	// they do for an alias in the Wyoming path.
	params := voice.MergeParams(
		voice.ParsedInput{Model: req.Model, Voice: req.Voice, Speed: req.Speed, Instructions: req.Instructions},
		nil,
		&voice.ParsedInput{Speed: ep.DefaultSpeed, Instructions: ep.DefaultInstructions},
	)

	start := time.Now()
	format, pcm, err := s.synthesizePCM(ctx, ep, req.Text, params)
	if err != nil {
		slog.Warn("synthesize failed", "error", err, "endpoint_id", ep.ID, "model", params.Model, "voice", params.Voice)
		respondError(w, http.StatusBadGateway, "upstream_error", err.Error())
		return
	}

	header := tts.WAVHeader(*format, uint32(len(pcm)))
	w.Header().Set("Content-Type", "audio/wav")
	w.Header().Set("Content-Length", strconv.Itoa(len(header)+len(pcm)))
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	if _, err := io.Copy(w, io.MultiReader(bytes.NewReader(header), bytes.NewReader(pcm))); err != nil {
		slog.Warn("synthesize: write audio", "error", err)
	}

	slog.Info("synthesized",
		"endpoint_id", ep.ID,
		"model", params.Model,
		"voice", params.Voice,
		"text_len", len(req.Text),
		"bytes", len(pcm),
		"duration", time.Since(start),
	)
}

// synthesizePCM opens the upstream segment for text and reads all of its PCM,
// trimmed to whole frames. The returned error is the client-facing message.
func (s *Server) synthesizePCM(ctx context.Context, ep *model.Endpoint, text string, params voice.ParsedInput) (*tts.AudioFormat, []byte, error) {
	seg, err := tts.OpenEndpointSegment(ctx, s.clientFactory(ep), ep, params, text)
	if err != nil {
		return nil, nil, fmt.Errorf("synthesis failed: %w", err)
	}
	defer seg.Close()

	var buf bytes.Buffer
	n, err := io.Copy(&buf, io.LimitReader(seg, maxSynthesizeAudioBytes+1))
	if err != nil {
		return nil, nil, fmt.Errorf("synthesis failed: read audio: %w", err)
	}
	if n > maxSynthesizeAudioBytes {
		return nil, nil, fmt.Errorf("audio exceeds the %d MiB limit", maxSynthesizeAudioBytes>>20)
	}

	format := seg.Format()
	frame := format.Width * format.Channels
	if frame <= 0 || format.Rate <= 0 {
		return nil, nil, fmt.Errorf("synthesis failed: invalid audio format (%d Hz, %d bytes/sample, %d channels)", format.Rate, format.Width, format.Channels)
	}
	pcm := buf.Bytes()
	pcm = pcm[:len(pcm)-len(pcm)%frame]
	if len(pcm) == 0 {
		return nil, nil, fmt.Errorf("endpoint returned no audio")
	}
	return format, pcm, nil
}
