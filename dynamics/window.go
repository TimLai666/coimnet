package dynamics

import "fmt"

// WindowStart is the first step of the truncation window that holds step t.
// Window zero keeps the whole sequence, so every step belongs to the window
// starting at 0. A positive window detaches at the boundaries offset,
// offset+window, offset+2*window, ...; the steps before offset form one
// shorter leading window that starts at 0. Offset zero is the start-aligned
// layout 0, window, 2*window, ...
func WindowStart(t, window, offset int) int {
	if window <= 0 || t < offset {
		return 0
	}
	return offset + (t-offset)/window*window
}

// checkWindowOffset refuses an offset that names no boundary: it must be zero
// without truncation and inside [0, window) with it.
func checkWindowOffset(window, offset int) error {
	if offset < 0 || (window == 0 && offset != 0) || (window > 0 && offset >= window) {
		return fmt.Errorf("truncation offset %d must be in [0, window %d)", offset, window)
	}
	return nil
}

// EndAlignedOffset is the window offset that makes the last truncation window
// end exactly at the final step of a steps-long sequence, so a loss on the
// final row reaches back a full window. It is zero without truncation.
func EndAlignedOffset(steps, window int) int {
	if window <= 0 || steps <= 0 {
		return 0
	}
	return steps % window
}
