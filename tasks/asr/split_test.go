package asr_test

import (
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/TimLai666/coimnet/tasks/asr"
)

func splitUtterance(text, speaker, session string) asr.Utterance {
	return asr.Utterance{Text: text, Speaker: speaker, Session: session}
}

func TestSplitBySpeakerSessionTransitiveGroup(t *testing.T) {
	utterances := []asr.Utterance{
		splitUtterance("a", "speaker-a", "session-a"),
		splitUtterance("b", "speaker-a", "session-b"),
		splitUtterance("c", "speaker-c", "session-b"),
		splitUtterance("d", "speaker-d", "session-d"),
	}

	train, test, err := asr.SplitBySpeakerSession(utterances, 0.5, 7)
	if err != nil {
		t.Fatalf("SplitBySpeakerSession: %v", err)
	}
	if len(train) == 0 || len(test) == 0 {
		t.Fatalf("split lengths = (%d, %d), want both partitions non-empty", len(train), len(test))
	}

	partition := make(map[string]int, len(utterances))
	for _, u := range train {
		partition[u.Text] = 0
	}
	for _, u := range test {
		partition[u.Text] = 1
	}
	if partition["a"] != partition["b"] || partition["b"] != partition["c"] {
		t.Fatalf("transitive speaker/session group was split: %#v", partition)
	}
	if partition["d"] == partition["a"] {
		t.Fatalf("independent group was not placed on the other side: %#v", partition)
	}
}

func TestSplitBySpeakerSessionFindsCloserGroupCombination(t *testing.T) {
	var utterances []asr.Utterance
	for group, size := range []int{60, 25, 15} {
		for index := 0; index < size; index++ {
			utterances = append(utterances, splitUtterance(
				fmt.Sprintf("group-%d-%d", group, index),
				fmt.Sprintf("speaker-%d", group),
				fmt.Sprintf("session-%d-%d", group, index),
			))
		}
	}

	_, test, err := asr.SplitBySpeakerSession(utterances, 0.4, 8)
	if err != nil {
		t.Fatalf("SplitBySpeakerSession: %v", err)
	}
	if len(test) != 40 {
		t.Fatalf("test count = %d, want exact achievable target 40", len(test))
	}
}

func TestSplitBySpeakerSessionFindsCloserImbalancedCombination(t *testing.T) {
	utterances := splitGroups([]int{90, 7, 3})

	_, test, err := asr.SplitBySpeakerSession(utterances, 0.1, 4)
	if err != nil {
		t.Fatalf("SplitBySpeakerSession: %v", err)
	}
	if len(test) != 10 {
		t.Fatalf("test count = %d, want closest achievable count 10", len(test))
	}
}

func TestSplitBySpeakerSessionFindsExactCombinationAfterLargeGroup(t *testing.T) {
	sizes := make([]int, 302)
	for i := 0; i < 300; i++ {
		sizes[i] = 4
	}
	sizes[300] = 2
	sizes[301] = 10000
	utterances := splitGroups(sizes)

	_, test, err := asr.SplitBySpeakerSession(utterances, 10002.0/11202.0, 226)
	if err != nil {
		t.Fatalf("SplitBySpeakerSession: %v", err)
	}
	if len(test) != 10002 {
		t.Fatalf("test count = %d, want exact achievable count 10002", len(test))
	}
}

func TestSplitBySpeakerSessionMatchesSmallSubsetOptimum(t *testing.T) {
	cases := []struct {
		name        string
		sizes       []int
		targetCount int
	}{
		{name: "exact pair", sizes: []int{2, 5, 9, 4}, targetCount: 7},
		{name: "exact middle", sizes: []int{3, 7, 2, 8, 5}, targetCount: 10},
		{name: "high target", sizes: []int{10, 1, 4, 6, 3, 11}, targetCount: 25},
		{name: "unreachable tie", sizes: []int{30, 1, 1, 1, 2, 5}, targetCount: 20},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			utterances := splitGroups(tc.sizes)
			total := len(utterances)
			want := closestSubsetCount(tc.sizes, float64(tc.targetCount))
			_, test, err := asr.SplitBySpeakerSession(utterances, float64(tc.targetCount)/float64(total), 31)
			if err != nil {
				t.Fatalf("SplitBySpeakerSession: %v", err)
			}
			if len(test) != want {
				t.Fatalf("test count = %d, want exact nearest count %d", len(test), want)
			}
		})
	}
}

func TestSplitBySpeakerSessionUsesSeededTieBreak(t *testing.T) {
	utterances := splitGroups([]int{1, 1, 1, 1})

	_, first, err := asr.SplitBySpeakerSession(utterances, 0.5, 1)
	if err != nil {
		t.Fatalf("first split: %v", err)
	}
	_, repeat, err := asr.SplitBySpeakerSession(utterances, 0.5, 1)
	if err != nil {
		t.Fatalf("repeat split: %v", err)
	}
	_, other, err := asr.SplitBySpeakerSession(utterances, 0.5, 2)
	if err != nil {
		t.Fatalf("other split: %v", err)
	}
	if !reflect.DeepEqual(first, repeat) {
		t.Fatalf("same seed changed tie result: first=%#v repeat=%#v", first, repeat)
	}
	if len(first) != 2 || len(other) != 2 {
		t.Fatalf("tie split lengths = (%d, %d), want 2 each", len(first), len(other))
	}
	if reflect.DeepEqual(first, other) {
		t.Fatalf("different seeds did not change seeded tie result: first=%#v other=%#v", first, other)
	}
}

