package distill_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/distill"
	"github.com/TimLai666/coimnet/signal"
	"github.com/TimLai666/coimnet/teacher"
)

const (
	hashA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	hashB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	hashC = "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	hashD = "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
	hashE = "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
)

var _ teacher.Teacher = (*fakeTeacher)(nil)

// fakeTeacher records the requests it is asked and answers from a per-hash
// map; hashes outside the map get teacher.ErrNoAnswer.
type fakeTeacher struct {
	asked   []teacher.Request
	answers map[string]json.RawMessage
}

func (t *fakeTeacher) Ask(ctx context.Context, r teacher.Request) (teacher.Response, error) {
	t.asked = append(t.asked, r)
	a, ok := t.answers[r.InputHash]
	if !ok {
		return teacher.Response{}, teacher.ErrNoAnswer
	}
	return teacher.Response{
		TeacherID:      "fake",
		TeacherVersion: "v1",
		RequestID:      r.RequestID,
		InputHash:      r.InputHash,
		AnswerKind:     r.Kind,
		Answer:         a,
		Time:           signal.Timestamp{Value: 1, Unit: signal.TimeUnitModelStep},
	}, nil
}

func (t *fakeTeacher) Describe() teacher.Descriptor {
	return teacher.Descriptor{ID: "fake", Version: "v1", Kind: teacher.DescriptorOffline}
}

// echoEncoder accepts any kind and any answer; collection tests are about the
// asking and keeping, not about encoding.
type echoEncoder struct{}

func (echoEncoder) Encode(kind string, answer json.RawMessage) (distill.Target, error) {
	return distill.Target{}, nil
}

type fakeTokenizer struct {
	vocab map[string]int
}

func (fakeTokenizer) VocabHash() string { return "student-v1" }

