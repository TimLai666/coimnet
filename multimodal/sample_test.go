package multimodal

import (
	"reflect"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/signal"
)

func mustTS(t *testing.T, step int64) signal.Timestamp {
	t.Helper()
	ts, err := signal.NewTimestamp(step, signal.TimeUnitModelStep)
	if err != nil {
		t.Fatalf("NewTimestamp(%d): %v", step, err)
	}
	return ts
}

func imageModality(values ...float64) Modality {
	return Modality{Present: true, Values: values, Shape: []int{2, 2}}
}

func audioModality(values ...float64) Modality {
	return Modality{Present: true, Values: values, Shape: []int{3}}
}

func missingAudioModality() Modality {
	return Modality{Present: false, Values: nil, Shape: []int{3}}
}

func imageFirstLayout() Layout {
	return Layout{Order: []string{"image", "audio"}, Widths: map[string]int{"image": 4, "audio": 3}}
}

func TestMissingModalityMustHaveNilValues(t *testing.T) {
	missingWithZeros := Sample{
		EntityID: "e1", EventID: "ev1",
		Modalities: map[string]Modality{
			"image": imageModality(1, 0, 0, 1),
			"audio": {Present: false, Values: []float64{0, 0, 0}, Shape: []int{3}},
		},
		Timestamps: map[string]signal.Timestamp{"image": mustTS(t, 1)},
	}
	err := missingWithZeros.Validate()
	if err == nil || !strings.Contains(err.Error(), "must be nil") {
		t.Fatalf("Validate() with zero-valued missing modality: error = %v, want containing %q", err, "must be nil")
	}

	nilValues := Sample{
		EntityID: "e1", EventID: "ev1",
		Modalities: map[string]Modality{
			"image": imageModality(1, 0, 0, 1),
			"audio": missingAudioModality(),
		},
		Timestamps: map[string]signal.Timestamp{"image": mustTS(t, 1)},
	}
	if err := nilValues.Validate(); err != nil {
		t.Fatalf("Validate() with nil-values missing modality: %v", err)
	}

	wrongLength := Sample{
		EntityID: "e1", EventID: "ev1",
		Modalities: map[string]Modality{
			"image": imageModality(1, 0, 0, 1),
			"audio": {Present: true, Values: []float64{1, 0, 0, 0}, Shape: []int{3}},
		},
		Timestamps: map[string]signal.Timestamp{"image": mustTS(t, 1), "audio": mustTS(t, 1)},
	}
	err = wrongLength.Validate()
	if err == nil || !strings.Contains(err.Error(), "shape") {
		t.Fatalf("Validate() with wrong value count: error = %v, want containing %q", err, "shape")
	}
}

func TestVectorDistinguishesMissingFromZero(t *testing.T) {
	layout := imageFirstLayout()
	if err := layout.Validate(); err != nil {
		t.Fatalf("Layout.Validate: %v", err)
	}
	if got := layout.Width(); got != 9 {
		t.Fatalf("Layout.Width() = %d, want 9", got)
	}

	missingAudioSample := Sample{
		EntityID: "e1", EventID: "ev1",
		Modalities: map[string]Modality{"image": imageModality(1, 0, 0, 1), "audio": missingAudioModality()},
		Timestamps: map[string]signal.Timestamp{"image": mustTS(t, 1)},
	}
	zeroAudioSample := Sample{
		EntityID: "e1", EventID: "ev1",
		Modalities: map[string]Modality{"image": imageModality(1, 0, 0, 1), "audio": audioModality(0, 0, 0)},
		Timestamps: map[string]signal.Timestamp{"image": mustTS(t, 1), "audio": mustTS(t, 1)},
	}

	missingVector, err := Vector(missingAudioSample, layout)
	if err != nil {
		t.Fatalf("Vector(missing audio): %v", err)
	}
	zeroVector, err := Vector(zeroAudioSample, layout)
	if err != nil {
		t.Fatalf("Vector(zero audio): %v", err)
	}
	if len(missingVector) != 9 || len(zeroVector) != 9 {
		t.Fatalf("vector lengths = %d, %d, want 9", len(missingVector), len(zeroVector))
	}
	wantMissing := []float64{1, 0, 0, 1, 1, 0, 0, 0, 0}
	wantZero := []float64{1, 0, 0, 1, 1, 0, 0, 0, 1}
	if !reflect.DeepEqual(missingVector, wantMissing) {
		t.Fatalf("missing vector = %v, want %v", missingVector, wantMissing)
	}
	if !reflect.DeepEqual(zeroVector, wantZero) {
		t.Fatalf("zero vector = %v, want %v", zeroVector, wantZero)
	}
	if reflect.DeepEqual(missingVector, zeroVector) {
		t.Fatalf("missing and zero audio must produce different vectors")
	}
	if missingVector[8] != 0 || zeroVector[8] != 1 {
		t.Fatalf("last element = %v, %v; want 0 and 1", missingVector[8], zeroVector[8])
	}
}

