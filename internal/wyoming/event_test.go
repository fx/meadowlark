package wyoming

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadWriteRoundTrip(t *testing.T) {
	tests := []struct {
		name  string
		event Event
	}{
		{
			name:  "type only",
			event: Event{Type: "ping"},
		},
		{
			name: "with data",
			event: Event{
				Type: "synthesize",
				Data: map[string]any{
					"text": "hello world",
				},
			},
		},
		{
			name: "with payload",
			event: Event{
				Type:    "audio-chunk",
				Data:    map[string]any{"rate": float64(24000), "width": float64(2), "channels": float64(1)},
				Payload: []byte{0x01, 0x02, 0x03, 0x04},
			},
		},
		{
			name: "with data and payload",
			event: Event{
				Type:    "audio-chunk",
				Data:    map[string]any{"rate": float64(22050), "width": float64(2), "channels": float64(2)},
				Payload: bytes.Repeat([]byte{0xAB, 0xCD}, 1024),
			},
		},
		{
			name:  "empty data map",
			event: Event{Type: "describe"},
		},
		{
			name: "nested data",
			event: Event{
				Type: "synthesize",
				Data: map[string]any{
					"text":  "hi",
					"voice": map[string]any{"name": "alloy"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := WriteEvent(&buf, &tt.event)
			require.NoError(t, err)

			reader := bufio.NewReader(&buf)
			got, err := ReadEvent(reader)
			require.NoError(t, err)

			assert.Equal(t, tt.event.Type, got.Type)
			if len(tt.event.Data) > 0 {
				assert.Equal(t, len(tt.event.Data), len(got.Data))
				wantJSON, _ := json.Marshal(tt.event.Data)
				gotJSON, _ := json.Marshal(got.Data)
				assert.JSONEq(t, string(wantJSON), string(gotJSON))
			} else {
				assert.Empty(t, got.Data)
			}
			assert.Equal(t, tt.event.Payload, got.Payload)
		})
	}
}

func TestReadMultipleEvents(t *testing.T) {
	events := []*Event{
		{Type: "ping"},
		{Type: "synthesize", Data: map[string]any{"text": "hello"}},
		{Type: "audio-chunk", Data: map[string]any{"rate": float64(24000)}, Payload: []byte{1, 2, 3}},
	}

	var buf bytes.Buffer
	for _, ev := range events {
		require.NoError(t, WriteEvent(&buf, ev))
	}

	reader := bufio.NewReader(&buf)
	for i, want := range events {
		got, err := ReadEvent(reader)
		require.NoError(t, err, "event %d", i)
		assert.Equal(t, want.Type, got.Type)
		if len(want.Payload) > 0 {
			assert.Equal(t, want.Payload, got.Payload)
		}
	}
}

func TestReadEventInlineData(t *testing.T) {
	h := `{"type":"synthesize","version":"1.8.0","data":{"text":"inline"}}` + "\n"
	reader := bufio.NewReader(strings.NewReader(h))
	ev, err := ReadEvent(reader)
	require.NoError(t, err)
	assert.Equal(t, "synthesize", ev.Type)
	assert.Equal(t, "inline", ev.Data["text"])
}

func TestReadEventExternalDataOverridesInline(t *testing.T) {
	externalData := `{"text":"external"}`
	h := header{
		Type:       "synthesize",
		Version:    ProtocolVersion,
		DataLength: len(externalData),
		Data:       map[string]any{"text": "inline"},
	}
	hBytes, _ := json.Marshal(h)
	raw := string(hBytes) + "\n" + externalData

	reader := bufio.NewReader(strings.NewReader(raw))
	ev, err := ReadEvent(reader)
	require.NoError(t, err)
	assert.Equal(t, "external", ev.Data["text"])
}

func TestReadEventErrors(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{
			name:    "empty input",
			input:   "",
			wantErr: "read header line",
		},
		{
			name:    "invalid JSON",
			input:   "not json\n",
			wantErr: "parse header",
		},
		{
			name:    "missing type",
			input:   `{"version":"1.8.0"}` + "\n",
			wantErr: "missing event type",
		},
		{
			name:    "truncated data bytes",
			input:   `{"type":"test","version":"1.8.0","data_length":100}` + "\n" + "short",
			wantErr: "read data bytes",
		},
		{
			name:    "invalid data JSON",
			input:   `{"type":"test","version":"1.8.0","data_length":3}` + "\n" + "abc",
			wantErr: "parse data bytes",
		},
		{
			name:    "truncated payload bytes",
			input:   `{"type":"test","version":"1.8.0","payload_length":100}` + "\n" + "short",
			wantErr: "read payload bytes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := bufio.NewReader(strings.NewReader(tt.input))
			_, err := ReadEvent(reader)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestWriteEventHeaderFormat(t *testing.T) {
	ev := &Event{
		Type:    "audio-chunk",
		Data:    map[string]any{"rate": 24000},
		Payload: []byte{0x01, 0x02},
	}

	var buf bytes.Buffer
	require.NoError(t, WriteEvent(&buf, ev))

	reader := bufio.NewReader(&buf)
	headerLine, err := reader.ReadBytes('\n')
	require.NoError(t, err)

	var h header
	require.NoError(t, json.Unmarshal(headerLine, &h))

	assert.Equal(t, "audio-chunk", h.Type)
	assert.Equal(t, ProtocolVersion, h.Version)
	assert.Greater(t, h.DataLength, 0)
	assert.Equal(t, 2, h.PayloadLength)
}

func TestWriteEventNoDataNoPayload(t *testing.T) {
	ev := &Event{Type: "ping"}
	var buf bytes.Buffer
	require.NoError(t, WriteEvent(&buf, ev))

	reader := bufio.NewReader(&buf)
	headerLine, err := reader.ReadBytes('\n')
	require.NoError(t, err)

	var h header
	require.NoError(t, json.Unmarshal(headerLine, &h))

	assert.Equal(t, "ping", h.Type)
	assert.Equal(t, 0, h.DataLength)
	assert.Equal(t, 0, h.PayloadLength)

	_, err = reader.ReadByte()
	assert.Equal(t, io.EOF, err)
}

func TestLargePayloadRoundTrip(t *testing.T) {
	payload := make([]byte, 1024*1024)
	for i := range payload {
		payload[i] = byte(i % 256)
	}
	ev := &Event{
		Type:    "audio-chunk",
		Data:    map[string]any{"rate": float64(48000)},
		Payload: payload,
	}

	var buf bytes.Buffer
	require.NoError(t, WriteEvent(&buf, ev))

	reader := bufio.NewReader(&buf)
	got, err := ReadEvent(reader)
	require.NoError(t, err)
	assert.Equal(t, payload, got.Payload)
}

func TestEmptyDataMapNotWritten(t *testing.T) {
	ev := &Event{Type: "pong", Data: map[string]any{}}
	var buf bytes.Buffer
	require.NoError(t, WriteEvent(&buf, ev))

	reader := bufio.NewReader(&buf)
	headerLine, err := reader.ReadBytes('\n')
	require.NoError(t, err)

	var h header
	require.NoError(t, json.Unmarshal(headerLine, &h))
	assert.Equal(t, 0, h.DataLength)
}

func TestNilPayloadNotWritten(t *testing.T) {
	ev := &Event{Type: "describe", Payload: nil}
	var buf bytes.Buffer
	require.NoError(t, WriteEvent(&buf, ev))

	reader := bufio.NewReader(&buf)
	got, err := ReadEvent(reader)
	require.NoError(t, err)
	assert.Nil(t, got.Payload)
}

type errWriter struct {
	maxBytes int
	written  int
}

func (w *errWriter) Write(p []byte) (int, error) {
	if w.written+len(p) > w.maxBytes {
		remaining := w.maxBytes - w.written
		w.written = w.maxBytes
		return remaining, io.ErrShortWrite
	}
	w.written += len(p)
	return len(p), nil
}

func TestWriteEventHeaderWriteError(t *testing.T) {
	ev := &Event{Type: "ping"}
	w := &errWriter{maxBytes: 5}
	err := WriteEvent(w, ev)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "write header")
}

func TestWriteEventNewlineWriteError(t *testing.T) {
	ev := &Event{Type: "ping"}
	headerJSON := `{"type":"ping","version":"1.8.0"}`
	w := &errWriter{maxBytes: len(headerJSON)}
	err := WriteEvent(w, ev)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "write header newline")
}

func TestWriteEventDataWriteError(t *testing.T) {
	ev := &Event{Type: "test", Data: map[string]any{"key": "value"}}
	headerJSON := `{"type":"test","version":"1.8.0","data_length":15}`
	w := &errWriter{maxBytes: len(headerJSON) + 1}
	err := WriteEvent(w, ev)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "write data")
}

func TestWriteEventPayloadWriteError(t *testing.T) {
	ev := &Event{Type: "test", Payload: []byte{0x01, 0x02, 0x03}}
	headerJSON := `{"type":"test","version":"1.8.0","payload_length":3}`
	w := &errWriter{maxBytes: len(headerJSON) + 1}
	err := WriteEvent(w, ev)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "write payload")
}
