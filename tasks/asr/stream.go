package asr

import (
	"context"
	"errors"
	"fmt"
	"math"
	"unicode/utf8"

	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/tasks/asr/audio"
)

// blankBreak is the remembered class that means "the last frame was blank, or
// no frame has been analysed yet", the state greedy CTC decoding starts in and
// returns to on every blank. It is not a class index: blank is class 0 and a
// remembered class is never blank, so a separate value keeps the two apart.
const blankBreak = -1

// Stream is the incremental decoder of one utterance: it keeps a persistent
// individual built from the recognizer's parameters, the samples that do not
// yet fill a full analysis window, and the last emitted CTC class. For audio
// whose length is Window + k*Hop, its complete windows decode the same frames
// as Transcribe; Flush additionally handles a short final tail by padding it.
// It only ever reads samples that have arrived; nothing is computed from the
// future.
type Stream struct {
	ind       *learning.Individual
	cfg       RecognizerConfig
	pending   []float64
	prevClass int
	emitted   []rune
	frames    int
	flushed   bool
}

// newIndividual builds one persistent individual on the recognizer's current
// model: the same configuration, a copy of the parameters and options the
// trainer holds now, and zero initial voltage, which is the state a whole-file
// episode starts from. The individual owns its own optimizer and never writes
// back, so a stream cannot disturb training.
func (r *Recognizer) newIndividual() (*learning.Individual, error) {
	if r == nil || r.trainer == nil {
		return nil, errors.New("asr: nil recognizer")
	}
	s := r.trainer.Snapshot()
	return learning.NewIndividual(s.Config, s.Parameters, s.Options, make([]float64, s.Config.Dynamics.Nodes))
}

// NewStream builds a stream from the recognizer's current parameters. Later
// training does not reach a stream that already exists: it decodes with the
// parameters of the moment it was created.
func (r *Recognizer) NewStream() (*Stream, error) {
	if r != nil {
		if err := validateStreamAlphabet(r.config.Alphabet); err != nil {
			return nil, err
		}
	}
	ind, err := r.newIndividual()
	if err != nil {
		return nil, err
	}
	return &Stream{ind: ind, cfg: r.config, prevClass: blankBreak}, nil
}

type streamCheckpoint struct {
	individual learning.IndividualSnapshot
	pending    []float64
	prevClass  int
	emitted    []rune
	frames     int
	flushed    bool
}

func (s *Stream) checkpoint() streamCheckpoint {
	return streamCheckpoint{
		individual: s.ind.Snapshot(),
		pending:    append([]float64(nil), s.pending...),
		prevClass:  s.prevClass,
		emitted:    append([]rune(nil), s.emitted...),
		frames:     s.frames,
		flushed:    s.flushed,
	}
}

func (s *Stream) rollback(cp streamCheckpoint) error {
	ind, err := learning.RestoreIndividual(cp.individual)
	if err != nil {
		return fmt.Errorf("asr: stream rollback failed: %w", err)
	}
	s.ind = ind
	s.pending = append([]float64(nil), cp.pending...)
	s.prevClass = cp.prevClass
	s.emitted = append([]rune(nil), cp.emitted...)
	s.frames = cp.frames
	s.flushed = cp.flushed
	return nil
}

// Feed appends samples, analyses every complete window and returns the text
// those windows newly emitted. The framing is the one Features uses on the
// prefix that has arrived: the first window covers the first Window samples
// and each further window starts Hop samples later, so the stream keeps the
// last Window-Hop samples, the overlap the next window will reuse. Greedy CTC
// decoding carries across chunk boundaries through the remembered last class,
// so a character split over two chunks is still collapsed to one.
//
// A non-finite sample is an error and nothing of that chunk is consumed. Feed
// after Flush is an error. A chunk that fails part way through is rolled back
// as one operation, so retrying it cannot duplicate an already analysed frame.
func (s *Stream) Feed(ctx context.Context, samples []float64) (string, error) {
	if s == nil || s.ind == nil {
		return "", errors.New("asr: nil stream")
	}
	if s.flushed {
		return "", errors.New("asr: the stream was flushed and takes no more samples")
	}
	if ctx == nil {
		return "", errors.New("asr: nil context")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	for i, v := range samples {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return "", fmt.Errorf("asr: sample %d of this chunk is %v, not a finite number", i, v)
		}
	}
	cp := s.checkpoint()
	s.pending = append(s.pending, samples...)
	if err := ctx.Err(); err != nil {
		if rollbackErr := s.rollback(cp); rollbackErr != nil {
			return "", fmt.Errorf("%w; %v", err, rollbackErr)
		}
		return "", err
	}
	window, hop := s.cfg.FrontEnd.Window, s.cfg.FrontEnd.Hop
	for len(s.pending) >= window {
		text, err := s.step(ctx, s.pending[:window])
		if err != nil {
			if rollbackErr := s.rollback(cp); rollbackErr != nil {
				return "", fmt.Errorf("%w; %v", err, rollbackErr)
			}
			return "", err
		}
		s.emitted = append(s.emitted, text...)
		s.frames++
		s.pending = s.pending[hop:]
	}
	return string(s.emitted[len(cp.emitted):]), nil
}

