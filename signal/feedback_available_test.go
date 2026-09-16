package signal

import (
	"reflect"
	"testing"
)

// availabilityFeedback builds one valid feedback whose produced and available
// timestamps are both in milliseconds, the unit testTimestamp uses.
func availabilityFeedback(t *testing.T, actionID string, producedAt, availableAt int64, score float64) Feedback {
	t.Helper()
	f, err := NewFeedback(FeedbackSpec{
		SchemaVersion: testVersion(),
		ExperienceID:  "exp-1",
		ActionID:      actionID,
		ProducedAt:    testTimestamp(producedAt),
		AvailableAt:   testTimestamp(availableAt),
		Source:        "teacher",
		Score:         score,
		ModelVersion:  testVersion(),
	})
	if err != nil {
		t.Fatalf("build feedback %q: %v", actionID, err)
	}
	return f
}

// The filter is the only gate between recorded feedback and a consumer, so the
// boundary case (available_at exactly now) is kept and the declared order of
// the input is the order of the result.
func TestAvailableFeedbackKeepsArrivedFeedbackInInputOrder(t *testing.T) {
	all := []Feedback{
		availabilityFeedback(t, "late", 5, 21, 1),
		availabilityFeedback(t, "equal", 5, 20, 2),
		availabilityFeedback(t, "early", 1, 9, 3),
		availabilityFeedback(t, "much-later", 5, 100, 4),
	}
	available, err := AvailableFeedback(all, testTimestamp(20))
	if err != nil {
		t.Fatalf("AvailableFeedback() error = %v", err)
	}
	var got []string
	for _, f := range available {
		got = append(got, f.ActionID())
	}
	want := []string{"equal", "early"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("AvailableFeedback() = %v, want %v", got, want)
	}
	if available[0].Score() != 2 || available[1].Score() != 3 {
		t.Fatalf("AvailableFeedback() returned scores %v and %v, want 2 and 3", available[0].Score(), available[1].Score())
	}
}

func TestAvailableFeedbackReturnsNoFeedbackBeforeAnyArrives(t *testing.T) {
	all := []Feedback{availabilityFeedback(t, "a", 5, 21, 1)}
	available, err := AvailableFeedback(all, testTimestamp(20))
	if err != nil {
		t.Fatalf("AvailableFeedback() error = %v", err)
	}
	if len(available) != 0 {
		t.Fatalf("AvailableFeedback() returned %d feedback, want 0", len(available))
	}
	empty, err := AvailableFeedback(nil, testTimestamp(20))
	if err != nil {
		t.Fatalf("AvailableFeedback(nil) error = %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("AvailableFeedback(nil) returned %d feedback, want 0", len(empty))
	}
}

// A filter that compared values across different units would silently admit
// feedback that has not arrived, so a mismatch is an error and not a drop.
func TestAvailableFeedbackRejectsAUnitMismatch(t *testing.T) {
	all := []Feedback{availabilityFeedback(t, "a", 5, 10, 1)}
	if _, err := AvailableFeedback(all, Timestamp{Value: 20, Unit: TimeUnitModelStep}); err == nil {
		t.Fatal("AvailableFeedback() error = nil for a feedback in a different time unit")
	}
	if _, err := AvailableFeedback(all, Timestamp{Value: -1, Unit: TimeUnitMilliseconds}); err == nil {
		t.Fatal("AvailableFeedback() error = nil for a negative now")
	}
	if _, err := AvailableFeedback([]Feedback{{}}, testTimestamp(20)); err == nil {
		t.Fatal("AvailableFeedback() error = nil for an invalid feedback")
	}
}

// The caller keeps its own record; the filter hands out copies and never
// reorders, rewrites or truncates the slice it was given.
func TestAvailableFeedbackDoesNotMutateItsInput(t *testing.T) {
	all := []Feedback{
		availabilityFeedback(t, "equal", 5, 20, 2),
		availabilityFeedback(t, "late", 5, 21, 1),
	}
	before := append([]Feedback(nil), all...)
	available, err := AvailableFeedback(all, testTimestamp(20))
	if err != nil {
		t.Fatalf("AvailableFeedback() error = %v", err)
	}
	if len(available) != 1 {
		t.Fatalf("AvailableFeedback() returned %d feedback, want 1", len(available))
	}
	if !reflect.DeepEqual(all, before) {
		t.Fatal("AvailableFeedback() mutated the input slice")
	}
	if !reflect.DeepEqual(available[0], before[0]) {
		t.Fatal("AvailableFeedback() returned a value that differs from its input")
	}
}
