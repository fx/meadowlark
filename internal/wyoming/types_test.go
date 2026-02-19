package wyoming

import (
	"bufio"
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSynthesizeRoundTrip(t *testing.T) {
	s := &Synthesize{Text: "Hello world", Voice: "alloy", Speaker: "speaker1", Language: "en"}
	ev := s.ToEvent()
	assert.Equal(t, TypeSynthesize, ev.Type)
	got, err := SynthesizeFromEvent(ev)
	require.NoError(t, err)
	assert.Equal(t, s.Text, got.Text)
	assert.Equal(t, s.Voice, got.Voice)
	assert.Equal(t, s.Speaker, got.Speaker)
	assert.Equal(t, s.Language, got.Language)
}

func TestSynthesizeMinimalFields(t *testing.T) {
	s := &Synthesize{Text: "just text"}
	ev := s.ToEvent()
	got, err := SynthesizeFromEvent(ev)
	require.NoError(t, err)
	assert.Equal(t, "just text", got.Text)
	assert.Empty(t, got.Voice)
	assert.Empty(t, got.Speaker)
	assert.Empty(t, got.Language)
}

func TestSynthesizeFromEventWrongType(t *testing.T) {
	_, err := SynthesizeFromEvent(&Event{Type: TypePing})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected type")
}

func TestSynthesizeWireRoundTrip(t *testing.T) {
	s := &Synthesize{Text: "Hello", Voice: "nova", Language: "en"}
	var buf bytes.Buffer
	require.NoError(t, WriteEvent(&buf, s.ToEvent()))
	ev, err := ReadEvent(bufio.NewReader(&buf))
	require.NoError(t, err)
	got, err := SynthesizeFromEvent(ev)
	require.NoError(t, err)
	assert.Equal(t, "Hello", got.Text)
	assert.Equal(t, "nova", got.Voice)
	assert.Equal(t, "en", got.Language)
}

func TestAudioStartRoundTrip(t *testing.T) {
	a := &AudioStart{Rate: 24000, Width: 2, Channels: 1}
	ev := a.ToEvent()
	assert.Equal(t, TypeAudioStart, ev.Type)
	got, err := AudioStartFromEvent(ev)
	require.NoError(t, err)
	assert.Equal(t, 24000, got.Rate)
	assert.Equal(t, 2, got.Width)
	assert.Equal(t, 1, got.Channels)
}

func TestAudioStartFromEventWrongType(t *testing.T) {
	_, err := AudioStartFromEvent(&Event{Type: TypePing})
	require.Error(t, err)
}

func TestAudioStartWireRoundTrip(t *testing.T) {
	a := &AudioStart{Rate: 48000, Width: 4, Channels: 2}
	var buf bytes.Buffer
	require.NoError(t, WriteEvent(&buf, a.ToEvent()))
	ev, err := ReadEvent(bufio.NewReader(&buf))
	require.NoError(t, err)
	got, err := AudioStartFromEvent(ev)
	require.NoError(t, err)
	assert.Equal(t, 48000, got.Rate)
	assert.Equal(t, 4, got.Width)
	assert.Equal(t, 2, got.Channels)
}

func TestAudioChunkRoundTrip(t *testing.T) {
	audio := []byte{0x01, 0x02, 0x03, 0x04, 0x05}
	a := &AudioChunk{Rate: 22050, Width: 2, Channels: 1, Audio: audio}
	ev := a.ToEvent()
	assert.Equal(t, TypeAudioChunk, ev.Type)
	assert.Equal(t, audio, ev.Payload)
	got, err := AudioChunkFromEvent(ev)
	require.NoError(t, err)
	assert.Equal(t, 22050, got.Rate)
	assert.Equal(t, 2, got.Width)
	assert.Equal(t, 1, got.Channels)
	assert.Equal(t, audio, got.Audio)
}

func TestAudioChunkFromEventWrongType(t *testing.T) {
	_, err := AudioChunkFromEvent(&Event{Type: TypePing})
	require.Error(t, err)
}

func TestAudioChunkWireRoundTrip(t *testing.T) {
	audio := bytes.Repeat([]byte{0xDE, 0xAD}, 1024)
	a := &AudioChunk{Rate: 24000, Width: 2, Channels: 1, Audio: audio}
	var buf bytes.Buffer
	require.NoError(t, WriteEvent(&buf, a.ToEvent()))
	ev, err := ReadEvent(bufio.NewReader(&buf))
	require.NoError(t, err)
	got, err := AudioChunkFromEvent(ev)
	require.NoError(t, err)
	assert.Equal(t, 24000, got.Rate)
	assert.Equal(t, audio, got.Audio)
}

func TestAudioStopToEvent(t *testing.T) {
	ev := (&AudioStop{}).ToEvent()
	assert.Equal(t, TypeAudioStop, ev.Type)
	assert.Empty(t, ev.Data)
	assert.Nil(t, ev.Payload)
}

func TestAudioStopWireRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, WriteEvent(&buf, (&AudioStop{}).ToEvent()))
	ev, err := ReadEvent(bufio.NewReader(&buf))
	require.NoError(t, err)
	assert.Equal(t, TypeAudioStop, ev.Type)
}

