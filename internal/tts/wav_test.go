package tts

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildWAV constructs a valid WAV file with the given parameters and PCM data.
func buildWAV(rate, bitsPerSample, channels int, pcmData []byte) []byte {
	width := bitsPerSample / 8
	dataSize := len(pcmData)
	fmtChunkSize := 16
	totalSize := 4 + (8 + fmtChunkSize) + (8 + dataSize)

	buf := &bytes.Buffer{}

	buf.WriteString("RIFF")
	binary.Write(buf, binary.LittleEndian, uint32(totalSize))
	buf.WriteString("WAVE")

	buf.WriteString("fmt ")
	binary.Write(buf, binary.LittleEndian, uint32(fmtChunkSize))
	binary.Write(buf, binary.LittleEndian, uint16(1))
	binary.Write(buf, binary.LittleEndian, uint16(channels))
	binary.Write(buf, binary.LittleEndian, uint32(rate))
	binary.Write(buf, binary.LittleEndian, uint32(rate*channels*width))
	binary.Write(buf, binary.LittleEndian, uint16(channels*width))
	binary.Write(buf, binary.LittleEndian, uint16(bitsPerSample))

	buf.WriteString("data")
	binary.Write(buf, binary.LittleEndian, uint32(dataSize))
	buf.Write(pcmData)

	return buf.Bytes()
}

func buildWAVWithExtraChunks(rate, bitsPerSample, channels int, pcmData []byte, extraChunks []struct {
	ID   string
	Data []byte
}) []byte {
	width := bitsPerSample / 8
	dataSize := len(pcmData)
	fmtChunkSize := 16

	extraSize := 0
	for _, c := range extraChunks {
		chunkSize := len(c.Data)
		extraSize += 8 + chunkSize
		if chunkSize%2 != 0 {
			extraSize++
		}
	}

	totalSize := 4 + (8 + fmtChunkSize) + extraSize + (8 + dataSize)

	buf := &bytes.Buffer{}

	buf.WriteString("RIFF")
	binary.Write(buf, binary.LittleEndian, uint32(totalSize))
	buf.WriteString("WAVE")

	buf.WriteString("fmt ")
	binary.Write(buf, binary.LittleEndian, uint32(fmtChunkSize))
	binary.Write(buf, binary.LittleEndian, uint16(1))
	binary.Write(buf, binary.LittleEndian, uint16(channels))
	binary.Write(buf, binary.LittleEndian, uint32(rate))
	binary.Write(buf, binary.LittleEndian, uint32(rate*channels*width))
	binary.Write(buf, binary.LittleEndian, uint16(channels*width))
	binary.Write(buf, binary.LittleEndian, uint16(bitsPerSample))

	for _, c := range extraChunks {
		buf.WriteString(c.ID)
		binary.Write(buf, binary.LittleEndian, uint32(len(c.Data)))
		buf.Write(c.Data)
		if len(c.Data)%2 != 0 {
			buf.WriteByte(0)
		}
	}

	buf.WriteString("data")
	binary.Write(buf, binary.LittleEndian, uint32(dataSize))
	buf.Write(pcmData)

	return buf.Bytes()
}

func TestWAVReader_Standard44ByteHeader(t *testing.T) {
	pcm := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	wav := buildWAV(24000, 16, 1, pcm)

	r := NewWAVReader(bytes.NewReader(wav))
	format, err := r.ReadFormat()
	require.NoError(t, err)
	assert.Equal(t, 24000, format.Rate)
	assert.Equal(t, 2, format.Width)
	assert.Equal(t, 1, format.Channels)

	data, err := io.ReadAll(r)
	require.NoError(t, err)
	assert.Equal(t, pcm, data)
}

func TestWAVReader_DifferentFormats(t *testing.T) {
	tests := []struct {
		name     string
		rate     int
		bits     int
		channels int
	}{
		{"44100Hz 16bit stereo", 44100, 16, 2},
		{"22050Hz 8bit mono", 22050, 8, 1},
		{"48000Hz 24bit mono", 48000, 24, 1},
		{"16000Hz 16bit mono", 16000, 16, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pcm := make([]byte, 100)
			for i := range pcm {
				pcm[i] = byte(i)
			}
			wav := buildWAV(tt.rate, tt.bits, tt.channels, pcm)

			r := NewWAVReader(bytes.NewReader(wav))
			format, err := r.ReadFormat()
			require.NoError(t, err)
			assert.Equal(t, tt.rate, format.Rate)
			assert.Equal(t, tt.bits/8, format.Width)
			assert.Equal(t, tt.channels, format.Channels)

			data, err := io.ReadAll(r)
			require.NoError(t, err)
			assert.Equal(t, pcm, data)
		})
	}
}

func TestWAVReader_HeaderSplitAcrossChunks(t *testing.T) {
	pcm := []byte{0xAA, 0xBB, 0xCC, 0xDD}
	wav := buildWAV(24000, 16, 1, pcm)

	slowReader := &oneByteReader{data: wav}

	r := NewWAVReader(slowReader)
	format, err := r.ReadFormat()
	require.NoError(t, err)
	assert.Equal(t, 24000, format.Rate)
	assert.Equal(t, 2, format.Width)
	assert.Equal(t, 1, format.Channels)

	data, err := io.ReadAll(r)
	require.NoError(t, err)
	assert.Equal(t, pcm, data)
}

