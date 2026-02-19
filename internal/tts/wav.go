package tts

import (
	"encoding/binary"
	"fmt"
	"io"
)

// AudioFormat describes the PCM audio format extracted from a WAV header.
type AudioFormat struct {
	Rate     int // Sample rate in Hz (e.g. 24000)
	Width    int // Bytes per sample (e.g. 2 for 16-bit)
	Channels int // Number of channels (e.g. 1 for mono)
}

// WAVReader wraps an io.Reader to parse and strip the WAV header,
// exposing only raw PCM data. After calling ReadFormat, subsequent
// Read calls return PCM audio data with the header stripped.
type WAVReader struct {
	r      io.Reader
	format *AudioFormat
	parsed bool
}

// NewWAVReader creates a WAVReader that will parse the WAV header from r
// and then pass through raw PCM data.
func NewWAVReader(r io.Reader) *WAVReader {
	return &WAVReader{r: r}
}

// ReadFormat parses the WAV header and returns the audio format.
// It must be called before Read. It handles non-standard WAV files
// with extra chunks before the "data" chunk, and headers split
// across read boundaries via buffering.
func (w *WAVReader) ReadFormat() (*AudioFormat, error) {
	if w.parsed {
		return w.format, nil
	}

	// Read the RIFF header (12 bytes): "RIFF" + size + "WAVE"
	var riffHeader [12]byte
	if _, err := io.ReadFull(w.r, riffHeader[:]); err != nil {
		return nil, fmt.Errorf("wav: read RIFF header: %w", err)
	}
	if string(riffHeader[0:4]) != "RIFF" {
		return nil, fmt.Errorf("wav: not a RIFF file (got %q)", string(riffHeader[0:4]))
	}
	if string(riffHeader[8:12]) != "WAVE" {
		return nil, fmt.Errorf("wav: not a WAVE file (got %q)", string(riffHeader[8:12]))
	}

	// Read chunks until we find "fmt " and "data".
	var format *AudioFormat
	for {
		// Read chunk header: 4-byte ID + 4-byte little-endian size
		var chunkHeader [8]byte
		if _, err := io.ReadFull(w.r, chunkHeader[:]); err != nil {
			return nil, fmt.Errorf("wav: read chunk header: %w", err)
		}
		chunkID := string(chunkHeader[0:4])
		chunkSize := int64(binary.LittleEndian.Uint32(chunkHeader[4:8]))

		switch chunkID {
		case "fmt ":
			if chunkSize < 16 {
				return nil, fmt.Errorf("wav: fmt chunk too small (%d bytes)", chunkSize)
			}
			fmtData := make([]byte, chunkSize)
			if _, err := io.ReadFull(w.r, fmtData); err != nil {
				return nil, fmt.Errorf("wav: read fmt chunk: %w", err)
			}
			audioFmt := binary.LittleEndian.Uint16(fmtData[0:2])
			if audioFmt != 1 {
				return nil, fmt.Errorf("wav: unsupported audio format %d (expected PCM=1)", audioFmt)
			}
			channels := int(binary.LittleEndian.Uint16(fmtData[2:4]))
			sampleRate := int(binary.LittleEndian.Uint32(fmtData[4:8]))
			bitsPerSample := int(binary.LittleEndian.Uint16(fmtData[14:16]))
			format = &AudioFormat{
				Rate:     sampleRate,
				Width:    bitsPerSample / 8,
				Channels: channels,
			}

		case "data":
			// Found the data chunk. Everything after this is PCM.
			if format == nil {
				return nil, fmt.Errorf("wav: data chunk found before fmt chunk")
			}
			w.format = format
			w.parsed = true
			// If the data chunk has a finite size, wrap in a LimitReader.
			// For streaming WAV, size may be 0 or max uint32; in that case
			// read until EOF from the underlying reader.
			if chunkSize > 0 && chunkSize < 0x7FFFFFFF {
				w.r = io.LimitReader(w.r, chunkSize)
			}
			return w.format, nil

		default:
			// Skip unknown chunks (e.g. "LIST", "JUNK", "bext").
			// Chunk sizes must be word-aligned (padded to even).
			skipSize := chunkSize
			if skipSize%2 != 0 {
				skipSize++
			}
			if _, err := io.CopyN(io.Discard, w.r, skipSize); err != nil {
				return nil, fmt.Errorf("wav: skip chunk %q: %w", chunkID, err)
			}
		}
	}
}

// Read returns raw PCM data after the WAV header has been stripped.
// ReadFormat must be called before Read.
func (w *WAVReader) Read(p []byte) (int, error) {
	if !w.parsed {
		return 0, fmt.Errorf("wav: ReadFormat must be called before Read")
	}
	return w.r.Read(p)
}