func TestDescribeToEvent(t *testing.T) {
	ev := (&Describe{}).ToEvent()
	assert.Equal(t, TypeDescribe, ev.Type)
	assert.Empty(t, ev.Data)
}

func TestDescribeWireRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, WriteEvent(&buf, (&Describe{}).ToEvent()))
	ev, err := ReadEvent(bufio.NewReader(&buf))
	require.NoError(t, err)
	assert.Equal(t, TypeDescribe, ev.Type)
}

func TestInfoRoundTrip(t *testing.T) {
	info := &Info{
		Tts: []TtsProgram{{
			Name: "meadowlark", Description: "Meadowlark TTS Bridge",
			Installed: true, Version: "0.1.0",
			Voices: []TtsVoice{
				{Name: "alloy (OpenAI, tts-1)", Description: "alloy (OpenAI, tts-1)", Installed: true, Languages: []string{"en"}},
				{Name: "custom-alias", Description: "Custom Alias", Installed: true, Languages: []string{"en", "fr"}, Speakers: []TtsVoiceSpeaker{{Name: "speaker1"}}},
			},
		}},
	}
	ev := info.ToEvent()
	assert.Equal(t, TypeInfo, ev.Type)
	got, err := InfoFromEvent(ev)
	require.NoError(t, err)
	require.Len(t, got.Tts, 1)
	assert.Equal(t, "meadowlark", got.Tts[0].Name)
	assert.True(t, got.Tts[0].Installed)
	assert.Equal(t, "0.1.0", got.Tts[0].Version)
	require.Len(t, got.Tts[0].Voices, 2)
	assert.Equal(t, "alloy (OpenAI, tts-1)", got.Tts[0].Voices[0].Name)
	assert.Equal(t, []string{"en"}, got.Tts[0].Voices[0].Languages)
	assert.Empty(t, got.Tts[0].Voices[0].Speakers)
	assert.Equal(t, []string{"en", "fr"}, got.Tts[0].Voices[1].Languages)
	require.Len(t, got.Tts[0].Voices[1].Speakers, 1)
	assert.Equal(t, "speaker1", got.Tts[0].Voices[1].Speakers[0].Name)
}

func TestInfoFromEventWrongType(t *testing.T) {
	_, err := InfoFromEvent(&Event{Type: TypePing})
	require.Error(t, err)
}

func TestInfoFromEventNoTts(t *testing.T) {
	info, err := InfoFromEvent(&Event{Type: TypeInfo, Data: map[string]any{}})
	require.NoError(t, err)
	assert.Empty(t, info.Tts)
}

func TestInfoWireRoundTrip(t *testing.T) {
	info := &Info{Tts: []TtsProgram{{Name: "meadowlark", Installed: true, Version: "1.0.0", Voices: []TtsVoice{{Name: "alloy", Installed: true, Languages: []string{"en"}}}}}}
	var buf bytes.Buffer
	require.NoError(t, WriteEvent(&buf, info.ToEvent()))
	ev, err := ReadEvent(bufio.NewReader(&buf))
	require.NoError(t, err)
	got, err := InfoFromEvent(ev)
	require.NoError(t, err)
	require.Len(t, got.Tts, 1)
	assert.Equal(t, "meadowlark", got.Tts[0].Name)
	require.Len(t, got.Tts[0].Voices, 1)
	assert.Equal(t, "alloy", got.Tts[0].Voices[0].Name)
}

func TestPingToEvent(t *testing.T) {
	ev := (&Ping{}).ToEvent()
	assert.Equal(t, TypePing, ev.Type)
	assert.Empty(t, ev.Data)
}

func TestPongToEvent(t *testing.T) {
	ev := (&Pong{}).ToEvent()
	assert.Equal(t, TypePong, ev.Type)
	assert.Empty(t, ev.Data)
}

func TestPingPongWireRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, WriteEvent(&buf, (&Ping{}).ToEvent()))
	require.NoError(t, WriteEvent(&buf, (&Pong{}).ToEvent()))
	reader := bufio.NewReader(&buf)
	ev1, err := ReadEvent(reader)
	require.NoError(t, err)
	assert.Equal(t, TypePing, ev1.Type)
	ev2, err := ReadEvent(reader)
	require.NoError(t, err)
	assert.Equal(t, TypePong, ev2.Type)
}