func TestSplitBySpeakerSessionHandlesManySingletonGroups(t *testing.T) {
	const groupCount = 20000
	sizes := make([]int, groupCount)
	for i := range sizes {
		sizes[i] = 1
	}
	utterances := splitGroups(sizes)
	for i := range utterances {
		utterances[i].Speaker = fmt.Sprintf("speaker-%d", i)
		utterances[i].Session = fmt.Sprintf("session-%d", i)
	}

	started := time.Now()
	_, test, err := asr.SplitBySpeakerSession(utterances, 0.25, 17)
	if err != nil {
		t.Fatalf("SplitBySpeakerSession: %v", err)
	}
	if len(test) != groupCount/4 {
		t.Fatalf("test count = %d, want %d", len(test), groupCount/4)
	}
	t.Logf("split %d singleton groups in %s", groupCount, time.Since(started))
}

func TestSplitBySpeakerSessionHandlesManyNearlySingletonGroups(t *testing.T) {
	const groupCount = 2000
	sizes := make([]int, groupCount)
	for i := range sizes {
		sizes[i] = 1
	}
	sizes[groupCount-1] = 2
	utterances := splitGroups(sizes)

	started := time.Now()
	_, test, err := asr.SplitBySpeakerSession(utterances, 0.25, 19)
	if err != nil {
		t.Fatalf("SplitBySpeakerSession: %v", err)
	}
	if len(test) < 1 || len(test) >= len(utterances) {
		t.Fatalf("test count = %d, want a nonempty proper partition", len(test))
	}
	t.Logf("split %d nearly singleton groups in %s", groupCount, time.Since(started))
}

func TestSplitBySpeakerSessionFailsSafelyBeyondExactSearchBudget(t *testing.T) {
	sizes := make([]int, 400)
	for i := range sizes {
		sizes[i] = i + 1
	}
	utterances := splitGroups(sizes)

	train, test, err := asr.SplitBySpeakerSession(utterances, 0.5, 23)
	if err == nil || !strings.Contains(err.Error(), "bounded") {
		t.Fatalf("SplitBySpeakerSession error = %v, want bounded exact-search error", err)
	}
	if train != nil || test != nil {
		t.Fatalf("budget failure returned partitions: train=%#v test=%#v", train, test)
	}
}

func TestSplitBySpeakerSessionRejectsOneConnectedGroup(t *testing.T) {
	utterances := []asr.Utterance{
		splitUtterance("a", "speaker", "session-a"),
		splitUtterance("b", "speaker", "session-b"),
	}

	train, test, err := asr.SplitBySpeakerSession(utterances, 0.5, 1)
	if err == nil {
		t.Fatal("SplitBySpeakerSession accepted one connected group")
	}
	if !strings.Contains(err.Error(), "connected") {
		t.Fatalf("error = %q, want an explicit connected-group error", err)
	}
	if train != nil || test != nil {
		t.Fatalf("failed split returned partitions: train=%#v test=%#v", train, test)
	}
}

func TestSplitBySpeakerSessionIsReproducibleForSeed(t *testing.T) {
	utterances := []asr.Utterance{
		splitUtterance("a", "speaker-a", "session-a"),
		splitUtterance("b", "speaker-b", "session-b"),
		splitUtterance("c", "speaker-c", "session-c"),
		splitUtterance("d", "speaker-d", "session-d"),
		splitUtterance("e", "speaker-e", "session-e"),
	}

	train1, test1, err := asr.SplitBySpeakerSession(utterances, 0.4, 99)
	if err != nil {
		t.Fatalf("first split: %v", err)
	}
	train2, test2, err := asr.SplitBySpeakerSession(utterances, 0.4, 99)
	if err != nil {
		t.Fatalf("second split: %v", err)
	}
	if !reflect.DeepEqual(train1, train2) || !reflect.DeepEqual(test1, test2) {
		t.Fatalf("same seed produced different partitions:\nfirst train=%#v test=%#v\nsecond train=%#v test=%#v", train1, test1, train2, test2)
	}
}

