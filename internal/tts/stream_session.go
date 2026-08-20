package tts

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fx/meadowlark/internal/segment"
	"github.com/fx/meadowlark/internal/voice"
	"github.com/fx/meadowlark/internal/wyoming"
)

// maxInFlight bounds how many upstream segment requests a session holds open at
// once: the segment currently being emitted plus exactly one prefetch.
//
// It is a package constant rather than a flag because upstream time-to-first-byte
// (~0.2-0.6 s) is far shorter than a segment's playback duration (at least a
// couple of seconds at the configured minimum length), so a depth of two already
// hides it completely. Deeper prefetch multiplies upstream cost and wasted work
// on cancellation for no perceptible gain.
const maxInFlight = 2

// errSessionCanceled is reported by a session's writer wrapper once the session
// context is done, so the emitter stops mid-segment instead of draining a body
// nobody is waiting for. It is never written to the connection.
var errSessionCanceled = errors.New("tts: streaming session canceled")

// StreamSession drives at most one Wyoming streaming-synthesis session for a
// single connection: synthesize-start opens it, synthesize-chunk feeds it,
// synthesize-stop closes it, and the compatibility synthesize event is absorbed
// while it is not idle.
//
// A session is in exactly one of three states:
//
//   - idle — no session. Events fall through to the caller's non-streaming path.
//   - open — accumulating text and emitting audio.
//   - terminated — a tombstone left by a session that failed. It emits nothing
//     further but keeps absorbing that message's remaining events, because Home
//     Assistant sends its compatibility synthesize after the chunks: were the
//     session simply closed, that event would find no session and the whole
//     message would be spoken a second time.
//
// The state is derived rather than stored: no run is idle, a run whose failed
// flag is set is terminated, any other run is open.
//
// Every method may be called only from the connection's read loop, except Close
// which may also be called from the teardown path. The writer handed to Start
// must be safe for concurrent use, because the session's emitter goroutine
// writes audio events to it while the read loop may write its own events.
type StreamSession struct {
	proxy       *Proxy
	cfg         segment.Config
	idleTimeout time.Duration
	logger      *slog.Logger

	mu     sync.Mutex
	run    *streamRun
	closed bool
}

// NewStreamSession creates an idle session bound to a proxy. One per connection.
//
// An idleTimeout of zero disables the idle timer entirely. A negative timeout is
// a configuration error that R9 requires the command layer to reject before it
// gets here; it is treated as disabled rather than handed to a timer, which
// would fire it the instant every session opened.
func NewStreamSession(p *Proxy, cfg segment.Config, idleTimeout time.Duration, logger *slog.Logger) *StreamSession {
	if logger == nil {
		logger = slog.Default()
	}
	if idleTimeout < 0 {
		logger.Warn("negative streaming session timeout; idle timer disabled", "timeout", idleTimeout)
		idleTimeout = 0
	}
	return &StreamSession{proxy: p, cfg: cfg, idleTimeout: idleTimeout, logger: logger}
}

// Active reports whether the session is holding a message — either open or
// terminated. Both absorb events; only an idle session lets them through.
func (s *StreamSession) Active() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.run != nil
}

// Start opens a streaming session, replacing any session already held by this
// connection.
//
// Replacing one means quiescing it first — cancel, join the emitter, discard the
// queue — so no audio event of the abandoned session can be written after the
// synthesize-stopped that terminates it. A session that had already failed keeps
// the error it emitted as its terminator and gets no synthesize-stopped.
//
// The returned error is never a synthesis failure; see the note on Stop.
func (s *StreamSession) Start(ctx context.Context, w io.Writer, ev *wyoming.SynthesizeStart) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}

	var writeErr error
	if prev := s.run; prev != nil {
		s.disarmTimer(prev)
		prev.quiesce(joinSupervisor)
		// A terminated run already spent its single terminator on the error
		// event, so this writes synthesize-stopped only for a run still open.
		writeErr = prev.terminate(prev.stoppedEvent)
		s.run = nil
	}

	if ev.TextFormat == "ssml" {
		s.logger.Debug("ssml text format is treated as plain text")
	}
	s.logger.Debug("streaming session started",
		"voice", ev.Voice, "language", ev.Language, "speaker", ev.Speaker)

	runCtx, cancel := context.WithCancel(ctx)
	r := &streamRun{
		proxy:     s.proxy,
		logger:    s.logger,
		w:         w,
		ctx:       runCtx,
		cancel:    cancel,
		voiceName: ev.Voice,
		seg:       segment.New(s.cfg),
		sem:       make(chan struct{}, maxInFlight),
		ready:     make(chan *segJob, maxInFlight),
		emitDone:  make(chan struct{}),
	}
	r.qcond = sync.NewCond(&r.qmu)
	r.wg.Add(2)
	go r.openLoop()
	go r.emitLoop()
	r.swg.Add(1)
	go r.supervise()

	s.run = r
	s.armTimer(r)
	return writeErr
}

