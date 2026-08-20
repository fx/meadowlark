package wyoming

import "fmt"

// Wyoming event type constants.
const (
	TypeDescribe          = "describe"
	TypeInfo              = "info"
	TypeSynthesize        = "synthesize"
	TypeSynthesizeStart   = "synthesize-start"
	TypeSynthesizeChunk   = "synthesize-chunk"
	TypeSynthesizeStop    = "synthesize-stop"
	TypeSynthesizeStopped = "synthesize-stopped"
	TypeAudioStart        = "audio-start"
	TypeAudioChunk        = "audio-chunk"
	TypeAudioStop         = "audio-stop"
	TypePing              = "ping"
	TypePong              = "pong"
	TypeError             = "error"
)

// Synthesize represents a TTS synthesis request.
type Synthesize struct {
	Text     string
	Voice    string
	Speaker  string
	Language string
}

// ToEvent converts a Synthesize to a generic Event.
func (s *Synthesize) ToEvent() *Event {
	data := map[string]any{
		"text": s.Text,
	}
	if s.Voice != "" {
		data["voice"] = map[string]any{"name": s.Voice}
	}
	if s.Speaker != "" {
		data["speaker"] = s.Speaker
	}
	if s.Language != "" {
		data["language"] = s.Language
	}
	return &Event{Type: TypeSynthesize, Data: data}
}

// SynthesizeFromEvent extracts a Synthesize from a generic Event.
func SynthesizeFromEvent(ev *Event) (*Synthesize, error) {
	if ev.Type != TypeSynthesize {
		return nil, fmt.Errorf("expected type %q, got %q", TypeSynthesize, ev.Type)
	}
	s := &Synthesize{}
	if text, ok := ev.Data["text"].(string); ok {
		s.Text = text
	}
	if voice, ok := ev.Data["voice"].(map[string]any); ok {
		if name, ok := voice["name"].(string); ok {
			s.Voice = name
		}
	}
	if speaker, ok := ev.Data["speaker"].(string); ok {
		s.Speaker = speaker
	}
	if language, ok := ev.Data["language"].(string); ok {
		s.Language = language
	}
	return s, nil
}

// SynthesizeStart opens a streaming synthesis session. The client follows it
// with zero or more SynthesizeChunk events and terminates it with
// SynthesizeStop.
//
// Its voice encoding is deliberately NOT the same as Synthesize's: this event
// nests name, language and speaker all under "voice", following the upstream
// wyoming/tts.py shape, whereas Synthesize nests only "name" and emits speaker
// and language at the top level. The two must not be harmonised — Synthesize's
// shape is what existing Wyoming clients already speak.
type SynthesizeStart struct {
	Voice      string
	Language   string
	Speaker    string
	TextFormat string // "text" (default) or "ssml"
	Context    any    // opaque, round-tripped unchanged
}

// ToEvent converts a SynthesizeStart to a generic Event.
//
// The "voice" object is omitted only when Voice, Language and Speaker are all
// empty; a start event carrying only a language or only a speaker is valid and
// must survive the round trip. Within the object each field is omitted when
// empty.
func (s *SynthesizeStart) ToEvent() *Event {
	data := map[string]any{}
	if s.Voice != "" || s.Language != "" || s.Speaker != "" {
		v := map[string]any{}
		if s.Voice != "" {
			v["name"] = s.Voice
		}
		if s.Language != "" {
			v["language"] = s.Language
		}
		if s.Speaker != "" {
			v["speaker"] = s.Speaker
		}
		data["voice"] = v
	}
	if s.TextFormat != "" {
		data["text_format"] = s.TextFormat
	}
	if s.Context != nil {
		data["context"] = s.Context
	}
	return &Event{Type: TypeSynthesizeStart, Data: data}
}

// SynthesizeStartFromEvent extracts a SynthesizeStart from a generic Event.
func SynthesizeStartFromEvent(ev *Event) (*SynthesizeStart, error) {
	if ev.Type != TypeSynthesizeStart {
		return nil, fmt.Errorf("expected type %q, got %q", TypeSynthesizeStart, ev.Type)
	}
	s := &SynthesizeStart{}
	if v, ok := ev.Data["voice"].(map[string]any); ok {
		s.Voice = stringFromAny(v["name"])
		s.Language = stringFromAny(v["language"])
		s.Speaker = stringFromAny(v["speaker"])
	}
	s.TextFormat = stringFromAny(ev.Data["text_format"])
	if c, ok := ev.Data["context"]; ok {
		s.Context = c
	}
	return s, nil
}

// SynthesizeChunk carries a fragment of text into an open streaming session.
type SynthesizeChunk struct {
	Text string
}

// ToEvent converts a SynthesizeChunk to a generic Event.
func (s *SynthesizeChunk) ToEvent() *Event {
	return &Event{
		Type: TypeSynthesizeChunk,
		Data: map[string]any{"text": s.Text},
	}
}

