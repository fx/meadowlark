package segment

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	assert.Equal(t, 24, cfg.FirstSegmentChars)
	assert.Equal(t, 60, cfg.MinSegmentChars)
	assert.Equal(t, 400, cfg.MaxSegmentChars)
	require.NoError(t, cfg.Validate())
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr string
	}{
		{"defaults", DefaultConfig(), ""},
		{"all equal", Config{10, 10, 10}, ""},
		{"zero first", Config{0, 60, 400}, "must be positive"},
		{"zero min", Config{24, 0, 400}, "must be positive"},
		{"zero max", Config{24, 60, 0}, "must be positive"},
		{"negative first", Config{-1, 60, 400}, "must be positive"},
		{"first above min", Config{80, 60, 400}, "first <= min <= max"},
		{"min above max", Config{24, 600, 400}, "first <= min <= max"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

// New falls back to the defaults in full rather than running with thresholds
// its algorithm cannot honour.
func TestNewFallsBackOnIncoherentConfig(t *testing.T) {
	s := New(Config{FirstSegmentChars: 24, MinSegmentChars: 600, MaxSegmentChars: 400})
	assert.Equal(t, DefaultConfig(), s.cfg)

	ok := New(Config{FirstSegmentChars: 5, MinSegmentChars: 10, MaxSegmentChars: 20})
	assert.Equal(t, Config{5, 10, 20}, ok.cfg)
}

func TestAddEmptyStringIsNoOp(t *testing.T) {
	s := New(DefaultConfig())
	assert.Nil(t, s.Add(""))
	assert.Empty(t, s.Pending())
}

// Scenario: one chunk containing several sentences.
//
// The first period flushes on firstSegmentChars=24; the second is passed over
// at 49 runes because minSegmentChars=60; the third is not a boundary at all
// because it ends the buffer.
func TestOneChunkSeveralSentences(t *testing.T) {
	s := New(DefaultConfig())
	got := s.Add("The weather is sunny today and quite warm. " +
		"Tomorrow will bring rain across the whole region. Bring an umbrella.")
	assert.Equal(t, []string{"The weather is sunny today and quite warm."}, got)

	rest := s.Flush()
	assert.Equal(t, []string{"Tomorrow will bring rain across the whole region. Bring an umbrella."}, rest)
}

// Scenario: text that never reaches punctuation.
func TestForcedBreakHardCut(t *testing.T) {
	s := New(DefaultConfig())
	got := s.Add(strings.Repeat("a", 500))
	require.Len(t, got, 1)
	assert.Equal(t, 400, utf8.RuneCountInString(got[0]))
	assert.Equal(t, 100, utf8.RuneCountInString(s.Pending()))

	assert.Equal(t, []string{strings.Repeat("a", 100)}, s.Flush())
}

// Scenario: forced break prefers a soft break.
func TestForcedBreakPrefersSoftBreak(t *testing.T) {
	s := New(DefaultConfig())
	// Comma at rune index 380, space at 381, no sentence terminator anywhere.
	text := strings.Repeat("a", 380) + ", " + strings.Repeat("b", 118)
	require.Equal(t, 500, utf8.RuneCountInString(text))

	got := s.Add(text)
	require.Len(t, got, 1)
	assert.True(t, strings.HasSuffix(got[0], ","), "segment must end at the comma")
	assert.Equal(t, 381, utf8.RuneCountInString(got[0]))
}

// A forced break with whitespace but no soft break cuts at the last space.
func TestForcedBreakFallsBackToWhitespace(t *testing.T) {
	s := New(DefaultConfig())
	text := strings.Repeat("a", 390) + " " + strings.Repeat("b", 109)
	require.Equal(t, 500, utf8.RuneCountInString(text))

	got := s.Add(text)
	require.Len(t, got, 1)
	assert.Equal(t, strings.Repeat("a", 390), got[0])
}

// Scenario: multi-byte runes survive a hard cut.
func TestForcedBreakIsRuneAligned(t *testing.T) {
	s := New(DefaultConfig())
	got := s.Add(strings.Repeat("\U0001f600", 500))
	require.Len(t, got, 1)
	assert.True(t, utf8.ValidString(got[0]))
	assert.Equal(t, 400, utf8.RuneCountInString(got[0]))
}

// A forced cut whose consumed run is entirely whitespace emits nothing but
// still advances the buffer, so the loop terminates.
func TestForcedBreakOnAllWhitespace(t *testing.T) {
	s := New(Config{FirstSegmentChars: 2, MinSegmentChars: 4, MaxSegmentChars: 8})
	got := s.Add(strings.Repeat(" ", 20))
	assert.Empty(t, got)
	assert.Empty(t, s.Flush())
}

// Scenario: abbreviation does not split.
func TestAbbreviationDoesNotSplit(t *testing.T) {
	const text = "I asked Dr. Nakamura about the results of the second trial and she was optimistic."
	s := New(DefaultConfig())
	assert.Empty(t, s.Add(text))
	assert.Equal(t, []string{text}, s.Flush())
}

// Scenario: short sentence coalesces.
func TestShortSentenceCoalesces(t *testing.T) {
	s := New(DefaultConfig())
	assert.Empty(t, s.Add("Sure."))
	assert.Empty(t, s.Add(" The living room lights are now at forty percent brightness."))
	assert.Equal(t,
		[]string{"Sure. The living room lights are now at forty percent brightness."},
		s.Flush())
}

func TestBoundarySuppression(t *testing.T) {
	tests := []struct {
		name string
		text string
		want []string
	}{
		{"decimal point", "Pi is 3.14 which is a number worth remembering for a while yet. ", []string{"Pi is 3.14 which is a number worth remembering for a while yet."}},
		{"single-letter initial", "I met A. Smith yesterday afternoon at the station near the old bridge. ", []string{"I met A. Smith yesterday afternoon at the station near the old bridge."}},
		{"multi-part abbreviation", "Bring warm clothes, e.g. a coat, because the evenings there get very cold. ", []string{"Bring warm clothes, e.g. a coat, because the evenings there get very cold."}},
		{"uppercase abbreviation", "Please call MRS. Fielding before the end of the working day tomorrow now. ", []string{"Please call MRS. Fielding before the end of the working day tomorrow now."}},
		{"not an abbreviation", "The kitchen light is now on and the hallway light is off too. Good. ", []string{"The kitchen light is now on and the hallway light is off too.", "Good."}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := New(DefaultConfig())
			got := s.Add(tt.text)
			got = append(got, s.Flush()...)
			assert.Equal(t, tt.want, got)
		})
	}
}

// A period whose token is preceded by whitespace has an empty token and is not
// suppressed, and a period followed by a non-space is not a boundary at all.
func TestPeriodEdgeCases(t *testing.T) {
	s := New(DefaultConfig())
	// "example.com" — the period is followed by a letter, so no boundary.
	assert.Empty(t, s.Add("Visit example.com for the full documentation of every option."))
	assert.Len(t, s.Flush(), 1)

	s2 := New(Config{FirstSegmentChars: 1, MinSegmentChars: 1, MaxSegmentChars: 400})
	// A bare period preceded by whitespace: the token is empty, so nothing is
	// suppressed and the boundary stands.
	assert.Equal(t, []string{"a ."}, s2.Add("a . b"))
}

func TestBoundaryKinds(t *testing.T) {
	cfg := Config{FirstSegmentChars: 1, MinSegmentChars: 1, MaxSegmentChars: 400}
	tests := []struct {
		name string
		text string
		want []string
	}{
		{"period", "One. Two", []string{"One."}},
		{"exclamation", "One! Two", []string{"One!"}},
		{"question", "One? Two", []string{"One?"}},
		{"ellipsis", "One… Two", []string{"One…"}},
		{"newline", "One\nTwo", []string{"One"}},
		{"closing quote run", `He said "stop."  Then`, []string{`He said "stop."`}},
		{"closing bracket", "See (this.) Then", []string{"See (this.)"}},
		{"full width period", "これは。あれ", []string{"これは。"}},
		{"full width exclamation", "これは！あれ", []string{"これは！"}},
		{"full width question", "これは？あれ", []string{"これは？"}},
		{"full width with closing punct", "これは。»あれ", []string{"これは。»"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := New(cfg)
			assert.Equal(t, tt.want, s.Add(tt.text))
		})
	}
}

// An ASCII terminator at the very end of the buffer is not yet a boundary,
// because the next fragment may continue the token. A full-width terminator
// needs no such guard.
func TestTrailingTerminatorGuard(t *testing.T) {
	cfg := Config{FirstSegmentChars: 1, MinSegmentChars: 1, MaxSegmentChars: 400}

	ascii := New(cfg)
	assert.Empty(t, ascii.Add("Ready."))
	assert.Equal(t, []string{"Ready."}, ascii.Add(" Set"))

	full := New(cfg)
	assert.Equal(t, []string{"これは。"}, full.Add("これは。"))
}

// A boundary passed over for being too short is never reconsidered, because
// its candidate cannot grow.
func TestPassedOverBoundaryIsNotReconsidered(t *testing.T) {
	s := New(Config{FirstSegmentChars: 10, MinSegmentChars: 10, MaxSegmentChars: 400})
	assert.Empty(t, s.Add("Hi. "))
	assert.Empty(t, s.Add("Yes. "))
	assert.Equal(t, []string{"Hi. Yes. And then some more."}, s.Add("And then some more. "))
}

func TestFirstSegmentThresholdAppliesOnlyOnce(t *testing.T) {
	s := New(DefaultConfig())
	// 30 runes: above firstSegmentChars, below minSegmentChars.
	assert.Equal(t, []string{"Turning the hallway light on."},
		s.Add("Turning the hallway light on. "))
	// The same length again is now below the threshold and coalesces.
	assert.Empty(t, s.Add("Turning the hallway light on. "))
}

func TestFlushEmptyAndWhitespaceOnly(t *testing.T) {
	s := New(DefaultConfig())
	assert.Empty(t, s.Flush())

	s.Add("   \t\n  ")
	assert.Empty(t, s.Flush())
	assert.Empty(t, s.Pending())
}

func TestFlushClearsBuffer(t *testing.T) {
	s := New(DefaultConfig())
	s.Add("Some buffered text")
	assert.Equal(t, []string{"Some buffered text"}, s.Flush())
	assert.Empty(t, s.Pending())
	assert.Empty(t, s.Flush())
}

func TestReset(t *testing.T) {
	s := New(DefaultConfig())
	s.Add("Partial text that never completed")
	require.NotEmpty(t, s.Pending())

	s.Reset()
	assert.Empty(t, s.Pending())

	// The next segment is treated as a first segment again.
	assert.Equal(t, []string{"Turning the hallway light on."},
		s.Add("Turning the hallway light on. "))
}

func TestPending(t *testing.T) {
	s := New(DefaultConfig())
	s.Add("half a sen")
	assert.Equal(t, "half a sen", s.Pending())
	s.Add("tence")
	assert.Equal(t, "half a sentence", s.Pending())
}

// A boundary lying past MaxSegmentChars does not win over the forced break, so
// the buffer stays bounded.
func TestBoundaryBeyondLimitYieldsToForcedBreak(t *testing.T) {
	s := New(Config{FirstSegmentChars: 2, MinSegmentChars: 2, MaxSegmentChars: 20})
	got := s.Add(strings.Repeat("a", 30) + ". b")
	require.NotEmpty(t, got)
	assert.Equal(t, 20, utf8.RuneCountInString(got[0]))
}

// Every segment a session emits is non-empty and carries no leading or
// trailing whitespace.
func TestSegmentsAreTrimmed(t *testing.T) {
	s := New(DefaultConfig())
	got := s.Add("   The weather is sunny today and quite warm.   \n\n   ")
	got = append(got, s.Flush()...)
	require.Len(t, got, 1)
	assert.Equal(t, "The weather is sunny today and quite warm.", got[0])
}

// One Add can complete several segments, so it drains the buffer in a loop
// rather than emitting at most one per call. The unterminated tail stays
// buffered: it is not a boundary, because the next fragment may continue it.
func TestAddEmitsSeveralSegmentsFromOneCall(t *testing.T) {
	s := New(Config{FirstSegmentChars: 1, MinSegmentChars: 1, MaxSegmentChars: 400})

	got := s.Add("One. Two! Three? Four")
	assert.Equal(t, []string{"One.", "Two!", "Three?"}, got)

	// take trims only the segment it emits, so the separating space in front
	// of the tail is still buffered.
	assert.Equal(t, " Four", s.Pending())
	assert.Equal(t, []string{"Four"}, s.Flush())
}
