package tokenizer

import (
	"errors"
	"fmt"
	"unicode/utf8"
)

// StreamDecoder turns ids into text incrementally and never drops bytes: an
// incomplete UTF-8 sequence stays buffered until its continuation bytes
// arrive; Flush returns the error "incomplete UTF-8 sequence (%d bytes)" if
// bytes remain, instead of emitting a replacement character silently. Specials
// decode to "". A decoder is not safe for concurrent use.
type StreamDecoder struct {
	vocab Vocabulary
	buf   []byte
}

// NewStreamDecoder returns a decoder that reads ids through v.
func NewStreamDecoder(v Vocabulary) *StreamDecoder {
	return &StreamDecoder{vocab: v}
}

// Feed appends one id and returns the text that is now safe to emit: every
// complete character at the front of the buffer. An id the vocabulary does not
// know is an error and leaves the buffer untouched, so feeding the rest of a
// sequence afterwards still completes it.
func (d *StreamDecoder) Feed(id int) (string, error) {
	if d.vocab == nil {
		return "", errors.New("tokenizer: stream decoder has no vocabulary")
	}
	piece, err := d.vocab.Decode([]int{id})
	if err != nil {
		return "", err
	}
	d.buf = append(d.buf, piece...)
	return d.take(), nil
}

// Flush ends the stream. It reports what is still buffered rather than
// guessing at it, and keeps those bytes, so a stream that turns out to
// continue can still be completed by feeding the missing ids.
func (d *StreamDecoder) Flush() (string, error) {
	if len(d.buf) > 0 {
		return "", fmt.Errorf("incomplete UTF-8 sequence (%d bytes)", len(d.buf))
	}
	return "", nil
}

// take removes and returns the longest prefix of the buffer made of complete
// UTF-8 encodings. A byte that can never begin a valid sequence is complete by
// this measure and is emitted as itself: it came from the ids, so dropping or
// replacing it would lose what the stream actually carried. Only a sequence
// that is still waiting for continuation bytes stays behind.
func (d *StreamDecoder) take() string {
	emit := 0
	for emit < len(d.buf) && utf8.FullRune(d.buf[emit:]) {
		_, size := utf8.DecodeRune(d.buf[emit:])
		emit += size
	}
	if emit == 0 {
		return ""
	}
	out := string(d.buf[:emit])
	d.buf = d.buf[:copy(d.buf, d.buf[emit:])]
	return out
}
