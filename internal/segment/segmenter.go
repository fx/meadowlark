// Package segment aggregates incrementally arriving text into sentence-sized
// segments suitable for one text-to-speech request each.
//
// It is pure: no I/O, no logging, no dependency beyond the standard library.
// Callers feed it fragments with Add, which returns the segments that have
// become complete, and drain the remainder with Flush when no further text is
// coming.
package segment

import (
	"fmt"
	"strings"
	"unicode"
)

// Default segmentation thresholds, measured in runes.
//
// minSegmentChars is roughly ten words, about four seconds of speech, which is
// comfortably longer than an upstream time-to-first-byte — so the next
// segment's request completes while the current one is still playing.
// firstSegmentChars is lower because the opening segment alone determines
// perceived latency. maxSegmentChars bounds both latency and buffer growth for
// pathological run-on text.
const (
	DefaultFirstSegmentChars = 24
	DefaultMinSegmentChars   = 60
	DefaultMaxSegmentChars   = 400
)

// Config holds the segmentation thresholds, all measured in runes.
type Config struct {
	// FirstSegmentChars is the minimum length of the session's first segment.
	FirstSegmentChars int
	// MinSegmentChars is the minimum length of every later segment.
	MinSegmentChars int
	// MaxSegmentChars is the buffer length at which a break is forced even
	// though no boundary has been found.
	MaxSegmentChars int
}

// DefaultConfig returns the default thresholds.
func DefaultConfig() Config {
	return Config{
		FirstSegmentChars: DefaultFirstSegmentChars,
		MinSegmentChars:   DefaultMinSegmentChars,
		MaxSegmentChars:   DefaultMaxSegmentChars,
	}
}

// Validate reports whether the thresholds are coherent. They must satisfy
// 0 < FirstSegmentChars <= MinSegmentChars <= MaxSegmentChars; an incoherent
// mix is rejected as a whole rather than partially honoured.
func (c Config) Validate() error {
	if c.FirstSegmentChars <= 0 || c.MinSegmentChars <= 0 || c.MaxSegmentChars <= 0 {
		return fmt.Errorf("segment: thresholds must be positive, got first=%d min=%d max=%d",
			c.FirstSegmentChars, c.MinSegmentChars, c.MaxSegmentChars)
	}
	if c.FirstSegmentChars > c.MinSegmentChars || c.MinSegmentChars > c.MaxSegmentChars {
		return fmt.Errorf("segment: thresholds must satisfy first <= min <= max, got first=%d min=%d max=%d",
			c.FirstSegmentChars, c.MinSegmentChars, c.MaxSegmentChars)
	}
	return nil
}

// abbreviations are tokens after which a period does not end a sentence.
// Matching is case-insensitive. Multi-part entries such as "e.g" are matched
// directly because a token is the whole run of non-whitespace before the final
// period, dots included.
var abbreviations = map[string]struct{}{
	"mr": {}, "mrs": {}, "ms": {}, "dr": {}, "prof": {},
	"sr": {}, "jr": {}, "st": {}, "vs": {}, "etc": {},
	"approx": {}, "no": {}, "fig": {}, "inc": {}, "ltd": {},
	"co": {}, "e.g": {}, "i.e": {},
}

// Segmenter accumulates text and emits completed segments. It is not safe for
// concurrent use; a caller owning one session owns its segmenter.
type Segmenter struct {
	cfg Config

	// buf holds the text not yet emitted, as runes so every threshold and cut
	// is rune-aligned.
	buf []rune
	// scanned is the index past which boundaries have already been considered
	// and passed over for being too short. A passed-over boundary can never
	// qualify later: its candidate is buf[:end], which does not grow.
	scanned int
	// first reports whether the next emitted segment is the session's first,
	// which uses the lower FirstSegmentChars threshold.
	first bool
}

// New creates a Segmenter using cfg. An incoherent cfg — one that fails
// Validate — falls back to DefaultConfig in full, so the segmenter always runs
// with thresholds its algorithm can honour. Callers that need to report the
// misconfiguration should call Validate themselves first.
func New(cfg Config) *Segmenter {
	if cfg.Validate() != nil {
		cfg = DefaultConfig()
	}
	return &Segmenter{cfg: cfg, first: true}
}

// Reset discards any buffered text and returns the segmenter to its initial
// state, so the next segment emitted is again treated as a first segment.
func (s *Segmenter) Reset() {
	s.buf = s.buf[:0]
	s.scanned = 0
	s.first = true
}

// Pending returns the text buffered but not yet emitted.
func (s *Segmenter) Pending() string {
	return string(s.buf)
}

// Add appends text and returns every segment that became complete, in order.
// It returns nil when the text merely accumulates.
func (s *Segmenter) Add(text string) []string {
	if text == "" {
		return nil
	}
	s.buf = append(s.buf, []rune(text)...)

	var out []string
	for {
		seg, ok := s.next()
		if !ok {
			return out
		}
		if seg != "" {
			out = append(out, seg)
		}
	}
}