// Chunk appends a fragment of text to the open session, flushing any segments
// that became complete. It is ignored when no session is open and absorbed
// silently by a terminated one.
func (s *StreamSession) Chunk(ev *wyoming.SynthesizeChunk) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	r := s.run
	if r == nil {
		s.logger.Debug("synthesize-chunk outside a session; ignoring")
		return nil
	}
	if r.failed.Load() {
		return nil
	}

	s.resetTimer(r)
	r.chunks++
	r.addText(ev.Text)
	return nil
}

// Compat handles Home Assistant's compatibility synthesize event, which carries
// the entire message and arrives after the chunks.
//
// It reports true when the session absorbed the event, which is the case in both
// the open state (the message is already being synthesized from its chunks) and
// the terminated state (the message failed, and synthesizing it now would speak
// it a second time long after the client raised). It reports false only when the
// session is idle, in which case the caller must fall through to the ordinary
// whole-message path, unchanged.
func (s *StreamSession) Compat(ev *wyoming.Synthesize) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	r := s.run
	if r == nil {
		return false
	}
	if r.failed.Load() {
		s.logger.Debug("absorbing compatibility synthesize for a terminated session")
		return true
	}

	s.resetTimer(r)
	// Kept only as a fallback: it becomes the session's content if no
	// synthesize-chunk ever arrived, and is discarded otherwise.
	r.fallback = ev.Text
	r.hasFallback = true
	return true
}

// Stop ends the session: it flushes the remaining text, waits for every segment
// to finish emitting, and writes the single terminating synthesize-stopped. A
// terminated session emits nothing — its error was the terminator — and a
// synthesize-stop with no session at all emits nothing either.
//
// The returned error is never a synthesis failure. A failed segment, a format
// mismatch, or an idle timeout is reported by the session itself as a Wyoming
// error event, and the method still returns nil: the Wyoming server turns a
// non-nil handler error into a second error event with code handler-error, which
// would both break the exactly-one-terminator rule and report the wrong code. A
// non-nil return means only that a write to the connection failed.
func (s *StreamSession) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	r := s.run
	if r == nil {
		s.logger.Debug("synthesize-stop outside a session; ignoring")
		return nil
	}
	s.disarmTimer(r)
	s.run = nil

	if r.failed.Load() {
		// Tombstone: the error already terminated this message. Join whatever
		// is still unwinding and write nothing.
		r.quiesce(joinSupervisor)
		return r.writeErr
	}
	return r.finish()
}

// Close tears the session down for a connection that is going away. It cancels
// in-flight upstream requests, closes their bodies, and blocks until the
// session's goroutines have exited, so the server's shutdown drain is genuine.
//
// Nothing is written: the connection is gone, so neither a closing audio-stop
// nor a terminator may be emitted.
func (s *StreamSession) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true

	if r := s.run; r != nil {
		s.disarmTimer(r)
		r.teardown.Store(true)
		r.quiesce(joinSupervisor)
		s.run = nil
	}
}

// --- idle timer ------------------------------------------------------------
//
// The timer is armed when a session opens and re-armed by every subsequent
// client event belonging to it. Meadowlark's own progress does not reset it: a
// session whose client has gone silent is dead however much audio is still
// draining.
//
// Every arming carries a generation, and an expiry that does not match the
// current generation is stale — it lost the race with an event that arrived
// while the timer callback was waiting for the session lock.

func (s *StreamSession) armTimer(r *streamRun) {
	if s.idleTimeout <= 0 {
		return
	}
	gen := r.timerGen
	r.timer = time.AfterFunc(s.idleTimeout, func() { s.onIdleTimeout(r, gen) })
}

func (s *StreamSession) resetTimer(r *streamRun) {
	if s.idleTimeout <= 0 {
		return
	}
	s.disarmTimer(r)
	s.armTimer(r)
}

func (s *StreamSession) disarmTimer(r *streamRun) {
	r.timerGen++
	if r.timer != nil {
		r.timer.Stop()
		r.timer = nil
	}
}

