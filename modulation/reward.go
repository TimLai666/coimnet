package modulation

import "fmt"

// Reward baselines and mapping kinds. Both are declared strings so that a
// report names the rule that produced a release rate.
const (
	// BaselineNone expects zero: the raw value is used as it arrives.
	BaselineNone = "none"
	// BaselineRunningMean expects the mean of the previous Window raw values,
	// this one excluded. Fewer than Window values means the mean of the values
	// there are, and the first value has no history and expects zero.
	BaselineRunningMean = "running_mean"

	// KindRelu applies the positive part of delta on one channel and reports
	// the negative part as a clearance boost, which the concentration layer
	// consumes as an extra clearance term.
	KindRelu = "relu"
	// KindSplit applies the positive part of delta on the first channel and
	// the negative part on the second. Nothing is left for clearance.
	KindSplit = "split"
)

// RewardRecord keeps the four values of one mapping apart. Raw is the value the
// environment produced, untouched. Expected is the baseline it was compared
// against. Transformed is delta = Raw - Expected, sign included. Applied is the
// non-negative release per channel, and ClearanceBoost is the non-negative
// clearance term the relu kind produces instead of a negative release.
type RewardRecord struct {
	Raw            float64   `json:"raw"`
	Expected       float64   `json:"expected"`
	Transformed    float64   `json:"transformed"`
	Applied        []float64 `json:"applied"`
	ClearanceBoost float64   `json:"clearance_boost"`
}

// RewardMapper maps a raw reward or punishment onto non-negative modulation
// input by a declared rule. It never rewrites the raw value: the model path
// sees Applied, and Raw is copied into the record exactly as it arrived.
//
// The mapper owns its baseline history; a record it handed out earlier shares
// nothing with it, and writing into a record cannot move the baseline.
type RewardMapper struct {
	Baseline string `json:"baseline"`
	Window   int    `json:"window"`
	Kind     string `json:"kind"`

	history []float64
}

func (m *RewardMapper) validate() error {
	switch m.Baseline {
	case BaselineNone:
		// A window that is never read would be a second knob with no effect.
		if m.Window != 0 {
			return fmt.Errorf("modulation: baseline %q declares window %d, which it never uses", m.Baseline, m.Window)
		}
	case BaselineRunningMean:
		if m.Window < 1 {
			return fmt.Errorf("modulation: baseline %q needs a window of at least 1, got %d", m.Baseline, m.Window)
		}
	default:
		return fmt.Errorf("modulation: unsupported reward baseline %q", m.Baseline)
	}
	switch m.Kind {
	case KindRelu, KindSplit:
	default:
		return fmt.Errorf("modulation: unsupported reward mapping kind %q", m.Kind)
	}
	return nil
}

// Map maps one raw value. A non-finite raw value is rejected and does not enter
// the baseline history.
func (m *RewardMapper) Map(raw float64) (RewardRecord, error) {
	if m == nil {
		return RewardRecord{}, fmt.Errorf("modulation: nil reward mapper")
	}
	if err := m.validate(); err != nil {
		return RewardRecord{}, err
	}
	if !finite(raw) {
		return RewardRecord{}, fmt.Errorf("modulation: raw reward must be finite")
	}
	record := RewardRecord{Raw: raw, Expected: m.expected()}
	record.Transformed = raw - record.Expected
	if !finite(record.Transformed) {
		return RewardRecord{}, fmt.Errorf("modulation: raw reward %v minus baseline %v is not finite", raw, record.Expected)
	}
	switch m.Kind {
	case KindRelu:
		record.Applied = []float64{nonNegative(record.Transformed)}
		record.ClearanceBoost = nonNegative(-record.Transformed)
	case KindSplit:
		record.Applied = []float64{nonNegative(record.Transformed), nonNegative(-record.Transformed)}
	}
	if _, err := checkedRelease(record.Applied, len(record.Applied)); err != nil {
		return RewardRecord{}, err
	}
	m.remember(raw)
	return record, nil
}

func (m *RewardMapper) expected() float64 {
	if m.Baseline != BaselineRunningMean || len(m.history) == 0 {
		return 0
	}
	var sum float64
	for _, v := range m.history {
		sum += v
	}
	return sum / float64(len(m.history))
}

// remember keeps at most Window raw values, the mapper's own copy of them.
func (m *RewardMapper) remember(raw float64) {
	if m.Baseline != BaselineRunningMean {
		return
	}
	m.history = append(m.history, raw)
	if len(m.history) > m.Window {
		m.history = append(m.history[:0], m.history[len(m.history)-m.Window:]...)
	}
}
