package modulation

import (
	"fmt"
	"strings"

	"github.com/TimLai666/coimnet/simulate"
)

var (
	_ Source = ExternalTimeline{}
	_ Source = NeuralActivity{}
	_ Source = InternalResource{}
	_ Source = Replay{}
)

// TimelineEntry is one declared release: Rate on Channel at Step.
type TimelineEntry struct {
	Step    uint64  `json:"step"`
	Channel int     `json:"channel"`
	Rate    float64 `json:"rate"`
}

// ExternalTimeline releases a rate the experimenter declared in advance, such
// as an injected drug or an optogenetic pulse train. A step with no entry
// releases zero on every channel, which is a declaration and not a gap.
type ExternalTimeline struct {
	ChannelCount int             `json:"channel_count"`
	Entries      []TimelineEntry `json:"entries"`
}

func (t ExternalTimeline) Channels() int { return t.ChannelCount }

// Validate checks the declaration itself: the steps do not go backwards, every
// channel exists, every rate is finite and non-negative, and no step declares
// the same channel twice, because two rates for one channel is a declaration
// the framework must not silently resolve.
func (t ExternalTimeline) Validate() error {
	if t.ChannelCount <= 0 {
		return fmt.Errorf("modulation: external timeline declares %d channels", t.ChannelCount)
	}
	for i, e := range t.Entries {
		if i > 0 && e.Step < t.Entries[i-1].Step {
			return fmt.Errorf("modulation: external timeline entry %d is at step %d, before entry %d at step %d", i, e.Step, i-1, t.Entries[i-1].Step)
		}
		if e.Channel < 0 || e.Channel >= t.ChannelCount {
			return fmt.Errorf("modulation: external timeline entry %d is on channel %d, outside [0,%d)", i, e.Channel, t.ChannelCount)
		}
		if !finite(e.Rate) {
			return fmt.Errorf("modulation: external timeline entry %d declares a non-finite rate", i)
		}
		if e.Rate < 0 {
			return fmt.Errorf("modulation: external timeline entry %d declares a negative rate %v", i, e.Rate)
		}
		// Entries are non-decreasing in step, so a repeat of the same step is
		// adjacent and this scan stays linear in the entries of one step.
		for j := i - 1; j >= 0 && t.Entries[j].Step == e.Step; j-- {
			if t.Entries[j].Channel == e.Channel {
				return fmt.Errorf("modulation: external timeline declares step %d channel %d twice", e.Step, e.Channel)
			}
		}
	}
	return nil
}

func (t ExternalTimeline) Release(step uint64, c SourceContext) ([]float64, error) {
	if err := t.Validate(); err != nil {
		return nil, err
	}
	if err := c.checkFeedback(step); err != nil {
		return nil, err
	}
	rates := make([]float64, t.ChannelCount)
	for _, e := range t.Entries {
		if e.Step == step {
			rates[e.Channel] = e.Rate
		}
	}
	return checkedRelease(rates, t.ChannelCount)
}

// NeuralActivity releases in proportion to the mean activity of a named set of
// neurons. The set must be an explicit resolution: a modulatory population is
// named by the researcher and is never inferred here, so an unresolved or empty
// set is an error rather than a guess. Channels is Channel+1: the release
// vector is indexed by channel and every channel below the declared one
// releases zero.
type NeuralActivity struct {
	Set     simulate.ResolvedSet `json:"set"`
	Gain    float64              `json:"gain"`
	Channel int                  `json:"channel"`
}

func (n NeuralActivity) Channels() int { return n.Channel + 1 }