// onIdleTimeout abandons a session whose client has stopped sending: quiesce,
// emit a Wyoming error with code synthesize-timeout, and leave the tombstone
// behind so the rest of the message is absorbed rather than synthesized.
func (s *StreamSession) onIdleTimeout(r *streamRun, gen int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.run != r || r.timerGen != gen || r.failed.Load() {
		return
	}

	s.logger.Warn("streaming session idle timeout", "timeout", s.idleTimeout)
	r.quiesce(joinSupervisor)
	r.failed.Store(true)
	if err := r.terminate(func() error {
		return r.write(&wyoming.Error{Text: "streaming session idle timeout", Code: "synthesize-timeout"})
	}); err != nil {
		s.logger.Error("synthesize-timeout error event not written", "error", err)
	}
}

// --- the run ---------------------------------------------------------------

// segJob is one segment on its way to the connection: either an opened upstream
// response whose audio format is already known, or the failure that stopped it
// being opened.
type segJob struct {
	seg *OpenSegment
	err error
}

// segFailure is a domain failure recorded by the emitter for the supervisor to
// turn into the session's terminator. An empty code means the connection itself
// failed, so the session is quiesced but nothing is written.
type segFailure struct {
	err  error
	code string
}

// queuedText is a segment of text waiting to be synthesized, carrying the
// session's parsed input overrides so the opener can resolve the plan from the
// first one.
type queuedText struct {
	text   string
	parsed voice.ParsedInput
}

// streamRun is one generation of a streaming session — everything that a
// synthesize-start creates and a terminator destroys. A new start builds a new
// run rather than resetting this one, so no field can survive a restart by
// accident.
type streamRun struct {
	proxy  *Proxy
	logger *slog.Logger
	w      io.Writer
	ctx    context.Context
	cancel context.CancelFunc

	// voiceName is the start voice. It alone selects the endpoint, for the whole
	// session; an input voice override changes only the parameter sent upstream.
	voiceName string

	// --- owned by the connection's read loop, under StreamSession.mu ---

	seg         *segment.Segmenter
	parsed      voice.ParsedInput
	sniffed     bool
	override    bool
	held        strings.Builder // text seen before the override sniff decided
	raw         strings.Builder // whole message, buffered for override-form input
	chunks      int
	fallback    string
	hasFallback bool
	timer       *time.Timer
	timerGen    int

	// --- the segment queue, guarded by qmu ---

	qmu     sync.Mutex
	qcond   *sync.Cond
	pending []queuedText
	qclosed bool

	// --- the pipeline ---

	sem   chan struct{} // bounds open upstream requests to maxInFlight
	ready chan *segJob  // ordered FIFO from the opener to the emitter
	wg    sync.WaitGroup

	// emitDone is closed by the emitter as it exits, after it has written any
	// closing audio-stop and recorded any failure. swg tracks the supervisor
	// that turns such a failure into the session's terminator; it is separate
	// from wg because the supervisor joins wg itself.
	emitDone chan struct{}
	failure  *segFailure
	swg      sync.WaitGroup

	// plan is owned by the opener, format by the emitter.
	plan   *synthesisPlan
	format *AudioFormat

	// failed marks the run as a tombstone. It is set by whichever path
	// terminated the session with an error and read by the session's methods, so
	// it is atomic rather than lock-guarded: the emitter must never take the
	// session lock, because quiesce holds it while joining the emitter.
	failed   atomic.Bool
	teardown atomic.Bool
	termOnce sync.Once

	// writeErr records a failed write to the connection. The emitter sets it
	// before exiting and session methods read it only after joining, so the join
	// is what orders the two.
	writeErr error
}

func (r *streamRun) write(ev interface{ ToEvent() *wyoming.Event }) error {
	return wyoming.WriteEvent(r.w, ev.ToEvent())
}

func (r *streamRun) stoppedEvent() error { return r.write(&wyoming.SynthesizeStopped{}) }