func TestVectorHasNoIDs(t *testing.T) {
	layout := imageFirstLayout()
	base := Sample{
		EntityID: "e1", EventID: "ev1",
		Modalities: map[string]Modality{"image": imageModality(1, 0, 0, 1), "audio": audioModality(0.5, 0.5, 0.5)},
		Timestamps: map[string]signal.Timestamp{"image": mustTS(t, 1), "audio": mustTS(t, 1)},
	}
	renamed := base
	renamed.EntityID = "another-entity"
	renamed.EventID = "another-event"

	vBase, err := Vector(base, layout)
	if err != nil {
		t.Fatalf("Vector: %v", err)
	}
	vRenamed, err := Vector(renamed, layout)
	if err != nil {
		t.Fatalf("Vector(renamed): %v", err)
	}
	if len(vBase) != len(vRenamed) {
		t.Fatalf("vector lengths differ: %d vs %d", len(vBase), len(vRenamed))
	}
	for i := range vBase {
		if vBase[i] != vRenamed[i] {
			t.Fatalf("element %d differs: %v vs %v", i, vBase[i], vRenamed[i])
		}
	}
	rt := reflect.TypeOf(vBase)
	if rt.Kind() != reflect.Slice || rt.Elem().Kind() != reflect.Float64 {
		t.Fatalf("Vector output type = %v, want []float64", rt)
	}
	for _, v := range vBase {
		if reflect.ValueOf(v).Kind() == reflect.String {
			t.Fatalf("Vector output contains a string element %v", v)
		}
	}
	if SchemaVersion != "coimnet-multimodal-sample/v1" {
		t.Fatalf("SchemaVersion = %q", SchemaVersion)
	}
}

func TestLayoutOrderIsFixed(t *testing.T) {
	imageFirst := imageFirstLayout()
	audioFirst := Layout{Order: []string{"audio", "image"}, Widths: map[string]int{"audio": 3, "image": 4}}
	s := Sample{
		EntityID: "e1", EventID: "ev1",
		Modalities: map[string]Modality{"image": imageModality(1, 0, 0, 1), "audio": audioModality(0.25, 0.5, 0.75)},
		Timestamps: map[string]signal.Timestamp{"image": mustTS(t, 1), "audio": mustTS(t, 1)},
	}

	vImageOnce, err := Vector(s, imageFirst)
	if err != nil {
		t.Fatalf("Vector(image first): %v", err)
	}
	vImageTwice, err := Vector(s, imageFirst)
	if err != nil {
		t.Fatalf("Vector(image first, again): %v", err)
	}
	if !reflect.DeepEqual(vImageOnce, vImageTwice) {
		t.Fatalf("same layout not deterministic: %v vs %v", vImageOnce, vImageTwice)
	}
	vAudioOnce, err := Vector(s, audioFirst)
	if err != nil {
		t.Fatalf("Vector(audio first): %v", err)
	}
	vAudioTwice, err := Vector(s, audioFirst)
	if err != nil {
		t.Fatalf("Vector(audio first, again): %v", err)
	}
	if !reflect.DeepEqual(vAudioOnce, vAudioTwice) {
		t.Fatalf("same layout not deterministic: %v vs %v", vAudioOnce, vAudioTwice)
	}
	if reflect.DeepEqual(vImageOnce, vAudioOnce) {
		t.Fatalf("different orders must produce different vectors")
	}
	wantAudioFirst := []float64{0.25, 0.5, 0.75, 1, 1, 0, 0, 1, 1}
	if !reflect.DeepEqual(vAudioOnce, wantAudioFirst) {
		t.Fatalf("audio-first vector = %v, want %v", vAudioOnce, wantAudioFirst)
	}

	missingAudio := Sample{
		EntityID: "e1", EventID: "ev1",
		Modalities: map[string]Modality{"image": imageModality(1, 0, 0, 1)},
		Timestamps: map[string]signal.Timestamp{"image": mustTS(t, 1)},
	}
	_, err = Vector(missingAudio, imageFirst)
	if err == nil || !strings.Contains(err.Error(), "audio") {
		t.Fatalf("Vector with missing modality: error = %v, want containing %q", err, "audio")
	}

	extra := s
	extra.Modalities = map[string]Modality{
		"image": imageModality(1, 0, 0, 1),
		"audio": audioModality(0.25, 0.5, 0.75),
		"video": {Present: true, Values: []float64{1}, Shape: []int{1}},
	}
	_, err = Vector(extra, imageFirst)
	if err == nil || !strings.Contains(err.Error(), "video") {
		t.Fatalf("Vector with extra modality: error = %v, want containing %q", err, "video")
	}

	wrongShape := Sample{
		EntityID: "e1", EventID: "ev1",
		Modalities: map[string]Modality{
			"image": imageModality(1, 0, 0, 1),
			"audio": {Present: true, Values: []float64{0, 0, 0, 0}, Shape: []int{3}},
		},
		Timestamps: map[string]signal.Timestamp{"image": mustTS(t, 1), "audio": mustTS(t, 1)},
	}
	_, err = Vector(wrongShape, imageFirst)
	if err == nil || !strings.Contains(err.Error(), "audio") {
		t.Fatalf("Vector with shape mismatch: error = %v, want containing %q", err, "audio")
	}
}

