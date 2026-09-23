// Package studenteval evaluates distilled students with the teacher removed:
// it trains fixture students through the distill package's step on
// DelayedEpisode streams, scores them on held-out and independent splits, and
// reports how much the evaluation consulted a teacher (student mode: never).
package studenteval

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"math"

	"github.com/TimLai666/coimnet/distill"
	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/experiment"
	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/teacher"
)

// SchemaVersion identifies the student evaluation report format.
const SchemaVersion = "coimnet-student-evaluation/v1"

// ModeStudent removes the teacher completely: the evaluation period holds a
// teacher.Blocked that any Ask would hit, and every call count must stay 0.
const ModeStudent = "student"

// ModeTeacherAssisted keeps the teacher in the evaluation loop: every held-out
// and independent input is asked once and the assisted prediction is reported
// separately from the student's own scores, so a report is never taken for the
// student's independent capability.
const ModeTeacherAssisted = "teacher_assisted"

// fixtureVocabHash names the fixture's two-class vocabulary; the distiller
// declares it on both sides of the identical alignment.
const fixtureVocabHash = "coimnet-student-eval-fixture-v1"

// Config is one student-evaluation protocol: the seeds, the distillation
// budget, the two evaluation splits, the mislabel robustness group, and the
// distiller and optimiser parameters every seed runs under.
type Config struct {
	Mode            string   `json:"mode"`
	Seeds           []uint64 `json:"seeds"`            // ≥ 1, distinct
	Episodes        int      `json:"episodes"`         // ≥ 1; training set experiment.DelayedEpisode(1001+seed, e)
	HeldOut         int      `json:"held_out"`         // ≥ 1; held-out set DelayedEpisode(1003+seed, i)
	Independent     int      `json:"independent"`      // ≥ 1; independent-source set DelayedEpisode(2003+seed, i)
	CorruptFraction float64  `json:"corrupt_fraction"` // [0, 0.5]; robustness group flips the first round(CorruptFraction*Episodes) teacher labels, then re-distills a fresh student
	Temperature     float64  `json:"temperature"`      // > 0
	Mix             float64  `json:"mix"`              // [0, 1]
	LearningRate    float64  `json:"learning_rate"`    // > 0
}

// Validate checks the protocol without running anything.
func (c Config) Validate() error {
	switch c.Mode {
	case ModeStudent, ModeTeacherAssisted:
	default:
		return fmt.Errorf("student evaluation mode %q, want %q or %q", c.Mode, ModeStudent, ModeTeacherAssisted)
	}
	if len(c.Seeds) == 0 {
		return errors.New("student evaluation needs at least one seed")
	}
	seen := map[uint64]bool{}
	for _, s := range c.Seeds {
		if seen[s] {
			return fmt.Errorf("student evaluation declares duplicate seed %d", s)
		}
		seen[s] = true
	}
	if c.Episodes < 1 {
		return fmt.Errorf("student evaluation episodes %d, want a value >= 1", c.Episodes)
	}
	if c.HeldOut < 1 {
		return fmt.Errorf("student evaluation held_out %d, want a value >= 1", c.HeldOut)
	}
	if c.Independent < 1 {
		return fmt.Errorf("student evaluation independent %d, want a value >= 1", c.Independent)
	}
	if !finite(c.CorruptFraction) || c.CorruptFraction < 0 || c.CorruptFraction > 0.5 {
		return fmt.Errorf("student evaluation corrupt_fraction %v, want a value in [0, 0.5]", c.CorruptFraction)
	}
	if !finite(c.Temperature) || c.Temperature <= 0 {
		return fmt.Errorf("student evaluation temperature %v, want a positive finite value", c.Temperature)
	}
	if !finite(c.Mix) || c.Mix < 0 || c.Mix > 1 {
		return fmt.Errorf("student evaluation mix %v, want a value in [0, 1]", c.Mix)
	}
	if !finite(c.LearningRate) || c.LearningRate <= 0 {
		return fmt.Errorf("student evaluation learning_rate %v, want a positive finite value", c.LearningRate)
	}
	return nil
}