func (n NeuralActivity) Release(step uint64, c SourceContext) ([]float64, error) {
	if n.Channel < 0 {
		return nil, fmt.Errorf("modulation: neural source declares channel %d", n.Channel)
	}
	if !finite(n.Gain) {
		return nil, fmt.Errorf("modulation: neural source declares a non-finite gain")
	}
	nodes := n.Set.Nodes()
	if len(nodes) == 0 {
		return nil, fmt.Errorf("modulation: neural source needs an explicitly resolved, non-empty named set; %q resolved to %d nodes", n.Set.Name, len(nodes))
	}
	if err := c.checkFeedback(step); err != nil {
		return nil, err
	}
	if c.Activity == nil {
		return nil, fmt.Errorf("modulation: neural source has no activity for step %d", step)
	}
	var sum float64
	for _, node := range nodes {
		if node < 0 || node >= len(c.Activity) {
			return nil, fmt.Errorf("modulation: node %d of set %q is outside the %d activity values of step %d", node, n.Set.Name, len(c.Activity), step)
		}
		if !finite(c.Activity[node]) {
			return nil, fmt.Errorf("modulation: activity of node %d is not finite at step %d", node, step)
		}
		sum += c.Activity[node]
	}
	rates := make([]float64, n.Channels())
	rates[n.Channel] = nonNegative(sum / float64(len(nodes)) * n.Gain)
	return checkedRelease(rates, n.Channels())
}

// InternalResource releases from a resource the task declares, by the fixed
// rule Coefficient * max(Resources[Resource] - Threshold, 0). The rule is
// declared, not supplied as a function, so a report can state it and a reader
// can recompute it. A missing resource is an error: a source is never enabled
// against a resource the task did not declare.
type InternalResource struct {
	Resource    string  `json:"resource"`
	Coefficient float64 `json:"coefficient"`
	Threshold   float64 `json:"threshold"`
	Channel     int     `json:"channel"`
}

func (r InternalResource) Channels() int { return r.Channel + 1 }

func (r InternalResource) Release(step uint64, c SourceContext) ([]float64, error) {
	if strings.TrimSpace(r.Resource) == "" {
		return nil, fmt.Errorf("modulation: internal resource source names no resource")
	}
	if r.Channel < 0 {
		return nil, fmt.Errorf("modulation: internal resource source declares channel %d", r.Channel)
	}
	if !finite(r.Coefficient) || r.Coefficient < 0 {
		return nil, fmt.Errorf("modulation: internal resource source declares coefficient %v", r.Coefficient)
	}
	if !finite(r.Threshold) {
		return nil, fmt.Errorf("modulation: internal resource source declares a non-finite threshold")
	}
	if err := c.checkFeedback(step); err != nil {
		return nil, err
	}
	amount, declared := c.Resources[r.Resource]
	if !declared {
		return nil, fmt.Errorf("modulation: resource %q is not declared at step %d", r.Resource, step)
	}
	if !finite(amount) {
		return nil, fmt.Errorf("modulation: resource %q is not finite at step %d", r.Resource, step)
	}
	rates := make([]float64, r.Channels())
	rates[r.Channel] = nonNegative(r.Coefficient * nonNegative(amount-r.Threshold))
	return checkedRelease(rates, r.Channels())
}

// Replay replays a recorded release trace, one row per step. It holds the whole
// recording and nothing else: it has no reader, no path, no clock and no client
// of any kind, so replaying cannot reach outside the declared trace.
type Replay struct {
	Trace [][]float64 `json:"trace"`
}

func (r Replay) Channels() int {
	if len(r.Trace) == 0 {
		return 0
	}
	return len(r.Trace[0])
}

func (r Replay) Release(step uint64, c SourceContext) ([]float64, error) {
	if err := r.validate(); err != nil {
		return nil, err
	}
	if err := c.checkFeedback(step); err != nil {
		return nil, err
	}
	if step >= uint64(len(r.Trace)) {
		return nil, fmt.Errorf("modulation: replay trace has %d steps, cannot release step %d", len(r.Trace), step)
	}
	row := append([]float64(nil), r.Trace[step]...)
	return checkedRelease(row, r.Channels())
}

func (r Replay) validate() error {
	if len(r.Trace) == 0 {
		return fmt.Errorf("modulation: replay source has no recorded trace")
	}
	width := len(r.Trace[0])
	if width == 0 {
		return fmt.Errorf("modulation: replay trace declares no channel")
	}
	for i, row := range r.Trace {
		if len(row) != width {
			return fmt.Errorf("modulation: replay trace row %d has %d channels, row 0 has %d", i, len(row), width)
		}
		for k, v := range row {
			if !finite(v) {
				return fmt.Errorf("modulation: replay trace row %d channel %d is not finite", i, k)
			}
			if v < 0 {
				return fmt.Errorf("modulation: replay trace row %d channel %d is %v", i, k, v)
			}
		}
	}
	return nil
}