// terminate writes the session's single terminator, if it has not been written
// already. Exactly one of synthesize-stopped and error reaches the client:
// Home Assistant raises on error and breaks its read loop on synthesize-stopped,
// so emitting both would leave an unconsumed terminator in the connection buffer
// for the next stream to read as an immediate, silent end.
//
// It writes nothing once a write for this session has failed, and reports that
// earlier failure instead. That is what keeps the two halves of the error split
// from colliding: the failure is handed back to the caller, which surfaces it as
// the server's handler-error, so a terminator written here as well would be the
// second one for the same message.
//
// The guard is per session, which is the scope the exactly-one-terminator rule
// is written in. A connection whose write failed is gone — the server's own
// follow-up write fails too and its read loop returns — so whether a later
// synthesize-start could open another session on it is moot.
func (r *streamRun) terminate(write func() error) error {
	if r.writeErr != nil {
		return r.writeErr
	}
	var err error
	r.termOnce.Do(func() { err = write() })
	if err != nil {
		r.writeErr = err
	}
	return err
}

// addText feeds a fragment into the session.
//
// The session's first non-whitespace character decides, once, whether the
// message is override-form. voice.ParseInput dispatches on that same character:
// a leading '{' means it unmarshals the entire string as one JSON object, and a
// leading '[' means it consumes leading tag groups. Neither works on a fragment
// — a partial JSON object fails to unmarshal and falls through to plain text, so
// the braces would be spoken aloud and every override silently dropped. So an
// override-form message buffers whole and is parsed at synthesize-stop, while
// ordinary prose is never handed to ParseInput at all and segments as it
// arrives.
func (r *streamRun) addText(text string) {
	if text == "" {
		return
	}

	if !r.sniffed {
		r.held.WriteString(text)
		trimmed := strings.TrimSpace(r.held.String())
		if trimmed == "" {
			// Nothing but whitespace so far; the deciding character is still to
			// come.
			return
		}
		r.sniffed = true
		r.override = trimmed[0] == '{' || trimmed[0] == '['
		buffered := r.held.String()
		r.held.Reset()
		if r.override {
			r.logger.Debug("override-form input; incremental segmentation suspended")
			r.raw.WriteString(buffered)
			return
		}
		r.enqueue(r.seg.Add(buffered))
		return
	}

	if r.override {
		r.raw.WriteString(text)
		return
	}
	r.enqueue(r.seg.Add(text))
}

// finish completes a session normally: flush what is left, wait for every
// segment to be emitted, then write synthesize-stopped.
func (r *streamRun) finish() error {
	// Nothing is in flight once the drain below returns, so this only releases
	// the session context from its parent.
	defer r.cancel()

	// A session that received no synthesize-chunk at all takes the compatibility
	// synthesize's text as its content. It is a whole message, so it goes down
	// the same path as chunked text, override sniff included.
	if r.chunks == 0 && r.hasFallback {
		r.addText(r.fallback)
	}

	if r.override {
		// The whole message has arrived, so ParseInput can finally see it.
		r.parsed = voice.ParseInput(r.raw.String())
		r.enqueue(r.seg.Add(r.parsed.Input))
	}
	r.enqueue(r.seg.Flush())

	r.closeQueue()
	r.wg.Wait()
	r.swg.Wait()
	r.drainReady()
	r.discardQueue()

	if r.failed.Load() {
		// A failure during the drain already spent the terminator on its error
		// event, or broke the connection. Either way a session that errored gets
		// no synthesize-stopped.
		return r.writeErr
	}
	return r.terminate(r.stoppedEvent)
}

// quiesce is the ordered shutdown every path that ends a session early must
// perform — a restart, a failure, the idle timeout, connection teardown — before
// anything else is written to the connection:
//
//  1. cancel the session context, aborting in-flight and prefetched upstream
//     requests and closing their bodies;
//  2. join the emitter, which is the only goroutine that writes audio events and
//     which writes the closing audio-stop for a group it had opened as its final
//     act, so after the join no audio event for this session can ever appear;
//  3. discard the queue, closing any prefetched segment that will never be
//     emitted.
//
// Only then may a terminator be written or a replacement session open. The join
// must come before any closing audio-stop and the emitter must be the one to
// write it: cancelling a context does not synchronously stop a goroutine already
// inside a write, so a terminating path that wrote the audio-stop itself and
// then joined could still see an audio-chunk land after it.
// joinSupervisor / fromSupervisor name quiesce's only variation: the failure
// supervisor performs the same shutdown, and is the one caller that must not
// wait for itself.
const (
	joinSupervisor = true
	fromSupervisor = false
)

func (r *streamRun) quiesce(join bool) {
	r.cancel()
	r.qcond.Broadcast()
	r.wg.Wait()
	if join {
		r.swg.Wait()
	}
	r.drainReady()
	r.discardQueue()
}

// --- the segment queue -----------------------------------------------------