// Flush analyses any complete windows already buffered, then zero-pads a
// remaining tail to one window and marks the stream done. A tail of at most
// Window-Hop samples is exactly the overlap the next window would have reused
// after at least one complete window, so it emits nothing; an initial tail has
// no preceding window and is therefore padded and analysed. A second Flush is
// an error.
func (s *Stream) Flush(ctx context.Context) (string, error) {
	if s == nil || s.ind == nil {
		return "", errors.New("asr: nil stream")
	}
	if s.flushed {
		return "", errors.New("asr: the stream was already flushed")
	}
	if ctx == nil {
		return "", errors.New("asr: nil context")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	window, hop := s.cfg.FrontEnd.Window, s.cfg.FrontEnd.Hop
	cp := s.checkpoint()
	for len(s.pending) >= window {
		text, err := s.step(ctx, s.pending[:window])
		if err != nil {
			if rollbackErr := s.rollback(cp); rollbackErr != nil {
				return "", fmt.Errorf("%w; %v", err, rollbackErr)
			}
			return "", err
		}
		s.emitted = append(s.emitted, text...)
		s.frames++
		s.pending = s.pending[hop:]
	}
	if len(s.pending) == 0 || (s.frames > 0 && len(s.pending) <= window-hop) {
		s.pending, s.flushed = nil, true
		return string(s.emitted[len(cp.emitted):]), nil
	}
	frame := make([]float64, window)
	copy(frame, s.pending)
	text, err := s.step(ctx, frame)
	if err != nil {
		if rollbackErr := s.rollback(cp); rollbackErr != nil {
			return "", fmt.Errorf("%w; %v", err, rollbackErr)
		}
		return "", err
	}
	s.emitted = append(s.emitted, text...)
	s.frames++
	s.pending, s.flushed = nil, true
	return string(s.emitted[len(cp.emitted):]), nil
}

// Text returns everything emitted so far.
func (s *Stream) Text() string {
	if s == nil {
		return ""
	}
	return string(s.emitted)
}

// step analyses one complete window: audio.LogMel over exactly those Window
// samples, which is one frame, one core step on that frame, and the greedy CTC
// rule on the logits it read out.
func (s *Stream) step(ctx context.Context, frame []float64) ([]rune, error) {
	rows, _, err := audio.LogMel(frame, s.cfg.SampleRate, s.cfg.FrontEnd)
	if err != nil {
		return nil, err
	}
	if len(rows) != 1 {
		return nil, fmt.Errorf("asr: a %d-sample window produced %d frames, want 1", len(frame), len(rows))
	}
	logits, err := s.ind.Advance(ctx, rows)
	if err != nil {
		return nil, err
	}
	if len(logits) != 1 {
		return nil, fmt.Errorf("asr: one frame read out %d rows, want 1", len(logits))
	}
	return s.emit(logits[0])
}

// emit applies one frame of greedy CTC decoding, the same rule ctc.Decode
// applies to a whole file, carried across chunk boundaries by the remembered
// class: argmax with ties to the lower class index, a repeat of the remembered
// class dropped, a blank dropped and remembered as a break so the next
// occurrence of the same class is emitted again. It returns at most one rune.
func (s *Stream) emit(row []float64) ([]rune, error) {
	if len(row) < 2 {
		return nil, fmt.Errorf("asr: a frame has %d classes, want at least 2", len(row))
	}
	best := 0
	for k := 1; k < len(row); k++ {
		if row[k] > row[best] {
			best = k
		}
	}
	if best == 0 {
		s.prevClass = blankBreak
		return nil, nil
	}
	if best > len(s.cfg.Alphabet) {
		return nil, fmt.Errorf("asr: decoded class %d outside the alphabet", best)
	}
	if best == s.prevClass {
		return nil, nil
	}
	s.prevClass = best
	return []rune{s.cfg.Alphabet[best-1]}, nil
}

// StreamState is the resumable snapshot of a stream: the persistent
// individual, the samples that do not yet fill a window, the remembered CTC
// class, the text emitted so far, the recognizer's audio semantics and whether
// the stream is done. It owns no buffer the stream still uses, so taking one
// and feeding more audio are independent.
type StreamState struct {
	Individual      learning.IndividualSnapshot `json:"individual"`
	Pending         []float64                   `json:"pending"`
	PrevClass       int                         `json:"prev_class"`
	Emitted         string                      `json:"emitted"`
	SampleRate      int                         `json:"sample_rate"`
	FrontEnd        audio.FrontEndConfig        `json:"front_end"`
	FrontEndVersion string                      `json:"front_end_version"`
	Alphabet        []rune                      `json:"alphabet"`
	Flushed         bool                        `json:"flushed"`
}

// Snapshot returns the stream's resumable state. A nil or zero Stream returns
// StreamState{}, which no recognizer accepts back.
func (s *Stream) Snapshot() StreamState {
	if s == nil || s.ind == nil {
		return StreamState{}
	}
	return StreamState{
		Individual:      s.ind.Snapshot(),
		Pending:         append([]float64(nil), s.pending...),
		PrevClass:       s.prevClass,
		Emitted:         string(s.emitted),
		SampleRate:      s.cfg.SampleRate,
		FrontEnd:        s.cfg.FrontEnd,
		FrontEndVersion: audio.FrontEndVersion,
		Alphabet:        append([]rune(nil), s.cfg.Alphabet...),
		Flushed:         s.flushed,
	}
}

// RestoreStream resumes a stream this recognizer can carry on: the snapshot's
// individual must have been built from the same core configuration, and its
// sample rate, front end and alphabet must also match so equal-dimensional
// models cannot reinterpret the buffered audio or emitted text. The restored
// stream keeps its own copies.
func (r *Recognizer) RestoreStream(st StreamState) (*Stream, error) {
	reference, err := r.newIndividual()
	if err != nil {
		return nil, err
	}
	if err := validateStreamAlphabet(r.config.Alphabet); err != nil {
		return nil, err
	}
	if st.SampleRate != r.config.SampleRate {
		return nil, fmt.Errorf("asr: stream snapshot sample rate %d Hz is not this recognizer's %d Hz", st.SampleRate, r.config.SampleRate)
	}
	if st.FrontEnd != r.config.FrontEnd {
		return nil, errors.New("asr: stream snapshot front end does not match this recognizer")
	}
	if st.FrontEndVersion != audio.FrontEndVersion {
		return nil, fmt.Errorf("asr: stream snapshot front end version %q is not %q", st.FrontEndVersion, audio.FrontEndVersion)
	}
	if !sameRunes(st.Alphabet, r.config.Alphabet) {
		return nil, errors.New("asr: stream snapshot alphabet does not match this recognizer")
	}
	if !utf8.ValidString(st.Emitted) {
		return nil, errors.New("asr: stream snapshot emitted text is not valid UTF-8")
	}
	if st.Flushed && len(st.Pending) != 0 {
		return nil, errors.New("asr: flushed stream snapshot still has pending samples")
	}
	if st.Individual.Plastic != nil || st.Individual.Chemical != nil {
		return nil, errors.New("asr: stream snapshot contains unsupported individual mechanisms")
	}
	if want := reference.Snapshot().ConfigHash; st.Individual.ConfigHash != want {
		return nil, fmt.Errorf("asr: stream snapshot configuration %q is not this recognizer's %q", st.Individual.ConfigHash, want)
	}
	if st.PrevClass != blankBreak && (st.PrevClass < 1 || st.PrevClass > len(r.config.Alphabet)) {
		return nil, fmt.Errorf("asr: stream snapshot remembers class %d, outside 1..%d and not %d", st.PrevClass, len(r.config.Alphabet), blankBreak)
	}
	for i, v := range st.Pending {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return nil, fmt.Errorf("asr: pending sample %d is %v, not a finite number", i, v)
		}
	}
	steps := individualSteps(st.Individual)
	if steps > uint64(^uint(0)>>1) {
		return nil, errors.New("asr: stream snapshot frame count is too large")
	}
	window, hop := r.config.FrontEnd.Window, r.config.FrontEnd.Hop
	if len(st.Pending) >= window {
		return nil, fmt.Errorf("asr: stream snapshot has %d pending samples, want fewer than %d", len(st.Pending), window)
	}
	if !st.Flushed && steps > 0 && len(st.Pending) < window-hop {
		return nil, fmt.Errorf("asr: stream snapshot has %d pending samples after %d frames, want at least %d", len(st.Pending), steps, window-hop)
	}
	runeCount := utf8.RuneCountInString(st.Emitted)
	if uint64(runeCount) > steps {
		return nil, fmt.Errorf("asr: stream snapshot has %d emitted runes after %d frames", runeCount, steps)
	}
	runes := []rune(st.Emitted)
	for i, ru := range runes {
		if _, ok := r.index[ru]; !ok {
			return nil, fmt.Errorf("asr: stream snapshot emitted rune %q at position %d outside the alphabet", ru, i)
		}
	}
	if steps == 0 && (len(runes) != 0 || st.PrevClass != blankBreak) {
		return nil, errors.New("asr: stream snapshot has text or a class before its first frame")
	}
	if st.PrevClass != blankBreak {
		if len(runes) == 0 || r.index[runes[len(runes)-1]] != st.PrevClass {
			return nil, errors.New("asr: stream snapshot previous class does not match its emitted text")
		}
	}
	minFrames := uint64(len(runes))
	for i := 1; i < len(runes); i++ {
		if runes[i] == runes[i-1] {
			minFrames++
		}
	}
	if len(runes) > 0 && st.PrevClass == blankBreak {
		minFrames++
	}
	if minFrames > steps {
		return nil, fmt.Errorf("asr: stream snapshot needs at least %d frames for %d emitted runes, has %d", minFrames, len(runes), steps)
	}
	ind, err := learning.RestoreIndividual(st.Individual)
	if err != nil {
		return nil, err
	}
	return &Stream{
		ind:       ind,
		cfg:       r.config,
		pending:   append([]float64(nil), st.Pending...),
		prevClass: st.PrevClass,
		emitted:   []rune(st.Emitted),
		frames:    int(steps),
		flushed:   st.Flushed,
	}, nil
}

