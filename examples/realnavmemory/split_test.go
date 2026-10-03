package main

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestMakeSplitStratifiesTrainAndPreservesOriginalTestOrder(t *testing.T) {
	trials := make([]historyTrial, 0, 39)
	originalTrain := make([]string, 0, 26)
	originalTest := make([]string, 0, 13)
	for i := 0; i < 39; i++ {
		id := trialID(i)
		condition := "non-rewarded"
		if i%3 != 0 {
			condition = "rewarded"
		}
		trials = append(trials, historyTrial{ID: id, Condition: condition})
		if i < 26 {
			originalTrain = append(originalTrain, id)
		} else {
			originalTest = append(originalTest, id)
		}
	}
	split, err := makeSplit(trials, originalTrain, originalTest)
	if err != nil {
		t.Fatalf("makeSplit: %v", err)
	}
	if len(split.Train) != 20 || len(split.Validation) != 6 || len(split.Test) != 13 {
		t.Fatalf("split counts = %d/%d/%d, want 20/6/13", len(split.Train), len(split.Validation), len(split.Test))
	}
	if !reflect.DeepEqual(split.Test, originalTest) {
		t.Fatalf("test order = %v, want original order %v", split.Test, originalTest)
	}
	if !sort.StringsAreSorted(split.Train) || !sort.StringsAreSorted(split.Validation) {
		t.Fatalf("train/validation are not lexical: train=%v validation=%v", split.Train, split.Validation)
	}
	if err := validateSplit(trials, split); err != nil {
		t.Fatalf("validateSplit: %v", err)
	}
	for _, id := range split.Validation {
		if !strings.HasPrefix(id, "trial-") {
			t.Fatalf("unexpected validation ID %q", id)
		}
	}
}

func TestMakeSplitUsesLargestRemainderAndHashOrderDeterministically(t *testing.T) {
	trials := []historyTrial{
		{ID: "a1", Condition: "alpha"}, {ID: "a2", Condition: "alpha"}, {ID: "a3", Condition: "alpha"},
		{ID: "b1", Condition: "beta"}, {ID: "b2", Condition: "beta"}, {ID: "b3", Condition: "beta"},
		{ID: "c1", Condition: "charlie"}, {ID: "c2", Condition: "charlie"}, {ID: "c3", Condition: "charlie"},
	}
	train := []string{"a1", "a2", "a3", "b1", "b2", "b3", "c1", "c2", "c3"}
	test := []string{"held-out"}
	all := append(append([]string(nil), train...), test...)
	trials = append(trials, historyTrial{ID: "held-out", Condition: "beta"})
	split, err := makeSplit(trials, train, test)
	if err != nil {
		t.Fatalf("makeSplit: %v", err)
	}
	if len(split.Validation) != 2 || len(split.Train) != 7 {
		t.Fatalf("split counts = %d/%d, want 7/2", len(split.Train), len(split.Validation))
	}
	again, err := makeSplit(trials, train, test)
	if err != nil {
		t.Fatalf("second makeSplit: %v", err)
	}
	if !reflect.DeepEqual(split, again) {
		t.Fatalf("split is not deterministic: first=%+v second=%+v", split, again)
	}
	if len(all) != len(trials) {
		t.Fatal("fixture construction lost a trial")
	}
}

func TestMakeSplitSupportsSmallFixtureWithOneValidationAndOneTrain(t *testing.T) {
	trials := []historyTrial{
		{ID: "a", Condition: "rewarded"},
		{ID: "b", Condition: "rewarded"},
		{ID: "c", Condition: "non-rewarded"},
		{ID: "d", Condition: "non-rewarded"},
	}
	split, err := makeSplit(trials, []string{"a", "b", "c"}, []string{"d"})
	if err != nil {
		t.Fatalf("makeSplit: %v", err)
	}
	if len(split.Validation) != 1 || len(split.Train) != 2 {
		t.Fatalf("split counts = %d/%d, want 2/1", len(split.Train), len(split.Validation))
	}
}

func TestMakeSplitAndValidateRejectBadMembership(t *testing.T) {
	trials := []historyTrial{{ID: "a", Condition: "rewarded"}, {ID: "b", Condition: "rewarded"}, {ID: "c", Condition: "non-rewarded"}}
	tests := []struct {
		name  string
		train []string
		test  []string
		want  string
	}{
		{name: "empty original train", train: nil, test: []string{"c"}, want: "empty"},
		{name: "empty original test", train: []string{"a", "b", "c"}, test: nil, want: "empty"},
		{name: "duplicate original", train: []string{"a", "a", "b"}, test: []string{"c"}, want: "duplicate"},
		{name: "cross group", train: []string{"a", "b"}, test: []string{"b", "c"}, want: "overlap"},
		{name: "unknown original", train: []string{"a", "b", "unknown"}, test: []string{"c"}, want: "unknown"},
		{name: "omitted original", train: []string{"a"}, test: []string{"c"}, want: "cover"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := makeSplit(trials, test.train, test.test); err == nil || !strings.Contains(strings.ToLower(err.Error()), test.want) {
				t.Fatalf("error = %v, want substring %q", err, test.want)
			}
		})
	}

	invalid := []trialSplit{
		{Train: []string{"a", "b"}, Validation: []string{"b"}, Test: []string{"c"}},
		{Train: []string{"a", "unknown"}, Validation: []string{"b"}, Test: []string{"c"}},
		{Train: []string{"a"}, Validation: []string{"b"}, Test: []string{"c", "c"}},
		{Train: []string{"a"}, Validation: []string{"b"}, Test: []string{"c", "unknown"}},
		{Train: []string{"a"}, Validation: []string{"b"}, Test: nil},
	}
	for i, split := range invalid {
		if err := validateSplit(trials, split); err == nil {
			t.Fatalf("invalid split %d accepted: %+v", i, split)
		}
	}
}

func TestSelectTrialsFollowsRequestedIDsAndCopiesSteps(t *testing.T) {
	trials := []historyTrial{
		{ID: "a", Condition: "rewarded", Steps: []historyStep{{T: 1}}},
		{ID: "b", Condition: "non-rewarded", Steps: []historyStep{{T: 2}}},
	}
	selected, err := selectTrials(trials, []string{"b", "a"})
	if err != nil {
		t.Fatalf("selectTrials: %v", err)
	}
	if got := []string{selected[0].ID, selected[1].ID}; !reflect.DeepEqual(got, []string{"b", "a"}) {
		t.Fatalf("selected IDs = %v, want [b a]", got)
	}
	selected[0].Steps[0].T = 99
	if trials[1].Steps[0].T == 99 {
		t.Fatal("selectTrials shared mutable steps with input")
	}
	for _, ids := range [][]string{nil, {"a", "a"}, {"unknown"}, {""}} {
		if _, err := selectTrials(trials, ids); err == nil {
			t.Fatalf("selectTrials(%v) succeeded", ids)
		}
	}
}

func trialID(i int) string {
	return fmt.Sprintf("trial-%02d", i)
}
