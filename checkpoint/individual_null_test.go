package checkpoint

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestIndividualNullNeverBecomesValidZero(t *testing.T) {
	payload := mustIndividualJSON(t, newCheckpointIndividual(t, false).Snapshot())
	for _, tc := range []struct{ name, from, to string }{
		{"update count", `"updates":0`, `"updates":null`},
		{"step count", `"steps":0`, `"steps":null`},
		{"decay", `"weight_decay":0`, `"weight_decay":null`},
		{"source zero", `"sources":[0]`, `"sources":[null]`},
		{"voltage zero", `"voltage":[0,0]`, `"voltage":[null,0]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := bytes.Replace(payload, []byte(tc.from), []byte(tc.to), 1)
			if bytes.Equal(changed, payload) {
				t.Fatal("fixture field missing")
			}
			path := filepath.Join(t.TempDir(), "bad.json")
			writeRaw(t, path, envelopeJSON(IndividualSchemaVersion, changed, checksumHex(changed)))
			if _, err := LoadIndividual(context.Background(), path); err == nil || !strings.Contains(err.Error(), "null") {
				t.Fatalf("must reject null before conversion to zero: %v", err)
			}
		})
	}
}