// Seed is one seed's report: how many teacher calls the evaluation made, how
// much the student agrees with the fixture oracle on each split, how well it
// scores against the real pulse label, and what the mislabeled-robustness
// group cost. A failed seed keeps its index and reason.
type Seed struct {
	Seed                         uint64   `json:"seed"`
	TeacherCalls                 int      `json:"teacher_calls"`                             // Blocked.Calls plus the passed teacher's own Calls counter; must be 0 in student mode
	HeldOutAgreement             float64  `json:"held_out_agreement"`                        // student argmax vs oracle teacher argmax on the held-out split
	HeldOutTaskScore             float64  `json:"held_out_task_score"`                       // student argmax vs pulse label on the held-out split
	IndependentAgreement         float64  `json:"independent_agreement"`                     // like HeldOutAgreement on the independent split
	IndependentTaskScore         float64  `json:"independent_task_score"`                    // like HeldOutTaskScore on the independent split
	CorruptedIndependentScore    float64  `json:"corrupted_independent_task_score"`          // independent task score of the mislabel-distilled student
	RobustnessDelta              float64  `json:"robustness_delta"`                          // IndependentTaskScore − CorruptedIndependentScore
	AssistedHeldOutTaskScore     *float64 `json:"assisted_held_out_task_score,omitempty"`    // held-out task score of the teacher-assisted prediction; absent in student mode
	AssistedIndependentTaskScore *float64 `json:"assisted_independent_task_score,omitempty"` // like AssistedHeldOutTaskScore on the independent split
	Fallbacks                    *int     `json:"fallbacks,omitempty"`                       // inputs where the teacher could not answer and the student's own argmax stood in; absent in student mode
	Failed                       bool     `json:"failed"`
	Error                        string   `json:"error,omitempty"`
}

// Report separates the shared protocol from the per-seed runs and states what
// the numbers may and may not mean.
type Report struct {
	SchemaVersion  string   `json:"schema_version"`
	Mode           string   `json:"mode"`
	Config         Config   `json:"config"`
	ConfigHash     string   `json:"config_hash"`     // hex sha256 of json.Marshal(Config)
	TeacherBlocked bool     `json:"teacher_blocked"` // true in student mode
	Seeds          []Seed   `json:"seeds"`
	Assumptions    []string `json:"assumptions"` // fixed caveats attached to every report
}

// teacherCallCounter is implemented by teachers that keep their own Ask count;
// Run adds that count to Blocked.Calls when building TeacherCalls.
type teacherCallCounter interface {
	Calls() int
}