func TestPairing(t *testing.T) {
	t1 := mustTS(t, 1)
	t2 := mustTS(t, 2)
	t4 := mustTS(t, 4)
	samples := []Sample{
		{EntityID: "e1", EventID: "ev1", Modalities: map[string]Modality{"image": imageModality(1, 0, 0, 1), "audio": audioModality(0.5, 0.5, 0.5)}, Timestamps: map[string]signal.Timestamp{"image": t1, "audio": t1}},
		{EntityID: "e1", EventID: "ev2", Modalities: map[string]Modality{"image": imageModality(1, 0, 0, 1), "audio": missingAudioModality()}, Timestamps: map[string]signal.Timestamp{"image": t2}},
		{EntityID: "e2", EventID: "ev3", Modalities: map[string]Modality{"image": imageModality(1, 0, 0, 1), "audio": audioModality(0.5, 0.5, 0.5)}, Timestamps: map[string]signal.Timestamp{"image": t1, "audio": t1}},
		{EntityID: "e2", EventID: "ev4", Modalities: map[string]Modality{"image": imageModality(1, 0, 0, 1), "audio": audioModality(0.5, 0.5, 0.5)}, Timestamps: map[string]signal.Timestamp{"image": t4, "audio": t4}},
		{EntityID: "e1", EventID: "ev1", Modalities: map[string]Modality{"image": {Present: false, Values: nil, Shape: []int{2, 2}}, "audio": audioModality(0.5, 0.5, 0.5)}, Timestamps: map[string]signal.Timestamp{"audio": t1}},
	}

	sync, err := PairSynchronous(samples, "image", "audio")
	if err != nil {
		t.Fatalf("PairSynchronous: %v", err)
	}
	wantSync := []Pair{{A: 0, B: 4}}
	if !reflect.DeepEqual(sync, wantSync) {
		t.Fatalf("PairSynchronous = %v, want %v", sync, wantSync)
	}

	if _, err := PairSynchronous(samples, "video", "audio"); err == nil || !strings.Contains(err.Error(), "video") {
		t.Fatalf("PairSynchronous with unknown A: error = %v, want containing %q", err, "video")
	}
	if _, err := PairSynchronous(samples, "image", "video"); err == nil || !strings.Contains(err.Error(), "video") {
		t.Fatalf("PairSynchronous with unknown B: error = %v, want containing %q", err, "video")
	}

	async, err := PairAsynchronous(samples)
	if err != nil {
		t.Fatalf("PairAsynchronous: %v", err)
	}
	wantAsync := []Pair{{A: 0, B: 1}, {A: 1, B: 4}, {A: 2, B: 3}}
	if !reflect.DeepEqual(async, wantAsync) {
		t.Fatalf("PairAsynchronous = %v, want %v", async, wantAsync)
	}
}
