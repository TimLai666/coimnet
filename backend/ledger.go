package backend

import (
	"fmt"
	"sync"
)

// Ledger serialises learning updates: Begin reserves the next sequence
// number, Commit applies it, Abort discards it. After an Abort the same
// number is reserved again by the next Begin, so a retried update is never
// counted twice and a half-applied one is never counted at all. Safe for
// concurrent use.
type Ledger struct {
	mu       sync.Mutex
	applied  uint64
	inflight uint64
	open     bool
}

// Applied returns the number of committed updates.
func (l *Ledger) Applied() uint64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.applied
}

// Begin reserves the next sequence number, applied+1, and returns it. An
// update already in flight is an error "update N still in flight".
func (l *Ledger) Begin() (uint64, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.open {
		return 0, fmt.Errorf("update %d still in flight", l.inflight)
	}
	l.inflight = l.applied + 1
	l.open = true
	return l.inflight, nil
}

// Commit applies the update identified by seq. The sequence number must be
// the one in flight; otherwise it is an error and applied is unchanged.
func (l *Ledger) Commit(seq uint64) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.open {
		return fmt.Errorf("update %d not in flight", seq)
	}
	if seq != l.inflight {
		return fmt.Errorf("update %d not in flight, expected %d", seq, l.inflight)
	}
	l.applied = seq
	l.open = false
	return nil
}

// Abort discards the update identified by seq. The checks are the same as
// Commit; on success applied is unchanged and the number is free to be
// reserved again.
func (l *Ledger) Abort(seq uint64) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.open {
		return fmt.Errorf("update %d not in flight", seq)
	}
	if seq != l.inflight {
		return fmt.Errorf("update %d not in flight, expected %d", seq, l.inflight)
	}
	l.open = false
	return nil
}
