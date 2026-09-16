package teacher

import (
	"context"
	"errors"
	"sync"
)

// ErrTeacherBlocked is returned by Blocked for every Ask.
var ErrTeacherBlocked = errors.New("teacher: teacher access is blocked in this mode")

// Blocked refuses every request and counts the attempts, so an evaluation that
// must not consult a teacher can prove it never did.
type Blocked struct {
	ID      string
	Version string

	mu    sync.Mutex
	calls int
}

// Ask counts the attempt and always returns ErrTeacherBlocked without
// validating the request or consulting the context.
func (b *Blocked) Ask(ctx context.Context, r Request) (Response, error) {
	b.mu.Lock()
	b.calls++
	b.mu.Unlock()
	return Response{}, ErrTeacherBlocked
}

// Describe identifies this teacher as a blocked teacher.
func (b *Blocked) Describe() Descriptor {
	return Descriptor{ID: b.ID, Version: b.Version, Kind: DescriptorBlocked}
}

// Calls returns how many Ask calls have been made so far.
func (b *Blocked) Calls() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.calls
}
