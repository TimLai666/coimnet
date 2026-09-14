package signal

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
)

// Clock advances an integer model step and maps it to an integer timestamp.
// CurrentStep is state carried by the value; Advance returns a new Clock.
type Clock struct {
	version     Version
	unit        TimeUnit
	stepSize    int64
	currentStep int64
}

func NewClock(version Version, unit TimeUnit, stepSize int64) (Clock, error) {
	if err := validateSchemaVersion(version); err != nil {
		return Clock{}, err
	}
	if err := unit.validate(); err != nil {
		return Clock{}, err
	}
	if stepSize <= 0 {
		return Clock{}, fmt.Errorf("step size must be positive")
	}
	return Clock{version: version, unit: unit, stepSize: stepSize}, nil
}

func (c Clock) Validate() error {
	if err := validateSchemaVersion(c.version); err != nil {
		return err
	}
	if err := c.unit.validate(); err != nil {
		return err
	}
	if c.stepSize <= 0 {
		return fmt.Errorf("step size must be positive")
	}
	if c.currentStep < 0 {
		return fmt.Errorf("current step must be non-negative")
	}
	if _, err := c.TimestampAt(c.currentStep); err != nil {
		return err
	}
	return nil
}

func (c Clock) SchemaVersion() Version { return c.version }
func (c Clock) Unit() TimeUnit         { return c.unit }
func (c Clock) StepSize() int64        { return c.stepSize }
func (c Clock) CurrentStep() int64     { return c.currentStep }

func (c Clock) TimestampAt(step int64) (Timestamp, error) {
	if err := c.ValidateStatic(); err != nil {
		return Timestamp{}, err
	}
	if step < 0 {
		return Timestamp{}, fmt.Errorf("step must be non-negative")
	}
	if step > math.MaxInt64/c.stepSize {
		return Timestamp{}, fmt.Errorf("step timestamp overflows int64")
	}
	return Timestamp{Value: step * c.stepSize, Unit: c.unit}, nil
}

// At is an alias for TimestampAt.
func (c Clock) At(step int64) (Timestamp, error) { return c.TimestampAt(step) }

func (c Clock) ValidateStatic() error {
	if err := validateSchemaVersion(c.version); err != nil {
		return err
	}
	if err := c.unit.validate(); err != nil {
		return err
	}
	if c.stepSize <= 0 {
		return fmt.Errorf("step size must be positive")
	}
	return nil
}

func (c Clock) Advance(steps int64) (Clock, Timestamp, error) {
	if err := c.Validate(); err != nil {
		return Clock{}, Timestamp{}, err
	}
	if steps < 0 {
		return Clock{}, Timestamp{}, fmt.Errorf("advance steps must be non-negative")
	}
	if steps > math.MaxInt64-c.currentStep {
		return Clock{}, Timestamp{}, fmt.Errorf("current step overflows int64")
	}
	next := c
	next.currentStep += steps
	timestamp, err := next.TimestampAt(next.currentStep)
	if err != nil {
		return Clock{}, Timestamp{}, err
	}
	return next, timestamp, nil
}

func (c Clock) StepFor(timestamp Timestamp) (int64, error) {
	if err := c.ValidateStatic(); err != nil {
		return 0, err
	}
	if err := timestamp.validate("timestamp"); err != nil {
		return 0, err
	}
	if timestamp.Unit != c.unit {
		return 0, fmt.Errorf("timestamp unit %q does not match clock unit %q", timestamp.Unit, c.unit)
	}
	if timestamp.Value%c.stepSize != 0 {
		return 0, fmt.Errorf("timestamp %d is not aligned to step size %d", timestamp.Value, c.stepSize)
	}
	return timestamp.Value / c.stepSize, nil
}

type clockJSON struct {
	SchemaVersion Version           `json:"schema_version"`
	Unit          TimeUnit          `json:"unit"`
	StepSize      jsonNumber[int64] `json:"step_size"`
	CurrentStep   jsonNumber[int64] `json:"current_step"`
}

func (c Clock) MarshalJSON() ([]byte, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(clockJSON{SchemaVersion: c.version, Unit: c.unit, StepSize: jsonNumber[int64]{c.stepSize}, CurrentStep: jsonNumber[int64]{c.currentStep}})
}

func (c *Clock) UnmarshalJSON(data []byte) error {
	var raw clockJSON
	if err := decodeStrictBytes(data, &raw); err != nil {
		return err
	}
	clock, err := NewClock(raw.SchemaVersion, raw.Unit, raw.StepSize.value)
	if err != nil {
		return err
	}
	if raw.CurrentStep.value < 0 {
		return fmt.Errorf("current step must be non-negative")
	}
	clock.currentStep = raw.CurrentStep.value
	if err := clock.Validate(); err != nil {
		return err
	}
	*c = clock
	return nil
}

func DecodeClock(data io.Reader) (Clock, error) {
	var c Clock
	if err := decodeStrict(data, &c); err != nil {
		return Clock{}, err
	}
	return c, nil
}
