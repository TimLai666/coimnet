package learning

import (
	"fmt"
	"math"
)

// ParameterSharing ties entries of the core parameter arrays into groups. An
// entry with group index g >= 0 shares one value with every other entry of the
// same g in the same array family; -1 keeps the entry individual. An empty
// slice means no sharing for that array. Group indices are in [0, Groups).
type ParameterSharing struct {
	Weights []int32 `json:"weights,omitempty"`
	Bias    []int32 `json:"bias,omitempty"`
	LogTau  []int32 `json:"log_tau,omitempty"`
	Groups  int     `json:"groups"`
}

// Validate checks lengths against the core, index range, that every group index
// in [0, Groups) is used, and that one group never spans two array families (a
// group lives in Weights, Bias or LogTau, not across them). weights, bias and
// logTau are the core's counts of the three families.
func (s ParameterSharing) Validate(weights, bias, logTau int) error {
	if s.Groups < 0 {
		return fmt.Errorf("share groups is %d, cannot be negative", s.Groups)
	}
	if len(s.Weights) != 0 && len(s.Weights) != weights {
		return fmt.Errorf("share weights has %d entries, the core has %d weight values", len(s.Weights), weights)
	}
	if len(s.Bias) != 0 && len(s.Bias) != bias {
		return fmt.Errorf("share bias has %d entries, the core has %d bias values", len(s.Bias), bias)
	}
	if len(s.LogTau) != 0 && len(s.LogTau) != logTau {
		return fmt.Errorf("share log_tau has %d entries, the core has %d log_tau values", len(s.LogTau), logTau)
	}
	want := "-1"
	if s.Groups > 0 {
		want = fmt.Sprintf("-1 or 0..%d", s.Groups-1)
	}
	owner := make(map[int]string)
	used := make([]bool, s.Groups)
	for _, family := range []struct {
		name    string
		indices []int32
	}{
		{"weights", s.Weights}, {"bias", s.Bias}, {"log_tau", s.LogTau},
	} {
		for i, g := range family.indices {
			if g < -1 || g >= int32(s.Groups) {
				return fmt.Errorf("share %s[%d] is group %d, want %s", family.name, i, g, want)
			}
			if g < 0 {
				continue
			}
			if previous, ok := owner[int(g)]; ok && previous != family.name {
				return fmt.Errorf("share group %d spans the %s and %s parameter arrays", g, previous, family.name)
			}
			owner[int(g)] = family.name
			used[int(g)] = true
		}
	}
	for g := 0; g < s.Groups; g++ {
		if !used[g] {
			return fmt.Errorf("share group %d has no members", g)
		}
	}
	return nil
}

// SharingReport is returned by the trainer: Conflicts counts groups where a mask
// froze some members and not others (the whole group is then frozen, which is
// the rule).
type SharingReport struct {
	Groups    int `json:"groups"`
	Members   int `json:"members"`
	Conflicts int `json:"conflicts"`
}

// sharePlan expands a sharing declaration over the flat parameter order
// weights, bias, log_tau, the three families sharing can touch. Every group of
// the declaration is one slice of flat indices; groupOf maps a flat index back
// to its group, or -1 for an individual entry. Entries beyond the core families
// (theta_raw, encoder, readout) are never covered and stay individual.
type sharePlan struct {
	groups  [][]int
	groupOf []int
	members int
}

// newSharePlan builds the plan from a validated declaration and the current
// family sizes of the parameter vector.
func newSharePlan(s *ParameterSharing, weights, bias, logTau int) *sharePlan {
	plan := &sharePlan{groupOf: make([]int, weights+bias+logTau), groups: make([][]int, s.Groups)}
	for i := range plan.groupOf {
		plan.groupOf[i] = -1
	}
	for _, family := range []struct {
		indices []int32
		offset  int
	}{
		{s.Weights, 0}, {s.Bias, weights}, {s.LogTau, weights + bias},
	} {
		for i, g := range family.indices {
			index := family.offset + i
			plan.groupOf[index] = int(g)
			if g >= 0 {
				plan.groups[int(g)] = append(plan.groups[int(g)], index)
				plan.members++
			}
		}
	}
	return plan
}