// TrainStudent distills one fresh fixture student on the training split
// DelayedEpisode(1001+seed, ·) under config c, using the same fixture,
// distiller, teacher distribution and episode stream as Run. It exists so a
// caller can rebuild Run's student beside a direct distill.StepDistribution
// loop and compare the two Predict results.
func TrainStudent(ctx context.Context, c Config, seed uint64) (*learning.Trainer, error) {
	if ctx == nil {
		return nil, errors.New("studenteval: nil context")
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return trainStudent(ctx, c, seed, 0)
}

// Run evaluates a distilled student under c with the teacher removed.
//
// In student mode Run never consults t's answering ability: the distillation
// target is the fixture oracle distribution (no Ask), and the evaluation
// period holds a teacher.Blocked that is never asked, so TeacherBlocked is
// true and TeacherCalls — Blocked.Calls plus t's own Calls counter when t
// keeps one — must come out 0. In teacher_assisted mode training is identical
// but every held-out and independent input is asked once: the assisted
// prediction (the teacher's answer, falling back to the student's own argmax)
// is reported separately and each seed's TeacherCalls counts the asks actually
// made. A canceled context aborts the whole run; a failed seed is recorded
// with its reason while the remaining seeds continue.
func Run(ctx context.Context, c Config, t teacher.Teacher) (Report, error) {
	var report Report
	if ctx == nil {
		return report, errors.New("studenteval: nil context")
	}
	if err := ctx.Err(); err != nil {
		return report, err
	}
	if err := c.Validate(); err != nil {
		return report, err
	}
	if c.Mode == ModeTeacherAssisted && t == nil {
		return report, errors.New("studenteval: teacher_assisted requires a teacher")
	}
	report = Report{
		SchemaVersion:  SchemaVersion,
		Mode:           c.Mode,
		Config:         c,
		ConfigHash:     configHash(c),
		TeacherBlocked: c.Mode == ModeStudent,
		Assumptions:    assumptions(c.Mode),
	}
	blocked := &teacher.Blocked{ID: "coimnet-student-evaluation", Version: SchemaVersion}
	for _, s := range c.Seeds {
		seed, err := runSeed(ctx, s, c, t)
		if err != nil {
			if ctx.Err() != nil {
				return report, ctx.Err()
			}
			seed.Failed, seed.Error = true, err.Error()
		}
		seed.Seed = s
		if c.Mode != ModeTeacherAssisted {
			seed.TeacherCalls = blocked.Calls() + callsOf(t)
		}
		report.Seeds = append(report.Seeds, seed)
	}
	return report, nil
}

// runSeed distills one fresh student, scores it on the held-out and
// independent splits, then re-distills on the flipped-label prefix and reports
// what the corruption cost on the independent split. In teacher_assisted mode
// the student's own scores still come from the same Predict-only evaluation,
// while a separate assisted evaluation asks the teacher once per input.
func runSeed(ctx context.Context, seed uint64, c Config, t teacher.Teacher) (Seed, error) {
	var out Seed
	tr, err := trainStudent(ctx, c, seed, 0)
	if err != nil {
		return out, err
	}
	var evalTeacher teacher.Teacher
	if c.Mode == ModeTeacherAssisted {
		evalTeacher = t
	}
	held, err := evaluateSet(ctx, tr, evalTeacher, seed, 1003+seed, c.HeldOut)
	out.TeacherCalls += held.Asks
	if err != nil {
		return out, err
	}
	indep, err := evaluateSet(ctx, tr, evalTeacher, seed, 2003+seed, c.Independent)
	out.TeacherCalls += indep.Asks
	if err != nil {
		return out, err
	}
	out.HeldOutAgreement = held.Agreement
	out.HeldOutTaskScore = held.TaskScore
	out.IndependentAgreement = indep.Agreement
	out.IndependentTaskScore = indep.TaskScore
	if evalTeacher != nil {
		assistedHeldOut := held.AssistedScore
		assistedIndependent := indep.AssistedScore
		fallbacks := held.Fallbacks + indep.Fallbacks
		out.AssistedHeldOutTaskScore = &assistedHeldOut
		out.AssistedIndependentTaskScore = &assistedIndependent
		out.Fallbacks = &fallbacks
	}
	if corrupt := int(math.Round(c.CorruptFraction * float64(c.Episodes))); corrupt > 0 {
		rtr, err := trainStudent(ctx, c, seed, corrupt)
		if err != nil {
			return out, err
		}
		corr, err := evaluateSet(ctx, rtr, nil, seed, 2003+seed, c.Independent)
		if err != nil {
			return out, err
		}
		out.CorruptedIndependentScore = corr.TaskScore
		out.RobustnessDelta = out.IndependentTaskScore - out.CorruptedIndependentScore
	}
	return out, nil
}

// trainStudent runs the delayed-sign distillation stream: the teacher target
// is the fixture oracle distribution indexed by the pulse label, never a
// teacher Ask. The first corrupt episodes flip that label (and with it the
// oracle distribution) before the steps reach the student.
func trainStudent(ctx context.Context, c Config, seed uint64, corrupt int) (*learning.Trainer, error) {
	tr, err := newFixture(c.LearningRate)
	if err != nil {
		return nil, err
	}
	d := distill.DistributionDistiller{
		Temperature: c.Temperature,
		Scale:       1,
		Mix:         c.Mix,
		Alignment: distill.Alignment{
			Rule:             distill.AlignmentIdentical,
			TeacherVocabHash: fixtureVocabHash,
			StudentVocabHash: fixtureVocabHash,
		},
	}
	for e := 0; e < c.Episodes; e++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		ep := experiment.DelayedEpisode(1001+seed, uint64(e))
		label := pulseLabel(ep)
		if e < corrupt {
			label = 1 - label
		}
		if _, err := distill.StepDistribution(ctx, tr, d, ep.Input, teacherDistribution(label), label); err != nil {
			return nil, err
		}
	}
	return tr, nil
}

// evalSetResult is one split's numbers: the student's own agreement and task
// score always, plus the assisted task score, fallback and Ask counts when a
// teacher was consulted.
type evalSetResult struct {
	Agreement     float64
	TaskScore     float64
	AssistedScore float64
	Fallbacks     int
	Asks          int
}

// evaluateSet scores student on DelayedEpisode(setSeed, ·) with Predict only:
// agreement is distill.Agreement against the fixture oracle's argmax (which
// equals the pulse label), task score the fraction of examples whose
// argmax(Predict) equals the real pulse label. When t is non-nil
// (teacher_assisted), every input is also asked once — RequestID
// assist-<seed>-<set>-<i>, InputHash the sha256 hex of the input row's JSON —
// and the assisted prediction (the teacher's "0"/"1" label, falling back to
// the student's own argmax on any Ask error or unusable answer) is scored
// against the pulse label.
func evaluateSet(ctx context.Context, student distill.Student, t teacher.Teacher, seed, setSeed uint64, count int) (out evalSetResult, err error) {
	inputs := make([][][]float64, count)
	teacherArgmax := make([]int, count)
	match := 0
	assistedMatch := 0
	for i := 0; i < count; i++ {
		ep := experiment.DelayedEpisode(setSeed, uint64(i))
		inputs[i] = ep.Input
		teacherArgmax[i] = pulseLabel(ep)
		logits, err := student.Predict(ctx, ep.Input)
		if err != nil {
			return out, err
		}
		pred := argmax(logits)
		if pred == teacherArgmax[i] {
			match++
		}
		if t != nil {
			assisted := pred
			hash, herr := inputHash(ep.Input)
			if herr == nil {
				out.Asks++
				resp, aerr := t.Ask(ctx, teacher.Request{
					RequestID:    fmt.Sprintf("assist-%d-%d-%d", seed, setSeed, i),
					InputHash:    hash,
					Kind:         teacher.KindLabel,
					ModelVersion: "studenteval/v1",
				})
				if label, ok := teacherLabel(resp, aerr); ok {
					assisted = label
				} else {
					out.Fallbacks++
				}
			} else {
				out.Fallbacks++
			}
			if assisted == teacherArgmax[i] {
				assistedMatch++
			}
		}
	}
	out.Agreement, err = distill.Agreement(ctx, student, inputs, teacherArgmax)
	if err != nil {
		return out, err
	}
	out.TaskScore = float64(match) / float64(count)
	if t != nil {
		out.AssistedScore = float64(assistedMatch) / float64(count)
	}
	return out, nil
}

