package dynamics

import "fmt"

// Modulation is the per-step, per-node adjustment a chemical layer hands to a
// core for the length of one call. Gain and Offset change the input current of
// a node, Threshold moves the effective threshold of the spiking core, and all
// three are indexed [step][node] over exactly the steps of that call.
//
// A modulation is temporary by construction: it is an argument, not state, and
// nothing here is ever written back into Parameters or LIFParameters.
//
// An array left nil is neutral: a nil Gain is 1 everywhere and a nil Offset or
// Threshold is 0 everywhere. The continuous core has no threshold to move, so
// it accepts a Threshold array only while every entry is zero.
type Modulation struct {
	Gain      [][]float64 `json:"gain,omitempty"`
	Offset    [][]float64 `json:"offset,omitempty"`
	Threshold [][]float64 `json:"threshold,omitempty"`
}

// validate rejects a modulation whose shape does not match the call or whose
// entries are not finite. spiking is false for the continuous core, where a
// threshold entry that is not zero is refused rather than ignored.
func (mod *Modulation) validate(steps, nodes int, spiking bool) error {
	if mod == nil {
		return nil
	}
	if err := modulationRows(mod.Gain, steps, nodes, "gain"); err != nil {
		return err
	}
	if err := modulationRows(mod.Offset, steps, nodes, "offset"); err != nil {
		return err
	}
	if err := modulationRows(mod.Threshold, steps, nodes, "threshold"); err != nil {
		return err
	}
	if !spiking {
		for t, row := range mod.Threshold {
			for i, v := range row {
				if v != 0 {
					return fmt.Errorf("modulation threshold[%d][%d] is %v, and the continuous core has no threshold to move", t, i, v)
				}
			}
		}
	}
	return nil
}

func modulationRows(rows [][]float64, steps, nodes int, name string) error {
	if rows == nil {
		return nil
	}
	if len(rows) != steps {
		return fmt.Errorf("modulation %s has %d steps, want %d", name, len(rows), steps)
	}
	for t, row := range rows {
		if err := vector(row, nodes, "modulation "+name); err != nil {
			return fmt.Errorf("%s[%d]: %w", name, t, err)
		}
	}
	return nil
}

// drives reports whether the call has to touch the input current at all.
func (mod *Modulation) drives() bool {
	return mod != nil && (mod.Gain != nil || mod.Offset != nil)
}

// shifts reports whether the call has to touch the effective threshold at all.
func (mod *Modulation) shifts() bool { return mod != nil && mod.Threshold != nil }

// modulatedDrive turns the input current of one node into gain*I + offset. A
// neutral pair is returned untouched rather than multiplied out, because
// (-0)*1 + 0 is a positive zero and would make a neutral modulation differ from
// no modulation in the sign of a zero.
func modulatedDrive(current float64, mod *Modulation, t, i int) float64 {
	gain, offset := 1.0, 0.0
	if mod.Gain != nil {
		gain = mod.Gain[t][i]
	}
	if mod.Offset != nil {
		offset = mod.Offset[t][i]
	}
	if gain == 1 && offset == 0 {
		return current
	}
	return gain*current + offset
}

// modulatedThreshold is max(theta + shift, low). A zero shift returns theta
// untouched: the floor would be a no-op there anyway, because the base
// threshold is already at or above theta_min and adaptation and homeostasis
// only raise it, and skipping it keeps an unmodulated step bit for bit intact.
func modulatedThreshold(theta float64, mod *Modulation, t, i int, low float64) float64 {
	shift := mod.Threshold[t][i]
	if shift == 0 {
		return theta
	}
	theta += shift
	if theta < low {
		return low
	}
	return theta
}