// reduceSharedGradients replaces the gradient of every member of a group by
// that group's summed member gradients (main spec 9.2 sums shared parameters,
// it does not average). After the reduction every member carries the same
// value, so AdamW applies the same gradient, the same momentum and the same
// update to every member and the parameters stay bit-identical.
func reduceSharedGradients(grad []float64, plan *sharePlan) {
	for _, members := range plan.groups {
		if len(members) < 2 {
			continue
		}
		total := 0.0
		for _, m := range members {
			total += grad[m]
		}
		for _, m := range members {
			grad[m] = total
		}
	}
}

// sharedMaskFreeze applies the group rule on top of the per-item mask: any one
// frozen member freezes the whole group. A group whose mask is mixed (some
// members frozen, some open) is counted as one conflict. The returned mask is a
// fresh copy; the caller's slice is not modified.
func sharedMaskFreeze(mask []bool, plan *sharePlan) ([]bool, int) {
	out := append([]bool(nil), mask...)
	conflicts := 0
	for _, members := range plan.groups {
		frozen, open := 0, 0
		for _, m := range members {
			if out[m] {
				open++
			} else {
				frozen++
			}
		}
		if frozen == 0 || open == 0 {
			continue
		}
		conflicts++
		for _, m := range members {
			out[m] = false
		}
	}
	return out, conflicts
}

// reducedNorm measures the norm over the reduced parameter vector: each group
// contributes exactly once, through its summed member value, and every
// individual entry counts once. Masked entries and fully frozen groups
// contribute nothing, so a shared parameter can never inflate the norm by
// repeating itself.
func reducedNorm(v []float64, mask []bool, plan *sharePlan) float64 {
	norm := 0.0
	counted := make([]bool, len(plan.groups))
	for i := range v {
		if !mask[i] {
			continue
		}
		if i < len(plan.groupOf) {
			if g := plan.groupOf[i]; g >= 0 {
				if counted[g] {
					continue
				}
				counted[g] = true
			}
		}
		norm = math.Hypot(norm, v[i])
	}
	return norm
}

// validateSharingParameters checks the trainer-level sharing rules against the
// parameters and the sign declaration: members of one group must start
// bit-identical, and a weight group must never mix edge signs. The parameter
// shape has already been validated, and a vector-state model with fixed signs
// has already been refused, before this runs.
func validateSharingParameters(s *ParameterSharing, p Parameters, c Config, core coreModel) error {
	plan := newSharePlan(s, len(p.Core.Weights), len(p.Core.Bias), len(p.Core.LogTau))
	flat := flatParameters(p)
	for _, members := range plan.groups {
		first := flat[members[0]]
		for _, m := range members[1:] {
			if flat[m] != first {
				return fmt.Errorf("shared parameters must start equal")
			}
		}
	}
	if len(c.EdgeSigns) == 0 {
		return nil
	}
	span := 1
	if core.matrixEdges() {
		span = core.stateDim() * core.stateDim()
	}
	for _, members := range plan.groups {
		if members[0] >= len(p.Core.Weights) {
			continue
		}
		first := c.EdgeSigns[members[0]/span]
		for _, m := range members[1:] {
			if c.EdgeSigns[m/span] != first {
				return fmt.Errorf("a shared group mixes edge signs")
			}
		}
	}
	return nil
}

// copySharing deep copies a sharing declaration so a network never aliases
// caller-owned slices.
func copySharing(s *ParameterSharing) *ParameterSharing {
	if s == nil {
		return nil
	}
	owned := *s
	owned.Weights = append([]int32(nil), s.Weights...)
	owned.Bias = append([]int32(nil), s.Bias...)
	owned.LogTau = append([]int32(nil), s.LogTau...)
	return &owned
}

// Sharing reports the sharing declaration the trainer was built with: how many
// groups, how many members across them, and how many groups a per-item mask
// froze only partially (those groups are fully frozen). A trainer without a
// sharing declaration returns an all-zero report.
func (tr *Trainer) Sharing() SharingReport {
	if tr == nil || tr.network == nil || tr.network.config.Sharing == nil {
		return SharingReport{}
	}
	tr.mu.Lock()
	defer tr.mu.Unlock()
	plan := newSharePlan(tr.network.config.Sharing, len(tr.parameters.Core.Weights), len(tr.parameters.Core.Bias), len(tr.parameters.Core.LogTau))
	report := SharingReport{Groups: len(plan.groups), Members: plan.members}
	mask := parameterMask(tr.parameters, tr.options, tr.network.core.thetaNodes(), tr.network.core)
	_, report.Conflicts = sharedMaskFreeze(mask, plan)
	return report
}