// inputHash is the sha256 hex of the input row's JSON encoding — the hash the
// teacher keys its answers by.
func inputHash(input [][]float64) (string, error) {
	b, err := json.Marshal(input)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256(b)), nil
}

// teacherLabel extracts the class from a teacher response: the response must
// pass Validate and its Answer must be the JSON string "0" or "1". Anything
// else — an Ask error, an invalid response, or another answer shape — is not
// a usable teacher label.
func teacherLabel(resp teacher.Response, askErr error) (int, bool) {
	if askErr != nil {
		return 0, false
	}
	if err := resp.Validate(); err != nil {
		return 0, false
	}
	var s string
	if err := json.Unmarshal(resp.Answer, &s); err != nil {
		return 0, false
	}
	switch s {
	case "0":
		return 0, true
	case "1":
		return 1, true
	}
	return 0, false
}

// newFixture is the three-neuron delayed chain with OutputSize 2 and a fixed
// readout {1, −1} — the same fixture as distill's train tests — learning at
// the rate the protocol declares.
func newFixture(rate float64) (*learning.Trainer, error) {
	cfg := learning.Config{
		Dynamics:  dynamics.Config{Nodes: 3, Sources: []int{0, 1, 1}, Targets: []int{1, 1, 2}, DT: 1, Activation: "tanh"},
		InputSize: 1, OutputSize: 2, ReadoutNodes: []int{2},
	}
	p := learning.Parameters{
		Core:    dynamics.Parameters{Weights: []float64{0.237, 0.2, 0.2}, Bias: []float64{0, 0, 0}, LogTau: []float64{math.Log(2), math.Log(2), math.Log(2)}},
		Encoder: []float64{1, 0, 0}, Readout: []float64{1, -1},
	}
	o := learning.DefaultOptions()
	o.LearningRate = rate
	return learning.NewTrainer(cfg, p, o)
}

// teacherDistribution is the fixture oracle's two-point distribution whose
// argmax is the pulse class: {0.9, 0.1} for class 0, {0.1, 0.9} for class 1.
func teacherDistribution(label int) distill.TeacherDistribution {
	if label == 1 {
		return distill.TeacherDistribution{Probabilities: []float64{0.1, 0.9}}
	}
	return distill.TeacherDistribution{Probabilities: []float64{0.9, 0.1}}
}

// pulseLabel maps the delayed pulse sign to its class: a positive pulse is
// class 1, a negative pulse class 0.
func pulseLabel(ep experiment.Episode) int {
	if ep.Input[0][0] > 0 {
		return 1
	}
	return 0
}

// argmax returns the index of the first largest value, or −1 when empty.
func argmax(v []float64) int {
	if len(v) == 0 {
		return -1
	}
	best := 0
	for i := 1; i < len(v); i++ {
		if v[i] > v[best] {
			best = i
		}
	}
	return best
}

// callsOf returns t's own Ask count when t keeps one, otherwise 0.
func callsOf(t teacher.Teacher) int {
	if c, ok := t.(teacherCallCounter); ok {
		return c.Calls()
	}
	return 0
}

// configHash is the hex sha256 of c's JSON encoding.
func configHash(c Config) string {
	b, _ := json.Marshal(c)
	return fmt.Sprintf("%x", sha256.Sum256(b))
}

// assumptions is the fixed caveat block attached to every report; the
// teacher_assisted mode appends its own so the assisted numbers are never read
// as the student's independent capability.
func assumptions(mode string) []string {
	list := []string{
		"Fixture teacher: an oracle distribution over the delayed-sign task, not a trained model.",
		"Scores are on a three-neuron fixture; they show that the evaluation runs without a teacher, not a capability claim.",
	}
	if mode == ModeTeacherAssisted {
		list = append(list, "teacher_assisted scores include the teacher's answers; they are never reported as the student's own capability.")
	}
	return list
}

// finite reports whether v is neither NaN nor ±Inf.
func finite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}
