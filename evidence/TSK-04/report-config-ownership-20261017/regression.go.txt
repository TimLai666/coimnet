package media_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/TimLai666/coimnet/tasks/media"
)

type mediaReportOwnership struct {
	originalConfigJSON           []byte
	report, reportConfig         any
	configHash                   string
	callerSeeds, reportSeeds     []uint64
	callerHoldout, reportHoldout []string
	validateCaller               func() error
	changedHoldout               string
}

func mediaOwnershipJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func mediaOwnershipHash(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func mediaOwnershipRun(t *testing.T, count int, seed uint64, failed bool, message string) {
	t.Helper()
	if count != 1 || seed != 1 || failed || message != "" {
		t.Fatalf("normal run: count=%d seed=%d failed=%v error=%q", count, seed, failed, message)
	}
}

func TestMediaReportsOwnRunConfiguration(t *testing.T) {
	for _, name := range []string{"image", "audio", "video", "roles"} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			var check mediaReportOwnership
			switch name {
			case "image", "audio":
				c := media.DefaultRunConfig(name)
				c.Generator.Embed, c.Generator.Hidden, c.Generator.Settle = 4, 8, 1
				c.Epochs, c.Blocks, c.Seeds = 1, 1, []uint64{1}
				before := mediaOwnershipJSON(t, c)
				r, err := media.RunMedia(ctx, c)
				if err != nil || len(r.Runs) != 1 {
					t.Fatalf("RunMedia: %v, runs=%d", err, len(r.Runs))
				}
				mediaOwnershipRun(t, len(r.Runs), r.Runs[0].Seed, r.Runs[0].Failed, r.Runs[0].Error)
				changed := "red square"
				if name == "audio" {
					changed = "low steady"
				}
				check = mediaReportOwnership{before, r, r.Config, r.ConfigHash, c.Seeds, r.Config.Seeds,
					c.Holdout, r.Config.Holdout, c.Validate, changed}
			case "video":
				c := media.DefaultVideoRunConfig()
				c.Video.Embed, c.Video.Hidden, c.Video.Settle = 4, 8, 1
				c.Epochs, c.Seeds = 1, []uint64{1}
				before := mediaOwnershipJSON(t, c)
				r, err := media.RunVideo(ctx, c)
				if err != nil || len(r.Runs) != 1 {
					t.Fatalf("RunVideo: %v, runs=%d", err, len(r.Runs))
				}
				mediaOwnershipRun(t, len(r.Runs), r.Runs[0].Seed, r.Runs[0].Failed, r.Runs[0].Error)
				check = mediaReportOwnership{before, r, r.Config, r.ConfigHash, c.Seeds, r.Config.Seeds,
					c.Holdout, r.Config.Holdout, c.Validate, "left low"}
			case "roles":
				c := media.DefaultRolesConfig()
				c.Generator.Embed, c.Generator.Hidden, c.Generator.Settle = 4, 8, 1
				c.Epochs, c.Seeds = 1, []uint64{1}
				before := mediaOwnershipJSON(t, c)
				r, err := media.RunRoles(ctx, c)
				if err != nil || len(r.Runs) != 1 {
					t.Fatalf("RunRoles: %v, runs=%d", err, len(r.Runs))
				}
				mediaOwnershipRun(t, len(r.Runs), r.Runs[0].Seed, r.Runs[0].Failed, r.Runs[0].Error)
				check = mediaReportOwnership{before, r, r.Config, r.ConfigHash, c.Seeds, r.Config.Seeds,
					c.Holdout, r.Config.Holdout, c.Validate, "red square"}
			}

			before := mediaOwnershipJSON(t, check.report)
			wantHash := mediaOwnershipHash(check.originalConfigJSON)
			if !bytes.Equal(mediaOwnershipJSON(t, check.reportConfig), check.originalConfigJSON) || check.configHash != wantHash {
				t.Fatal("report configuration or hash differs from the original run configuration")
			}
			t.Logf("initial report JSON sha256=%s", mediaOwnershipHash(before))

			originalHoldout := check.reportHoldout[0]
			// Change only the original caller's slices after the normal run returned.
			check.callerSeeds[0], check.callerHoldout[0] = 2, check.changedHoldout
			if err := check.validateCaller(); err != nil {
				t.Fatalf("changed caller configuration is invalid: %v", err)
			}
			if !bytes.Equal(mediaOwnershipJSON(t, check.report), before) {
				t.Error("report changed after mutating the caller configuration")
			}
			if mediaOwnershipHash(mediaOwnershipJSON(t, check.reportConfig)) != check.configHash {
				t.Error("report configuration no longer matches its hash")
			}

			// Editing the returned report itself must not change the caller's slices.
			check.reportSeeds[0], check.reportHoldout[0] = 3, originalHoldout
			if check.callerSeeds[0] != 2 || check.callerHoldout[0] != check.changedHoldout {
				t.Error("caller configuration changed after mutating report.Config")
			}
		})
	}
}