// TranscribeStreaming feeds x in chunks of chunk samples through a fresh
// stream and flushes it, so a test or an evaluation can put the streaming and
// the whole-file paths side by side. A signal of Window + k*Hop samples
// decodes to exactly what Transcribe returns for it; other lengths may include
// the explicitly padded final tail.
func (r *Recognizer) TranscribeStreaming(ctx context.Context, x []float64, chunk int) (string, error) {
	if chunk < 1 {
		return "", fmt.Errorf("asr: streaming chunk %d must be at least one sample", chunk)
	}
	s, err := r.NewStream()
	if err != nil {
		return "", err
	}
	for i := 0; i < len(x); {
		end := len(x)
		if remaining := len(x) - i; chunk < remaining {
			end = i + chunk
		}
		if _, err := s.Feed(ctx, x[i:end]); err != nil {
			return "", err
		}
		i = end
	}
	if _, err := s.Flush(ctx); err != nil {
		return "", err
	}
	return s.Text(), nil
}

func sameRunes(a, b []rune) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func validateStreamAlphabet(alphabet []rune) error {
	for i, r := range alphabet {
		if !utf8.ValidRune(r) {
			return fmt.Errorf("asr: alphabet rune at position %d is not valid Unicode", i)
		}
	}
	return nil
}

func individualSteps(s learning.IndividualSnapshot) uint64 {
	switch {
	case s.Neural.Continuous != nil:
		return s.Neural.Continuous.Steps
	case s.Neural.LIF != nil:
		return s.Neural.LIF.Steps
	case s.Neural.Mixed != nil:
		return s.Neural.Mixed.Steps
	default:
		return 0
	}
}