func (r *streamRun) enqueue(texts []string) {
	if len(texts) == 0 {
		return
	}
	r.qmu.Lock()
	for _, t := range texts {
		r.pending = append(r.pending, queuedText{text: t, parsed: r.parsed})
	}
	r.qmu.Unlock()
	r.qcond.Broadcast()
}

func (r *streamRun) closeQueue() {
	r.qmu.Lock()
	r.qclosed = true
	r.qmu.Unlock()
	r.qcond.Broadcast()
}

func (r *streamRun) discardQueue() {
	r.qmu.Lock()
	r.pending = nil
	r.qmu.Unlock()
}

// nextText blocks until a segment is queued, reporting false once the queue is
// closed and drained or the session is cancelled.
func (r *streamRun) nextText() (queuedText, bool) {
	r.qmu.Lock()
	defer r.qmu.Unlock()
	for len(r.pending) == 0 && !r.qclosed && r.ctx.Err() == nil {
		r.qcond.Wait()
	}
	if r.ctx.Err() != nil || len(r.pending) == 0 {
		return queuedText{}, false
	}
	item := r.pending[0]
	r.pending = r.pending[1:]
	return item, true
}

// --- the opener ------------------------------------------------------------

// openLoop issues upstream requests ahead of emission. Prefetching a segment is
// exactly an early openSegment call: the request is issued and its body held
// undrained, so the upstream's time-to-first-byte is spent while the previous
// segment is still playing.
func (r *streamRun) openLoop() {
	defer r.wg.Done()
	defer close(r.ready)

	for {
		item, ok := r.nextText()
		if !ok {
			return
		}
		if !r.acquire() {
			return
		}

		if r.plan == nil {
			// Resolve once for the whole session. The start voice fixes the
			// endpoint; the parsed input overrides only the request parameters.
			plan, err := r.proxy.resolveSynthesis(r.ctx, r.voiceName, item.parsed)
			if err != nil {
				r.release()
				r.send(&segJob{err: err})
				return
			}
			r.plan = plan
		}

		seg, err := r.proxy.openSegment(r.ctx, r.plan, item.text)
		if err != nil {
			r.release()
			r.send(&segJob{err: err})
			return
		}
		if !r.send(&segJob{seg: seg}) {
			_ = seg.Close()
			r.release()
			return
		}
	}
}

// acquire takes one of the maxInFlight upstream slots, releasing the caller when
// the session is cancelled instead.
func (r *streamRun) acquire() bool {
	select {
	case r.sem <- struct{}{}:
		return true
	case <-r.ctx.Done():
		return false
	}
}

func (r *streamRun) release() { <-r.sem }

func (r *streamRun) send(job *segJob) bool {
	select {
	case r.ready <- job:
		return true
	case <-r.ctx.Done():
		return false
	}
}

// drainReady closes every segment the emitter will never take. It is safe only
// after the join, which is what closed the channel.
func (r *streamRun) drainReady() {
	for job := range r.ready {
		if job.seg != nil {
			_ = job.seg.Close()
		}
	}
}

// --- the emitter -----------------------------------------------------------

// emitLoop is the only goroutine that writes audio events. That is a load-
// bearing invariant rather than an implementation detail: because it alone
// writes audio-start, audio-chunk and audio-stop, it is the only thing that can
// correctly close a group it has opened, which is what makes an ordered shutdown
// possible at all. It never touches job N+1 before writing job N's audio-stop,
// so ordering is structural rather than a timing accident.
func (r *streamRun) emitLoop() {
	defer r.wg.Done()
	// Closed only once every audio event this session will ever emit has been
	// written, which is what lets the supervisor order the terminator after it.
	defer close(r.emitDone)

	for job := range r.ready {
		if !r.emitOne(job) {
			return
		}
	}
}