// SynthesizeChunkFromEvent extracts a SynthesizeChunk from a generic Event.
func SynthesizeChunkFromEvent(ev *Event) (*SynthesizeChunk, error) {
	if ev.Type != TypeSynthesizeChunk {
		return nil, fmt.Errorf("expected type %q, got %q", TypeSynthesizeChunk, ev.Type)
	}
	return &SynthesizeChunk{Text: stringFromAny(ev.Data["text"])}, nil
}

// SynthesizeStop signals that the client will send no further text.
type SynthesizeStop struct{}

// ToEvent converts a SynthesizeStop to a generic Event.
func (s *SynthesizeStop) ToEvent() *Event {
	return &Event{Type: TypeSynthesizeStop}
}

// SynthesizeStopFromEvent extracts a SynthesizeStop from a generic Event.
func SynthesizeStopFromEvent(ev *Event) (*SynthesizeStop, error) {
	if ev.Type != TypeSynthesizeStop {
		return nil, fmt.Errorf("expected type %q, got %q", TypeSynthesizeStop, ev.Type)
	}
	return &SynthesizeStop{}, nil
}

// SynthesizeStopped terminates a completed streaming synthesis session. It is
// the only one of the four streaming events sent server to client.
type SynthesizeStopped struct{}

// ToEvent converts a SynthesizeStopped to a generic Event.
func (s *SynthesizeStopped) ToEvent() *Event {
	return &Event{Type: TypeSynthesizeStopped}
}

// SynthesizeStoppedFromEvent extracts a SynthesizeStopped from a generic Event.
func SynthesizeStoppedFromEvent(ev *Event) (*SynthesizeStopped, error) {
	if ev.Type != TypeSynthesizeStopped {
		return nil, fmt.Errorf("expected type %q, got %q", TypeSynthesizeStopped, ev.Type)
	}
	return &SynthesizeStopped{}, nil
}

// AudioStart signals the beginning of audio output.
type AudioStart struct {
	Rate     int
	Width    int
	Channels int
}

// ToEvent converts an AudioStart to a generic Event.
func (a *AudioStart) ToEvent() *Event {
	return &Event{
		Type: TypeAudioStart,
		Data: map[string]any{
			"rate":     a.Rate,
			"width":    a.Width,
			"channels": a.Channels,
		},
	}
}

// AudioStartFromEvent extracts an AudioStart from a generic Event.
func AudioStartFromEvent(ev *Event) (*AudioStart, error) {
	if ev.Type != TypeAudioStart {
		return nil, fmt.Errorf("expected type %q, got %q", TypeAudioStart, ev.Type)
	}
	return &AudioStart{
		Rate:     intFromAny(ev.Data["rate"]),
		Width:    intFromAny(ev.Data["width"]),
		Channels: intFromAny(ev.Data["channels"]),
	}, nil
}

// AudioChunk carries a chunk of raw PCM audio data.
type AudioChunk struct {
	Rate     int
	Width    int
	Channels int
	Audio    []byte
}

// ToEvent converts an AudioChunk to a generic Event.
func (a *AudioChunk) ToEvent() *Event {
	return &Event{
		Type: TypeAudioChunk,
		Data: map[string]any{
			"rate":     a.Rate,
			"width":    a.Width,
			"channels": a.Channels,
		},
		Payload: a.Audio,
	}
}

// AudioChunkFromEvent extracts an AudioChunk from a generic Event.
func AudioChunkFromEvent(ev *Event) (*AudioChunk, error) {
	if ev.Type != TypeAudioChunk {
		return nil, fmt.Errorf("expected type %q, got %q", TypeAudioChunk, ev.Type)
	}
	return &AudioChunk{
		Rate:     intFromAny(ev.Data["rate"]),
		Width:    intFromAny(ev.Data["width"]),
		Channels: intFromAny(ev.Data["channels"]),
		Audio:    ev.Payload,
	}, nil
}

// AudioStop signals the end of audio output.
type AudioStop struct{}

// ToEvent converts an AudioStop to a generic Event.
func (a *AudioStop) ToEvent() *Event {
	return &Event{Type: TypeAudioStop}
}

// Describe requests service capabilities.
type Describe struct{}

// ToEvent converts a Describe to a generic Event.
func (d *Describe) ToEvent() *Event {
	return &Event{Type: TypeDescribe}
}

// TtsVoiceSpeaker represents a speaker within a voice.
type TtsVoiceSpeaker struct {
	Name string `json:"name"`
}

// TtsVoice represents a voice in the info response.
type TtsVoice struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Installed   bool              `json:"installed"`
	Languages   []string          `json:"languages"`
	Speakers    []TtsVoiceSpeaker `json:"speakers"`
}

// TtsProgram represents a TTS program in the info response.
type TtsProgram struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Installed   bool       `json:"installed"`
	Version     string     `json:"version"`
	Voices      []TtsVoice `json:"voices"`

	// SupportsSynthesizeStreaming reports whether the service accepts the
	// synthesize-start / synthesize-chunk / synthesize-stop streaming input
	// protocol. Home Assistant reads this flag to choose between its
	// buffering and its streaming TTS path.
	SupportsSynthesizeStreaming bool `json:"supports_synthesize_streaming"`
}

// Info represents the service capabilities response.
type Info struct {
	Tts []TtsProgram
}