func (t fakeTokenizer) Encode(text string) ([]int, error) {
	var ids []int
	for _, w := range strings.Fields(text) {
		id, ok := t.vocab[w]
		if !ok {
			return nil, errors.New("unknown token")
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func askedHashes(f *fakeTeacher) []string {
	hs := make([]string, len(f.asked))
	for i, r := range f.asked {
		hs[i] = r.InputHash
	}
	sort.Strings(hs)
	return hs
}

func TestCollectNeverAsksHoldout(t *testing.T) {
	train := []string{hashC, hashA, hashB}
	holdout := []string{hashD, hashE}
	f := &fakeTeacher{answers: map[string]json.RawMessage{
		hashA: json.RawMessage(`"alpha"`),
		hashB: json.RawMessage(`"beta"`),
		hashC: json.RawMessage(`"gamma"`),
	}}
	d := distill.LabelDistiller{
		Teacher:      f,
		Encoder:      echoEncoder{},
		Kind:         teacher.KindLabel,
		ModelVersion: "v1",
		Fields:       map[string]string{"task": "cls"},
	}
	ex, rep, err := d.Collect(context.Background(), distill.Split{Train: train, Holdout: holdout})
	if err != nil {
		t.Fatalf("Collect = %v, want nil", err)
	}

	got := askedHashes(f)
	want := append([]string(nil), train...)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("asked hashes = %v, want %v", got, want)
	}
	for _, asked := range got {
		for _, ho := range holdout {
			if asked == ho {
				t.Fatalf("asked holdout hash %s", asked)
			}
		}
	}
	if len(ex) != len(train) {
		t.Fatalf("%d examples, want %d", len(ex), len(train))
	}
	if rep.Asked != len(train) || rep.Answered != len(train) || rep.NoAnswer != 0 ||
		rep.Deduplicated != 0 || rep.Holdout != len(holdout) {
		t.Fatalf("report = %+v, want Asked/Answered %d, NoAnswer/Deduplicated 0, Holdout %d",
			rep, len(train), len(holdout))
	}
	if !reflect.DeepEqual(rep.AskedHashes, want) {
		t.Fatalf("AskedHashes = %v, want %v", rep.AskedHashes, want)
	}
	for _, r := range f.asked {
		if r.RequestID != "distill-"+r.InputHash {
			t.Fatalf("RequestID = %q, want distill-<hash> for %s", r.RequestID, r.InputHash)
		}
		if r.Kind != teacher.KindLabel || r.ModelVersion != "v1" {
			t.Fatalf("request = %+v, want kind label and model version v1", r)
		}
		if r.Fields["task"] != "cls" {
			t.Fatalf("request fields = %v, want task=cls", r.Fields)
		}
	}
}

func TestCollectDropsTextThatRepeatsHoldout(t *testing.T) {
	repeated := "repeat this exact phrase"
	holdoutHash := sha256Hex(repeated)
	train := []string{hashA, hashB, hashC}
	f := &fakeTeacher{answers: map[string]json.RawMessage{
		hashA: json.RawMessage(`"` + repeated + `"`),
		hashB: json.RawMessage(`"distinct"`),
		hashC: json.RawMessage(`"other"`),
	}}
	d := distill.LabelDistiller{
		Teacher:      f,
		Encoder:      echoEncoder{},
		Kind:         teacher.KindText,
		ModelVersion: "v1",
	}
	ex, rep, err := d.Collect(context.Background(), distill.Split{Train: train, Holdout: []string{holdoutHash}})
	if err != nil {
		t.Fatalf("Collect = %v, want nil", err)
	}
	for _, e := range ex {
		if e.InputHash == hashA {
			t.Fatalf("example whose text repeats a holdout hash was kept")
		}
	}
	if len(ex) != 2 {
		t.Fatalf("%d examples, want 2", len(ex))
	}
	if rep.Deduplicated != 1 || rep.Answered != 3 || rep.NoAnswer != 0 {
		t.Fatalf("report = %+v, want Deduplicated 1, Answered 3, NoAnswer 0", rep)
	}
}

func TestCollectSkipsNoAnswer(t *testing.T) {
	train := []string{hashA, hashB, hashC, hashD}
	f := &fakeTeacher{answers: map[string]json.RawMessage{
		hashA: json.RawMessage(`"a"`),
		hashB: json.RawMessage(`"b"`),
		hashC: json.RawMessage(`"c"`),
	}}
	d := distill.LabelDistiller{
		Teacher:      f,
		Encoder:      echoEncoder{},
		Kind:         teacher.KindLabel,
		ModelVersion: "v1",
	}
	ex, rep, err := d.Collect(context.Background(), distill.Split{Train: train})
	if err != nil {
		t.Fatalf("Collect = %v, want nil", err)
	}
	if len(ex) != 3 {
		t.Fatalf("%d examples, want 3", len(ex))
	}
	if rep.NoAnswer != 1 || rep.Asked != 4 || rep.Answered != 3 {
		t.Fatalf("report = %+v, want NoAnswer 1, Asked 4, Answered 3", rep)
	}
	if got, want := askedHashes(f), []string{hashA, hashB, hashC, hashD}; !reflect.DeepEqual(got, want) {
		t.Fatalf("asked = %v, want %v", got, want)
	}
}

func TestTextEncoderUsesTheStudentTokenizer(t *testing.T) {
	tok := fakeTokenizer{vocab: map[string]int{"a": 0, "b": 1}}
	if got := tok.VocabHash(); got != "student-v1" {
		t.Fatalf("VocabHash() = %q, want student-v1", got)
	}
	got, err := (distill.TextEncoder{Tokenizer: tok}).Encode(teacher.KindText, json.RawMessage(`"a b a"`))
	if err != nil {
		t.Fatalf("TextEncoder.Encode = %v, want nil", err)
	}
	if want := (distill.Target{Classes: []int{0, 1, 0}}); !reflect.DeepEqual(got, want) {
		t.Fatalf("Encode = %+v, want %+v", got, want)
	}
	if _, err := (distill.TextEncoder{Tokenizer: tok}).Encode(teacher.KindText, json.RawMessage(`[1, 2, 3]`)); !errors.Is(err, distill.ErrTokenIDsRejected) {
		t.Fatalf("Encode(numeric array) = %v, want ErrTokenIDsRejected", err)
	}
}

func TestLabelEncoderIndexes(t *testing.T) {
	le := distill.LabelEncoder{Classes: []string{"dog", "cat"}}
	got, err := le.Encode(teacher.KindLabel, json.RawMessage(`"cat"`))
	if err != nil {
		t.Fatalf("Encode(cat) = %v, want nil", err)
	}
	if want := (distill.Target{Classes: []int{1}}); !reflect.DeepEqual(got, want) {
		t.Fatalf("Encode(cat) = %+v, want %+v", got, want)
	}
	if _, err := le.Encode(teacher.KindLabel, json.RawMessage(`"bird"`)); err == nil {
		t.Fatalf("Encode(bird) = nil, want unknown-label error")
	}
}

func TestActionEncoderIndexes(t *testing.T) {
	ae := distill.ActionEncoder{Actions: []string{"left", "right", "stay"}}
	got, err := ae.Encode(teacher.KindActionSequence, json.RawMessage(`["left","stay"]`))
	if err != nil {
		t.Fatalf("Encode = %v, want nil", err)
	}
	if want := (distill.Target{Classes: []int{0, 2}}); !reflect.DeepEqual(got, want) {
		t.Fatalf("Encode = %+v, want %+v", got, want)
	}
	if _, err := ae.Encode(teacher.KindActionSequence, json.RawMessage(`["unknown"]`)); err == nil {
		t.Fatalf("Encode(unknown) = nil, want error")
	}
}

func TestSplitValidate(t *testing.T) {
	good := distill.Split{Train: []string{hashA, hashB}, Holdout: []string{hashC}}
	if err := good.Validate(); err != nil {
		t.Fatalf("Validate(valid) = %v, want nil", err)
	}
	overlap := distill.Split{Train: []string{hashA, hashB}, Holdout: []string{hashB}}
	if err := overlap.Validate(); err == nil || !strings.Contains(err.Error(), "overlap") {
		t.Fatalf("Validate(overlap) = %v, want error containing overlap", err)
	}
	duplicate := distill.Split{Train: []string{hashA, hashA}}
	if err := duplicate.Validate(); err == nil {
		t.Fatalf("Validate(duplicate) = nil, want error")
	}
	for name, c := range map[string]distill.Split{
		"non-hex train":   {Train: []string{"short"}},
		"non-hex holdout": {Train: []string{hashA}, Holdout: []string{"NOTHEX"}},
		"empty train":     {Holdout: []string{hashA}},
	} {
		if err := c.Validate(); err == nil {
			t.Fatalf("Validate(%s) = nil, want error", name)
		}
	}
}

func TestCollectRejectsDistributionKind(t *testing.T) {
	f := &fakeTeacher{answers: map[string]json.RawMessage{hashA: json.RawMessage(`"x"`)}}
	d := distill.LabelDistiller{
		Teacher:      f,
		Encoder:      echoEncoder{},
		Kind:         teacher.KindDistribution,
		ModelVersion: "v1",
	}
	_, rep, err := d.Collect(context.Background(), distill.Split{Train: []string{hashA}})
	if !errors.Is(err, distill.ErrUnsupportedKind) {
		t.Fatalf("Collect = %v, want ErrUnsupportedKind", err)
	}
	if rep.Asked != 0 || len(f.asked) != 0 {
		t.Fatalf("teacher was asked %d times for a distribution kind, want 0", len(f.asked))
	}
}
