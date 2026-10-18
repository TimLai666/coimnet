package learning

import (
	"sync/atomic"
	"unsafe"

	"github.com/HazelnutParadise/insyra/nn"
)

// tensorCache keeps the float32 tensor last built from one parameter slice, so
// a forward pass with unchanged encoder or readout values reuses it instead of
// converting and checking every value again. A hit requires the source values
// to be bitwise equal to the ones the tensor was built from, so an in-place
// change to the caller's slice is always seen, and a miss converts and checks
// exactly as tensor does. Passes only read the cached tensor; it is never
// handed out for writing. Safe for concurrent use.
type tensorCache struct {
	entry atomic.Pointer[cachedTensor]
}

type cachedTensor struct {
	rows, cols int
	source     []float64
	tensor     *nn.Tensor
}

func (c *tensorCache) get(rows, cols int, values []float64) (*nn.Tensor, error) {
	if e := c.entry.Load(); e != nil && e.rows == rows && e.cols == cols && sameBits(e.source, values) {
		return e.tensor, nil
	}
	t, err := tensor([]int{rows, cols}, values)
	if err != nil {
		return nil, err
	}
	c.entry.Store(&cachedTensor{rows: rows, cols: cols, source: append([]float64(nil), values...), tensor: t})
	return t, nil
}

// sameBits compares the stored bits, so -0 and 0 differ and a NaN equals the
// same NaN; either way the converted tensor would be the one a fresh tensor
// call builds or refuses.
func sameBits(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	if len(a) == 0 {
		return true
	}
	n := len(a) * 8
	return string(unsafe.Slice((*byte)(unsafe.Pointer(&a[0])), n)) == string(unsafe.Slice((*byte)(unsafe.Pointer(&b[0])), n))
}
