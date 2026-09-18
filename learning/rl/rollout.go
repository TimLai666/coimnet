// Package rl provides policy-gradient and rollout machinery: transfer records
// of collected experience and advantage estimation over value functions.
package rl

// Transition is one step of one rollout as it was collected: the observation
// the policy saw, the action it took with the log-probability it assigned
// then, the value estimate, the reward and how the step ended.
type Transition struct {
	Obs     []float64 `json:"obs"`
	Action  int       `json:"action"`
	LogProb float64   `json:"log_prob"`
	Value   float64   `json:"value"`
	Reward  float64   `json:"reward"`
	// Done means the environment terminated after this step; Timeout means
	// the time limit cut the episode, which is not a terminal and bootstraps
	// from BootstrapValue, the V(s_{t+1}) the collector recorded then.
	Done           bool    `json:"done"`
	Timeout        bool    `json:"timeout"`
	BootstrapValue float64 `json:"bootstrap_value,omitempty"`
}
