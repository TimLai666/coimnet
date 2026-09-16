package signal

import "fmt"

// AvailableFeedback returns the feedback that has reached its declared arrival
// time at now, in the order it was given. Feedback whose AvailableAt is after
// now is dropped; feedback available exactly at now is kept, which is the same
// boundary Feedback.ValidateAvailableAt enforces.
//
// now and every feedback must use the same time unit: comparing an integer
// timestamp against a different unit would silently admit feedback that has
// not arrived, so a mismatch is an error rather than a drop. Feedback values
// carry no reference type, so the returned slice holds independent copies and
// the input slice is never read for writing, reordered or truncated.
func AvailableFeedback(all []Feedback, now Timestamp) ([]Feedback, error) {
	if err := now.validate("now"); err != nil {
		return nil, err
	}
	available := make([]Feedback, 0, len(all))
	for i, f := range all {
		if err := f.Validate(); err != nil {
			return nil, fmt.Errorf("feedback %d: %w", i, err)
		}
		if f.availableAt.Unit != now.Unit {
			return nil, fmt.Errorf("feedback %d uses time unit %q; now uses %q", i, f.availableAt.Unit, now.Unit)
		}
		if f.availableAt.Value <= now.Value {
			available = append(available, f)
		}
	}
	return available, nil
}