// emitOne emits one segment, reporting false when the session must stop.
func (r *streamRun) emitOne(job *segJob) bool {
	if r.ctx.Err() != nil {
		// The session is being quiesced: whatever was prefetched is discarded
		// unsynthesized. Whoever cancelled us owns the terminator.
		if job.seg != nil {
			_ = job.seg.Close()
		}
		return false
	}
	if job.err != nil {
		r.recordFailure(job.err, "tts-error")
		return false
	}
	defer func() {
		_ = job.seg.Close()
		r.release()
	}()

	format := job.seg.Format()
	switch {
	case r.format == nil:
		r.format = format
	case *format != *r.format:
		// Home Assistant writes its WAV header from the first audio-start it
		// sees and ignores every later one, so the first segment's format governs
		// the session. Emitting mismatched PCM under that header is audibly wrong
		// with no error anywhere; resampling is out of scope. The segment was
		// opened without writing a byte, so it can still be rejected cleanly.
		r.logger.Warn("audio format changed mid-session",
			"first", formatString(r.format), "segment", formatString(format))
		r.recordFailure(fmt.Errorf("audio format changed mid-session: first segment %s, this segment %s",
			formatString(r.format), formatString(format)), "tts-error")
		return false
	}

	cw := &cancelWriter{w: r.w, ctx: r.ctx}
	err := r.proxy.emitSegment(cw, job.seg)
	if err == nil {
		return true
	}

	groupOpen := cw.ok > 0
	switch {
	case r.ctx.Err() != nil:
		// The session is being quiesced — by our own writer refusing to write,
		// or by the upstream body being closed under us. Either way, close the
		// group we opened as our final act and leave the terminator to whoever
		// cancelled us.
		r.closeGroup(groupOpen)
	case cw.err != nil:
		// The connection itself failed. Nothing more can be written to it, so the
		// session is quiesced with no terminator at all.
		r.writeErr = err
		r.recordFailure(err, "")
	default:
		// The upstream failed. Close the group first so the client is never left
		// inside an unterminated one; the supervisor writes the error once this
		// goroutine has been joined.
		r.closeGroup(groupOpen)
		r.recordFailure(err, "tts-error")
	}
	return false
}

// closeGroup writes the audio-stop for a group this goroutine opened, which is
// the emitter's final act. Nothing is written on connection teardown: the
// connection is gone.
func (r *streamRun) closeGroup(open bool) {
	if !open || r.teardown.Load() {
		return
	}
	if err := r.write(&wyoming.AudioStop{}); err != nil {
		// The connection is broken, so this session writes nothing further:
		// terminate reports this error rather than adding a terminator after it.
		r.logger.Error("failed to write closing audio-stop", "error", err)
		r.writeErr = err
	}
}

// recordFailure marks the session as failed and hands the failure to the
// supervisor. It writes nothing: the error may only be written once the emitter
// has been joined, and the emitter is what calls this.
//
// The tombstone is raised here rather than by the supervisor so that every event
// arriving between the failure and its terminator is already absorbed.
func (r *streamRun) recordFailure(err error, code string) {
	r.logger.Error("streaming synthesis failed", "error", err, "code", code)
	r.failure = &segFailure{err: err, code: code}
	r.failed.Store(true)
	r.cancel()
}

// supervise turns a failure recorded by the emitter into the session's single
// terminator, performing R5's quiesce first — cancel, join the emitter, discard
// the queue — so the error is the last thing the client sees for this message
// and no prefetched body outlives it.
//
// It exists because the emitter cannot join itself, and because a client that
// has gone quiet would otherwise never trigger the join: nothing but this
// goroutine is guaranteed to run again after a mid-session failure.
func (r *streamRun) supervise() {
	defer r.swg.Done()

	// The emitter has written its last audio event, including any closing
	// audio-stop for a group it had opened.
	<-r.emitDone
	f := r.failure
	if f == nil {
		return
	}

	r.quiesce(fromSupervisor)

	// An empty code means the connection itself failed, and teardown means it is
	// gone: either way there is nothing worth writing to it.
	if f.code == "" || r.teardown.Load() {
		return
	}
	if writeErr := r.terminate(func() error {
		return r.write(&wyoming.Error{Text: f.err.Error(), Code: f.code})
	}); writeErr != nil {
		r.logger.Error("TTS error event not written", "error", writeErr)
	}
}

func formatString(f *AudioFormat) string {
	return fmt.Sprintf("%d Hz, %d-bit, %d channel(s)", f.Rate, f.Width*8, f.Channels)
}

// cancelWriter fails every write once the session context is done, so the
// emitter stops mid-segment on cancellation rather than draining a body nobody
// is waiting for. It also records whether the connection itself has failed, and
// how many events were written, which is what tells the emitter whether it has
// an audio group left open.
type cancelWriter struct {
	w   io.Writer
	ctx context.Context
	ok  int
	err error
}

func (c *cancelWriter) Write(p []byte) (int, error) {
	if c.ctx.Err() != nil {
		return 0, errSessionCanceled
	}
	n, err := c.w.Write(p)
	if err != nil {
		c.err = err
		return n, err
	}
	c.ok++
	return n, nil
}