// Flush emits the remainder regardless of length and empties the buffer. A
// remainder that is empty or entirely whitespace emits nothing.
func (s *Segmenter) Flush() []string {
	rest := strings.TrimSpace(string(s.buf))
	var out []string
	s.buf = s.buf[:0]
	s.scanned = 0
	if rest != "" {
		s.first = false
		out = append(out, rest)
	}
	return out
}

// next emits at most one segment from the buffer. It reports false when the
// buffer holds nothing emittable yet. The emitted string may be empty when the
// consumed run was entirely whitespace; the caller drops those.
func (s *Segmenter) next() (string, bool) {
	forced := len(s.buf) >= s.cfg.MaxSegmentChars

	end, ok := s.findBoundary()
	if !ok || (forced && end > s.cfg.MaxSegmentChars) {
		if !forced {
			return "", false
		}
		return s.take(s.forcedCut()), true
	}

	candidate := strings.TrimSpace(string(s.buf[:end]))
	if len([]rune(candidate)) < s.threshold() {
		// Pass the boundary over and keep accumulating.
		s.scanned = end
		return "", true
	}
	return s.take(end), true
}

// threshold returns the minimum candidate length for the next segment.
func (s *Segmenter) threshold() int {
	if s.first {
		return s.cfg.FirstSegmentChars
	}
	return s.cfg.MinSegmentChars
}

// take consumes cut runes from the front of the buffer and returns them
// trimmed. cut must be at least 1 so the buffer always shrinks.
func (s *Segmenter) take(cut int) string {
	seg := strings.TrimSpace(string(s.buf[:cut]))
	s.buf = append(s.buf[:0], s.buf[cut:]...)
	s.scanned = 0
	if seg != "" {
		s.first = false
	}
	return seg
}

// findBoundary returns the index just past the first segment boundary at or
// after s.scanned, or false when the buffer holds none.
//
// A boundary is a sentence-terminating rune plus any run of closing
// punctuation, or a newline. ASCII terminators additionally require trailing
// whitespace: a period at the very end of the buffer is not yet a boundary,
// because the next fragment may continue the token. Full-width terminators
// need no such guard — CJK text is not space-separated and they never appear
// inside a token.
func (s *Segmenter) findBoundary() (int, bool) {
	for i := s.scanned; i < len(s.buf); i++ {
		r := s.buf[i]

		if r == '\n' {
			return i + 1, true
		}

		switch {
		case isFullWidthTerminator(r):
			return s.skipClosing(i + 1), true
		case isWhitespaceGuardedTerminator(r):
			if r == '.' && s.suppressed(i) {
				continue
			}
			end := s.skipClosing(i + 1)
			if end < len(s.buf) && unicode.IsSpace(s.buf[end]) {
				return end, true
			}
		}
	}
	return 0, false
}

// skipClosing advances past a run of closing punctuation starting at i.
func (s *Segmenter) skipClosing(i int) int {
	for i < len(s.buf) && isClosingPunct(s.buf[i]) {
		i++
	}
	return i
}

// suppressed reports whether the period at index i is prevented from ending a
// sentence: a decimal point between digits, a known abbreviation, or a single
// letter used as an initial.
func (s *Segmenter) suppressed(i int) bool {
	if i > 0 && i+1 < len(s.buf) && unicode.IsDigit(s.buf[i-1]) && unicode.IsDigit(s.buf[i+1]) {
		return true
	}

	// The token is the maximal run of non-whitespace immediately preceding the
	// period, the period itself excluded.
	start := i
	for start > 0 && !unicode.IsSpace(s.buf[start-1]) {
		start--
	}
	token := s.buf[start:i]
	if len(token) == 0 {
		return false
	}
	if len(token) == 1 && unicode.IsLetter(token[0]) {
		return true
	}
	_, ok := abbreviations[strings.ToLower(string(token))]
	return ok
}

// forcedCut returns the number of runes to consume when the buffer has reached
// MaxSegmentChars with no qualifying boundary: the last soft break at or
// before the limit, else the last whitespace at or before it, else a hard cut
// exactly at the limit. Every result is rune-aligned, so a multi-byte rune is
// never split.
func (s *Segmenter) forcedCut() int {
	limit := s.cfg.MaxSegmentChars

	for i := limit - 1; i >= 0; i-- {
		if isSoftBreak(s.buf[i]) && i+1 < len(s.buf) && unicode.IsSpace(s.buf[i+1]) {
			return i + 1
		}
	}
	for i := limit - 1; i >= 1; i-- {
		if unicode.IsSpace(s.buf[i]) {
			return i
		}
	}
	return limit
}

// isWhitespaceGuardedTerminator reports whether r ends a sentence only when
// followed by whitespace. The ellipsis is not ASCII but behaves like one: it
// can still be followed by more of the same token in a partially received
// fragment.
func isWhitespaceGuardedTerminator(r rune) bool {
	return r == '.' || r == '!' || r == '?' || r == '…'
}

func isFullWidthTerminator(r rune) bool {
	return r == '。' || r == '！' || r == '？'
}

func isClosingPunct(r rune) bool {
	switch r {
	case '"', '\'', '”', '’', ')', ']', '}', '»':
		return true
	}
	return false
}

func isSoftBreak(r rune) bool {
	switch r {
	case ',', ';', ':', '—', '–':
		return true
	}
	return false
}