func TestErrorRoundTrip(t *testing.T) {
	e := &Error{Text: "something went wrong", Code: "tts_error"}
	ev := e.ToEvent()
	assert.Equal(t, TypeError, ev.Type)
	got, err := ErrorFromEvent(ev)
	require.NoError(t, err)
	assert.Equal(t, "something went wrong", got.Text)
	assert.Equal(t, "tts_error", got.Code)
}

func TestErrorNoCode(t *testing.T) {
	e := &Error{Text: "error without code"}
	got, err := ErrorFromEvent(e.ToEvent())
	require.NoError(t, err)
	assert.Equal(t, "error without code", got.Text)
	assert.Empty(t, got.Code)
}

func TestErrorFromEventWrongType(t *testing.T) {
	_, err := ErrorFromEvent(&Event{Type: TypePing})
	require.Error(t, err)
}

func TestErrorWireRoundTrip(t *testing.T) {
	e := &Error{Text: "voice not found", Code: "not_found"}
	var buf bytes.Buffer
	require.NoError(t, WriteEvent(&buf, e.ToEvent()))
	ev, err := ReadEvent(bufio.NewReader(&buf))
	require.NoError(t, err)
	got, err := ErrorFromEvent(ev)
	require.NoError(t, err)
	assert.Equal(t, "voice not found", got.Text)
	assert.Equal(t, "not_found", got.Code)
}

func TestIntFromAnyVariousTypes(t *testing.T) {
	assert.Equal(t, 42, intFromAny(42))
	assert.Equal(t, 42, intFromAny(float64(42)))
	assert.Equal(t, 42, intFromAny(int64(42)))
	assert.Equal(t, 0, intFromAny("not a number"))
	assert.Equal(t, 0, intFromAny(nil))
}

func TestStringFromAny(t *testing.T) {
	assert.Equal(t, "hello", stringFromAny("hello"))
	assert.Equal(t, "", stringFromAny(42))
	assert.Equal(t, "", stringFromAny(nil))
}

func TestBoolFromAny(t *testing.T) {
	assert.True(t, boolFromAny(true))
	assert.False(t, boolFromAny(false))
	assert.False(t, boolFromAny("true"))
	assert.False(t, boolFromAny(nil))
}

func TestStringsFromAny(t *testing.T) {
	assert.Equal(t, []string{"a", "b", "c"}, stringsFromAny([]any{"a", "b", "c"}))
	assert.Equal(t, []string{"a", "c"}, stringsFromAny([]any{"a", 42, "c"}))
	assert.Nil(t, stringsFromAny("not a slice"))
	assert.Nil(t, stringsFromAny(nil))
}

func TestToAnySlice(t *testing.T) {
	assert.Equal(t, []any{"en", "fr"}, toAnySlice([]string{"en", "fr"}))
	assert.Equal(t, []any{}, toAnySlice(nil))
}

func TestInfoFromEventMalformedData(t *testing.T) {
	info, err := InfoFromEvent(&Event{Type: TypeInfo, Data: map[string]any{"tts": "not a slice"}})
	require.NoError(t, err)
	assert.Empty(t, info.Tts)

	info, err = InfoFromEvent(&Event{Type: TypeInfo, Data: map[string]any{"tts": []any{"not a map"}}})
	require.NoError(t, err)
	assert.Empty(t, info.Tts)

	info, err = InfoFromEvent(&Event{Type: TypeInfo, Data: map[string]any{"tts": []any{map[string]any{"name": "test", "voices": []any{"not a map"}}}}})
	require.NoError(t, err)
	require.Len(t, info.Tts, 1)
	assert.Empty(t, info.Tts[0].Voices)

	info, err = InfoFromEvent(&Event{Type: TypeInfo, Data: map[string]any{"tts": []any{map[string]any{"name": "test", "voices": []any{map[string]any{"name": "v", "speakers": []any{"not a map"}}}}}}})
	require.NoError(t, err)
	require.Len(t, info.Tts[0].Voices, 1)
	assert.Empty(t, info.Tts[0].Voices[0].Speakers)
}

func TestSynthesizeFromEventMissingFields(t *testing.T) {
	got, err := SynthesizeFromEvent(&Event{Type: TypeSynthesize, Data: map[string]any{}})
	require.NoError(t, err)
	assert.Empty(t, got.Text)
	assert.Empty(t, got.Voice)
	assert.Empty(t, got.Speaker)
	assert.Empty(t, got.Language)
}

func TestSynthesizeFromEventBadVoiceType(t *testing.T) {
	got, err := SynthesizeFromEvent(&Event{Type: TypeSynthesize, Data: map[string]any{"text": "hello", "voice": "not-a-map"}})
	require.NoError(t, err)
	assert.Equal(t, "hello", got.Text)
	assert.Empty(t, got.Voice)
}