func TestWAVReader_NonStandardExtraChunks(t *testing.T) {
	pcm := []byte{0x10, 0x20, 0x30, 0x40}
	extraChunks := []struct {
		ID   string
		Data []byte
	}{
		{"LIST", []byte("some list info data!")},
		{"JUNK", []byte("junk padding data!!")},
	}

	wav := buildWAVWithExtraChunks(24000, 16, 1, pcm, extraChunks)

	r := NewWAVReader(bytes.NewReader(wav))
	format, err := r.ReadFormat()
	require.NoError(t, err)
	assert.Equal(t, 24000, format.Rate)
	assert.Equal(t, 2, format.Width)
	assert.Equal(t, 1, format.Channels)

	data, err := io.ReadAll(r)
	require.NoError(t, err)
	assert.Equal(t, pcm, data)
}

func TestWAVReader_NotRIFF(t *testing.T) {
	data := []byte("NOT_RIFF_FILE_DATA_HERE")
	r := NewWAVReader(bytes.NewReader(data))
	_, err := r.ReadFormat()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a RIFF file")
}

func TestWAVReader_NotWAVE(t *testing.T) {
	buf := &bytes.Buffer{}
	buf.WriteString("RIFF")
	binary.Write(buf, binary.LittleEndian, uint32(100))
	buf.WriteString("AVI ")

	r := NewWAVReader(buf)
	_, err := r.ReadFormat()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a WAVE file")
}

func TestWAVReader_FmtChunkTooSmall(t *testing.T) {
	buf := &bytes.Buffer{}
	buf.WriteString("RIFF")
	binary.Write(buf, binary.LittleEndian, uint32(100))
	buf.WriteString("WAVE")
	buf.WriteString("fmt ")
	binary.Write(buf, binary.LittleEndian, uint32(10))
	buf.Write(make([]byte, 10))

	r := NewWAVReader(buf)
	_, err := r.ReadFormat()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fmt chunk too small")
}

func TestWAVReader_NonPCMFormat(t *testing.T) {
	buf := &bytes.Buffer{}
	buf.WriteString("RIFF")
	binary.Write(buf, binary.LittleEndian, uint32(100))
	buf.WriteString("WAVE")
	buf.WriteString("fmt ")
	binary.Write(buf, binary.LittleEndian, uint32(16))
	binary.Write(buf, binary.LittleEndian, uint16(3))
	binary.Write(buf, binary.LittleEndian, uint16(1))
	binary.Write(buf, binary.LittleEndian, uint32(44100))
	binary.Write(buf, binary.LittleEndian, uint32(44100*2))
	binary.Write(buf, binary.LittleEndian, uint16(2))
	binary.Write(buf, binary.LittleEndian, uint16(16))

	r := NewWAVReader(buf)
	_, err := r.ReadFormat()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported audio format")
}

func TestWAVReader_ReadBeforeReadFormat(t *testing.T) {
	pcm := []byte{0x01, 0x02}
	wav := buildWAV(24000, 16, 1, pcm)

	r := NewWAVReader(bytes.NewReader(wav))
	buf := make([]byte, 10)
	_, err := r.Read(buf)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ReadFormat must be called before Read")
}

func TestWAVReader_ReadFormatCalledTwice(t *testing.T) {
	pcm := []byte{0x01, 0x02}
	wav := buildWAV(24000, 16, 1, pcm)

	r := NewWAVReader(bytes.NewReader(wav))

	format1, err := r.ReadFormat()
	require.NoError(t, err)

	format2, err := r.ReadFormat()
	require.NoError(t, err)
	assert.Equal(t, format1, format2)
}

func TestWAVReader_TruncatedRIFFHeader(t *testing.T) {
	r := NewWAVReader(bytes.NewReader([]byte("RIF")))
	_, err := r.ReadFormat()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read RIFF header")
}

func TestWAVReader_EmptyPCMData(t *testing.T) {
	wav := buildWAV(24000, 16, 1, nil)

	r := NewWAVReader(bytes.NewReader(wav))
	format, err := r.ReadFormat()
	require.NoError(t, err)
	assert.Equal(t, 24000, format.Rate)

	data, err := io.ReadAll(r)
	require.NoError(t, err)
	assert.Empty(t, data)
}

func TestWAVReader_LargePCMData(t *testing.T) {
	pcm := make([]byte, 65536)
	for i := range pcm {
		pcm[i] = byte(i % 256)
	}
	wav := buildWAV(24000, 16, 1, pcm)

	r := NewWAVReader(bytes.NewReader(wav))
	format, err := r.ReadFormat()
	require.NoError(t, err)
	assert.Equal(t, 24000, format.Rate)

	data, err := io.ReadAll(r)
	require.NoError(t, err)
	assert.Equal(t, pcm, data)
}

func TestWAVReader_DataBeforeFmt(t *testing.T) {
	buf := &bytes.Buffer{}
	buf.WriteString("RIFF")
	binary.Write(buf, binary.LittleEndian, uint32(100))
	buf.WriteString("WAVE")
	buf.WriteString("data")
	binary.Write(buf, binary.LittleEndian, uint32(4))
	buf.Write([]byte{0x01, 0x02, 0x03, 0x04})

	r := NewWAVReader(buf)
	_, err := r.ReadFormat()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "data chunk found before fmt chunk")
}

type oneByteReader struct {
	data []byte
	pos  int
}

func (r *oneByteReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	p[0] = r.data[r.pos]
	r.pos++
	return 1, nil
}
