package wyoming

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
)

// ProtocolVersion is the Wyoming protocol version.
const ProtocolVersion = "1.8.0"

// Event represents a Wyoming protocol event with optional data and binary payload.
type Event struct {
	Type    string
	Data    map[string]any
	Payload []byte
}

// header is the JSON structure written/read as the first line of a Wyoming event.
type header struct {
	Type          string         `json:"type"`
	Version       string         `json:"version"`
	DataLength    int            `json:"data_length,omitempty"`
	PayloadLength int            `json:"payload_length,omitempty"`
	Data          map[string]any `json:"data,omitempty"`
}

// ReadEvent reads a single Wyoming event from the given reader.
//
// Wire format:
//  1. JSON header line terminated by \n
//  2. Optional UTF-8 JSON data bytes (length = data_length)
//  3. Optional raw binary payload bytes (length = payload_length)
//
// When data_length is 0 and the header contains a "data" field, the inline
// data from the header is used directly.
func ReadEvent(r *bufio.Reader) (*Event, error) {
	line, err := r.ReadBytes('\n')
	if err != nil {
		return nil, fmt.Errorf("read header line: %w", err)
	}

	var h header
	if err := json.Unmarshal(line, &h); err != nil {
		return nil, fmt.Errorf("parse header: %w", err)
	}

	if h.Type == "" {
		return nil, fmt.Errorf("missing event type in header")
	}

	ev := &Event{
		Type: h.Type,
	}

	// Read external data bytes if data_length > 0.
	if h.DataLength > 0 {
		dataBuf := make([]byte, h.DataLength)
		if _, err := io.ReadFull(r, dataBuf); err != nil {
			return nil, fmt.Errorf("read data bytes: %w", err)
		}
		var data map[string]any
		if err := json.Unmarshal(dataBuf, &data); err != nil {
			return nil, fmt.Errorf("parse data bytes: %w", err)
		}
		ev.Data = data
	} else if h.Data != nil {
		// Use inline data from the header.
		ev.Data = h.Data
	}

	// Read payload bytes if payload_length > 0.
	if h.PayloadLength > 0 {
		ev.Payload = make([]byte, h.PayloadLength)
		if _, err := io.ReadFull(r, ev.Payload); err != nil {
			return nil, fmt.Errorf("read payload bytes: %w", err)
		}
	}

	return ev, nil
}

// WriteEvent writes a single Wyoming event to the given writer.
//
// Wire format:
//  1. JSON header line terminated by \n (contains type, version, data_length, payload_length)
//  2. UTF-8 JSON data bytes (if data is non-empty)
//  3. Raw binary payload bytes (if payload is non-empty)
//
// Data is always written as external bytes (not inlined in the header) so
// that the header stays compact and the format is unambiguous.
func WriteEvent(w io.Writer, ev *Event) error {
	h := header{
		Type:    ev.Type,
		Version: ProtocolVersion,
	}

	var dataBytes []byte
	if len(ev.Data) > 0 {
		var err error
		dataBytes, err = json.Marshal(ev.Data)
		if err != nil {
			return fmt.Errorf("marshal data: %w", err)
		}
		h.DataLength = len(dataBytes)
	}

	if len(ev.Payload) > 0 {
		h.PayloadLength = len(ev.Payload)
	}

	headerBytes, err := json.Marshal(h)
	if err != nil {
		return fmt.Errorf("marshal header: %w", err)
	}

	// Write header line.
	if _, err := w.Write(headerBytes); err != nil {
		return fmt.Errorf("write header: %w", err)
	}
	if _, err := w.Write([]byte("\n")); err != nil {
		return fmt.Errorf("write header newline: %w", err)
	}

	// Write data bytes.
	if len(dataBytes) > 0 {
		if _, err := w.Write(dataBytes); err != nil {
			return fmt.Errorf("write data: %w", err)
		}
	}

	// Write payload bytes.
	if len(ev.Payload) > 0 {
		if _, err := w.Write(ev.Payload); err != nil {
			return fmt.Errorf("write payload: %w", err)
		}
	}

	return nil
}
