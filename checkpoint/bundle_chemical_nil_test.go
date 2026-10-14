package checkpoint

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/learning"
)

func saveChemicalCollectionBundle(t *testing.T, dir, kind string, snapshot learning.TrainingSnapshot) {
	t.Helper()
	var err error
	if kind == "training" {
		err = SaveTrainingBundle(context.Background(), dir, snapshot)
	} else {
		err = SaveTaskTrainingBundle(context.Background(), dir, "coimnet-task-fixture/v1", []byte(`{"fixture":"chemical-null-collections"}`), snapshot)
	}
	if err != nil {
		t.Fatal(err)
	}
}

func loadChemicalCollectionBundle(dir, kind string) (learning.TrainingSnapshot, error) {
	if kind == "training" {
		return LoadTrainingBundle(context.Background(), dir)
	}
	s, err := LoadTaskTrainingBundle(context.Background(), dir, "coimnet-task-fixture/v1")
	return s.Training, err
}

func TestBundleChemicalNilCollectionsRoundTrip(t *testing.T) {
	for _, kind := range []string{"training", "task"} {
		for _, collections := range []string{"resources", "feedback", "both", "nonempty"} {
			t.Run(kind+"/"+collections, func(t *testing.T) {
				base := newChemicalEpisodeCheckpointTrainer(t)
				defer base.Close()
				s := base.Snapshot()
				if collections == "resources" || collections == "both" {
					s.Config.Chemistry.Part.Resources = nil
				}
				if collections == "feedback" || collections == "both" {
					s.Config.Chemistry.Part.PendingFeedback = nil
				}
				full, err := learning.NewTrainer(s.Config, s.Parameters, s.Options)
				if err != nil {
					t.Fatal(err)
				}
				defer full.Close()
				dir := filepath.Join(t.TempDir(), kind)
				saveChemicalCollectionBundle(t, dir, kind, full.Snapshot())
				files := make(map[string][]byte)
				for _, name := range []string{bundleDocumentFile, bundleManifestFile, bundleArraysFile} {
					files[name], err = os.ReadFile(filepath.Join(dir, name))
					if err != nil {
						t.Fatal(err)
					}
				}
				var object map[string]json.RawMessage
				if err := json.Unmarshal(files[bundleDocumentFile], &object); err != nil {
					t.Fatal(err)
				}
				if kind == "task" {
					if err := json.Unmarshal(object["training"], &object); err != nil {
						t.Fatal(err)
					}
				}
				var config, chemistry, part map[string]json.RawMessage
				if err := json.Unmarshal(object["config"], &config); err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal(config["chemistry"], &chemistry); err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal(chemistry["part"], &part); err != nil {
					t.Fatal(err)
				}
				for field, nilExpected := range map[string]bool{"resources": collections == "resources" || collections == "both", "pending_feedback": collections == "feedback" || collections == "both"} {
					raw, present := part[field]
					if !present || isJSONNull(raw) != nilExpected {
						t.Fatalf("writer %s present=%v null=%v, want null=%v", field, present, isJSONNull(raw), nilExpected)
					}
				}
				loaded, err := loadChemicalCollectionBundle(dir, kind)
				if err != nil {
					t.Fatalf("writer-produced legal collection boundary refused: %v", err)
				}
				assertBundleJSONEqual(t, loaded, normalizeBundleTrainingSnapshot(full.Snapshot()))
				resumed, err := learning.RestoreTrainer(loaded)
				if err != nil {
					t.Fatal(err)
				}
				defer resumed.Close()
				a := trainChemicalEpisodeCheckpoint(t, full, 0, 4)
				b := trainChemicalEpisodeCheckpoint(t, resumed, 0, 4)
				if !reflect.DeepEqual(a, b) {
					t.Fatal("resumed next training results differ")
				}
				applied := false
				for _, result := range a {
					applied = applied || result.Applied
				}
				if !applied {
					t.Fatal("control did not apply any parameter update")
				}
				assertBundleJSONEqual(t, resumed.Snapshot(), full.Snapshot())
				assertPresenceBundleUnchanged(t, dir, files)
			})
		}
	}
}

func TestBundleChemicalCollectionsRemainRequired(t *testing.T) {
	for _, kind := range []string{"training", "task"} {
		for _, field := range []string{"resources", "pending_feedback"} {
			t.Run(kind+"/"+field, func(t *testing.T) {
				trainer := newChemicalEpisodeCheckpointTrainer(t)
				defer trainer.Close()
				dir := filepath.Join(t.TempDir(), kind)
				saveChemicalCollectionBundle(t, dir, kind, trainer.Snapshot())
				path := "config.chemistry.part." + field
				if kind == "task" {
					path = "training." + path
				}
				files := rewritePresenceBundle(t, dir, path, false)
				loaded, err := loadChemicalCollectionBundle(dir, kind)
				if err == nil || !reflect.DeepEqual(loaded, learning.TrainingSnapshot{}) {
					t.Fatalf("accepted missing collection or returned partial snapshot: %v", err)
				}
				assertPresenceBundleUnchanged(t, dir, files)
			})
		}
	}
}

func TestEpisodeChemicalNullCollectionsRemainRejected(t *testing.T) {
	for _, field := range []string{"resources", "pending_feedback"} {
		t.Run(field, func(t *testing.T) {
			fixture := newChemicalEpisodeCheckpointFixture(t)
			document := chemicalEpisodeCheckpointMutation(t, fixture.doc, mutateChemicalPart(func(part map[string]json.RawMessage) {
				part[field] = json.RawMessage("null")
			}))
			got, err := loadChemicalEpisodeMutation(t, document)
			if err == nil || !reflect.DeepEqual(got, State{}) {
				t.Fatalf("single-file null collection accepted or returned partial state: %v", err)
			}
		})
	}
}