// ToEvent converts an Info to a generic Event.
func (i *Info) ToEvent() *Event {
	ttsSlice := make([]any, len(i.Tts))
	for idx, prog := range i.Tts {
		voices := make([]any, len(prog.Voices))
		for vi, v := range prog.Voices {
			var speakers any
			if len(v.Speakers) > 0 {
				s := make([]any, len(v.Speakers))
				for si, sp := range v.Speakers {
					s[si] = map[string]any{"name": sp.Name}
				}
				speakers = s
			}
			vm := map[string]any{
				"name":        v.Name,
				"description": v.Description,
				"installed":   v.Installed,
				"languages":   toAnySlice(v.Languages),
				"attribution": map[string]any{"name": "", "url": ""},
			}
			if speakers != nil {
				vm["speakers"] = speakers
			}
			voices[vi] = vm
		}
		ttsSlice[idx] = map[string]any{
			"name":                          prog.Name,
			"description":                   prog.Description,
			"installed":                     prog.Installed,
			"version":                       prog.Version,
			"voices":                        voices,
			"attribution":                   map[string]any{"name": "", "url": ""},
			"supports_synthesize_streaming": prog.SupportsSynthesizeStreaming,
		}
	}
	return &Event{
		Type: TypeInfo,
		Data: map[string]any{"tts": ttsSlice},
	}
}

// InfoFromEvent extracts an Info from a generic Event.
func InfoFromEvent(ev *Event) (*Info, error) {
	if ev.Type != TypeInfo {
		return nil, fmt.Errorf("expected type %q, got %q", TypeInfo, ev.Type)
	}
	info := &Info{}
	ttsRaw, ok := ev.Data["tts"].([]any)
	if !ok {
		return info, nil
	}
	for _, progRaw := range ttsRaw {
		prog, ok := progRaw.(map[string]any)
		if !ok {
			continue
		}
		tp := TtsProgram{
			Name:                        stringFromAny(prog["name"]),
			Description:                 stringFromAny(prog["description"]),
			Installed:                   boolFromAny(prog["installed"]),
			Version:                     stringFromAny(prog["version"]),
			SupportsSynthesizeStreaming: boolFromAny(prog["supports_synthesize_streaming"]),
		}
		if voicesRaw, ok := prog["voices"].([]any); ok {
			for _, vRaw := range voicesRaw {
				vm, ok := vRaw.(map[string]any)
				if !ok {
					continue
				}
				tv := TtsVoice{
					Name:        stringFromAny(vm["name"]),
					Description: stringFromAny(vm["description"]),
					Installed:   boolFromAny(vm["installed"]),
					Languages:   stringsFromAny(vm["languages"]),
				}
				if speakersRaw, ok := vm["speakers"].([]any); ok {
					for _, sRaw := range speakersRaw {
						sm, ok := sRaw.(map[string]any)
						if !ok {
							continue
						}
						tv.Speakers = append(tv.Speakers, TtsVoiceSpeaker{
							Name: stringFromAny(sm["name"]),
						})
					}
				}
				tp.Voices = append(tp.Voices, tv)
			}
		}
		info.Tts = append(info.Tts, tp)
	}
	return info, nil
}

// Ping is a health check request.
type Ping struct{}

// ToEvent converts a Ping to a generic Event.
func (p *Ping) ToEvent() *Event {
	return &Event{Type: TypePing}
}

// Pong is a health check response.
type Pong struct{}

// ToEvent converts a Pong to a generic Event.
func (p *Pong) ToEvent() *Event {
	return &Event{Type: TypePong}
}

// Error represents a protocol error.
type Error struct {
	Text string
	Code string
}

// ToEvent converts an Error to a generic Event.
func (e *Error) ToEvent() *Event {
	data := map[string]any{
		"text": e.Text,
	}
	if e.Code != "" {
		data["code"] = e.Code
	}
	return &Event{Type: TypeError, Data: data}
}

// ErrorFromEvent extracts an Error from a generic Event.
func ErrorFromEvent(ev *Event) (*Error, error) {
	if ev.Type != TypeError {
		return nil, fmt.Errorf("expected type %q, got %q", TypeError, ev.Type)
	}
	return &Error{
		Text: stringFromAny(ev.Data["text"]),
		Code: stringFromAny(ev.Data["code"]),
	}, nil
}

// Helper functions for extracting typed values from map[string]any.

func intFromAny(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case float64:
		return int(n)
	case int64:
		return int(n)
	default:
		return 0
	}
}

func stringFromAny(v any) string {
	s, _ := v.(string)
	return s
}

func boolFromAny(v any) bool {
	b, _ := v.(bool)
	return b
}

func stringsFromAny(v any) []string {
	raw, ok := v.([]any)
	if !ok {
		return nil
	}
	result := make([]string, 0, len(raw))
	for _, item := range raw {
		if s, ok := item.(string); ok {
			result = append(result, s)
		}
	}
	return result
}

func toAnySlice(ss []string) []any {
	result := make([]any, len(ss))
	for i, s := range ss {
		result[i] = s
	}
	return result
}