func TestSplitBySpeakerSessionHasNoSpeakerOrSessionLeakage(t *testing.T) {
	utterances := []asr.Utterance{
		splitUtterance("a", "speaker-a", "session-a"),
		splitUtterance("b", "speaker-a", "session-b"),
		splitUtterance("c", "speaker-c", "session-c"),
		splitUtterance("d", "speaker-d", "session-c"),
		splitUtterance("e", "speaker-e", "session-e"),
		splitUtterance("f", "speaker-f", "session-f"),
	}

	train, test, err := asr.SplitBySpeakerSession(utterances, 0.5, 13)
	if err != nil {
		t.Fatalf("SplitBySpeakerSession: %v", err)
	}
	trainSpeakers, trainSessions := identitySets(train)
	testSpeakers, testSessions := identitySets(test)
	for speaker := range trainSpeakers {
		if testSpeakers[speaker] {
			t.Fatalf("speaker %q leaked across partitions", speaker)
		}
	}
	for session := range trainSessions {
		if testSessions[session] {
			t.Fatalf("session %q leaked across partitions", session)
		}
	}
}

func TestSplitBySpeakerSessionPreservesInputOrderAndOwnsOutputSlice(t *testing.T) {
	utterances := []asr.Utterance{
		splitUtterance("a", "speaker-a", "session-a"),
		splitUtterance("b", "speaker-b", "session-b"),
		splitUtterance("c", "speaker-c", "session-c"),
		splitUtterance("d", "speaker-d", "session-d"),
		splitUtterance("e", "speaker-e", "session-e"),
		splitUtterance("f", "speaker-f", "session-f"),
	}

	train, test, err := asr.SplitBySpeakerSession(utterances, 0.5, 21)
	if err != nil {
		t.Fatalf("SplitBySpeakerSession: %v", err)
	}
	assertOriginalOrder(t, utterances, train)
	assertOriginalOrder(t, utterances, test)

	train[0].Text = "changed"
	if utterances[outputIndex(utterances, train[0].Speaker, train[0].Session)].Text == "changed" {
		t.Fatal("train output shares its backing array with input")
	}
}

func TestSplitBySpeakerSessionRejectsInvalidInput(t *testing.T) {
	valid := []asr.Utterance{
		splitUtterance("a", "speaker-a", "session-a"),
		splitUtterance("b", "speaker-b", "session-b"),
	}
	cases := []struct {
		name         string
		utterances   []asr.Utterance
		testFraction float64
	}{
		{name: "zero fraction", utterances: valid, testFraction: 0},
		{name: "one fraction", utterances: valid, testFraction: 1},
		{name: "negative fraction", utterances: valid, testFraction: -0.1},
		{name: "infinite fraction", utterances: valid, testFraction: math.Inf(1)},
		{name: "nan fraction", utterances: valid, testFraction: math.NaN()},
		{name: "empty input", utterances: nil, testFraction: 0.5},
		{name: "one utterance", utterances: valid[:1], testFraction: 0.5},
		{name: "empty speaker", utterances: []asr.Utterance{
			splitUtterance("a", "", "session-a"), splitUtterance("b", "speaker-b", "session-b"),
		}, testFraction: 0.5},
		{name: "empty session", utterances: []asr.Utterance{
			splitUtterance("a", "speaker-a", ""), splitUtterance("b", "speaker-b", "session-b"),
		}, testFraction: 0.5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			train, test, err := asr.SplitBySpeakerSession(tc.utterances, tc.testFraction, 3)
			if err == nil {
				t.Fatal("SplitBySpeakerSession accepted invalid input")
			}
			if train != nil || test != nil {
				t.Fatalf("invalid input returned partitions: train=%#v test=%#v", train, test)
			}
		})
	}
}

func identitySets(utterances []asr.Utterance) (speakers, sessions map[string]bool) {
	speakers = make(map[string]bool, len(utterances))
	sessions = make(map[string]bool, len(utterances))
	for _, u := range utterances {
		speakers[u.Speaker] = true
		sessions[u.Session] = true
	}
	return speakers, sessions
}

func assertOriginalOrder(t *testing.T, input, output []asr.Utterance) {
	t.Helper()
	last := -1
	for _, got := range output {
		index := outputIndex(input, got.Speaker, got.Session)
		if index <= last {
			t.Fatalf("output order is not stable: index %d after %d", index, last)
		}
		last = index
	}
}

func outputIndex(input []asr.Utterance, speaker, session string) int {
	for i, u := range input {
		if u.Speaker == speaker && u.Session == session {
			return i
		}
	}
	return -1
}

func splitGroups(sizes []int) []asr.Utterance {
	var utterances []asr.Utterance
	for group, size := range sizes {
		for index := 0; index < size; index++ {
			utterances = append(utterances, splitUtterance(
				fmt.Sprintf("group-%d-%d", group, index),
				fmt.Sprintf("speaker-%d", group),
				fmt.Sprintf("session-%d-%d", group, index),
			))
		}
	}
	return utterances
}

func closestSubsetCount(sizes []int, target float64) int {
	bestCount := 0
	bestDistance := math.Abs(target)
	for mask := 1; mask < (1<<len(sizes))-1; mask++ {
		count := 0
		for index, size := range sizes {
			if mask&(1<<index) != 0 {
				count += size
			}
		}
		distance := math.Abs(float64(count) - target)
		if distance < bestDistance || (distance == bestDistance && count < bestCount) {
			bestCount = count
			bestDistance = distance
		}
	}
	return bestCount
}
