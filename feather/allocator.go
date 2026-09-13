package feather

import (
	"fmt"
	"sync"

	"github.com/apache/arrow/go/v17/arrow/memory"
)

// boundedAllocator counts bytes requested through Arrow's allocator. It is a
// process-local accounting limit, not a limit on the process's complete RSS.
// Reallocation reserves the new buffer while the old one is still accounted,
// so the peak includes the temporary double allocation.
type boundedAllocator struct {
	base  memory.Allocator
	limit int64

	mu          sync.Mutex
	current     int64
	peak        int64
	allocations map[*byte]int64
	err         error
}

func newBoundedAllocator(limit int64) *boundedAllocator {
	return &boundedAllocator{
		base:        memory.NewGoAllocator(),
		limit:       limit,
		allocations: make(map[*byte]int64),
	}
}

func (a *boundedAllocator) Allocate(size int) []byte {
	if size < 0 {
		a.setError(fmt.Errorf("feather: Arrow allocator received negative allocation size %d", size))
		return nil
	}
	size64 := int64(size)
	a.mu.Lock()
	if !a.reserveLocked(size64) {
		a.mu.Unlock()
		return nil
	}
	a.mu.Unlock()

	buf := a.base.Allocate(size)
	if size != 0 {
		a.mu.Lock()
		a.allocations[bufferKey(buf)] = size64
		a.mu.Unlock()
	}
	return buf
}

func (a *boundedAllocator) Reallocate(size int, buf []byte) []byte {
	if size < 0 {
		a.setError(fmt.Errorf("feather: Arrow allocator received negative reallocation size %d", size))
		return nil
	}
	if cap(buf) >= size {
		return a.base.Reallocate(size, buf)
	}

	oldKey := bufferKey(buf)
	oldSize := int64(cap(buf))
	a.mu.Lock()
	if known, ok := a.allocations[oldKey]; ok {
		oldSize = known
	}
	if !a.reserveLocked(int64(size)) {
		a.mu.Unlock()
		return nil
	}
	// Keep the old allocation in current until the underlying realloc has
	// completed. This is the temporary double-buffer peak required by Scan.
	defer a.mu.Unlock()
	out := a.base.Reallocate(size, buf)
	a.current -= oldSize
	delete(a.allocations, oldKey)
	if size != 0 {
		a.allocations[bufferKey(out)] = int64(size)
	}
	return out
}

func (a *boundedAllocator) Free(buf []byte) {
	key := bufferKey(buf)
	a.mu.Lock()
	if size, ok := a.allocations[key]; ok {
		delete(a.allocations, key)
		a.current -= size
	}
	a.mu.Unlock()
	a.base.Free(buf)
}

func (a *boundedAllocator) Peak() int64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.peak
}

func (a *boundedAllocator) Error() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.err
}

func (a *boundedAllocator) reserveLocked(size int64) bool {
	if size > a.limit-a.current {
		if a.err == nil {
			a.err = fmt.Errorf("feather: Arrow allocation limit exceeded: requested=%d current=%d limit=%d", size, a.current, a.limit)
		}
		return false
	}
	a.current += size
	if a.current > a.peak {
		a.peak = a.current
	}
	return true
}

func (a *boundedAllocator) setError(err error) {
	a.mu.Lock()
	if a.err == nil {
		a.err = err
	}
	a.mu.Unlock()
}

func bufferKey(buf []byte) *byte {
	if cap(buf) == 0 {
		return nil
	}
	return &buf[:1][0]
}

var _ memory.Allocator = (*boundedAllocator)(nil)
