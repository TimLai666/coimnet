package signal_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/TimLai666/coimnet/signal"
)

func TestProjectionRejectsNoncanonicalMetadataOrder(t *testing.T) {
	spec := projectionSpec()
	spec.Selection = signal.NeuronSelection{Mode: signal.SelectCellType, Value: "T"}
	p, err := signal.NewProjection(spec, projectionCandidates())
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &raw); err != nil {
		t.Fatal(err)
	}
	ids := p.Neurons()
	ids[0], ids[1] = ids[1], ids[0]
	idJSON, err := json.Marshal(ids)
	if err != nil {
		t.Fatal(err)
	}
	raw["neurons"] = idJSON
	raw["neuron_hash"] = json.RawMessage(fmt.Sprintf("\"%x\"", sha256.Sum256(idJSON)))
	invalid, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(invalid, &p); err == nil {
		t.Fatal("accepted noncanonical metadata order with a matching hash")
	}
	after, err := json.Marshal(p)
	if err != nil || !bytes.Equal(after, encoded) {
		t.Fatal("failed load changed projection")
	}
}

func TestProjectionRejectsLossyUnicode(t *testing.T) {
	spec := projectionSpec()
	spec.Source = "replacement:�"
	candidates := projectionCandidates()
	candidates[1].ID.ExternalID = "�"
	spec.Selection.Neurons[0] = candidates[1].ID
	p, err := signal.NewProjection(spec, candidates)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"source":"replacement:`, `"external_id":"`} {
		for _, invalid := range [][]byte{{255}, []byte(`\ud800`), []byte(`\udfff`), []byte(`\ud800\u0041`)} {
			t.Run(fmt.Sprintf("%s/%x", field, invalid), func(t *testing.T) {
				original := []byte(field + "�")
				replacement := append([]byte(field), invalid...)
				bad := bytes.Replace(encoded, original, replacement, 1)
				if bytes.Equal(bad, encoded) {
					t.Fatal("fixture did not change")
				}
				if _, err := signal.DecodeProjection(bytes.NewReader(bad)); err == nil {
					t.Fatalf("accepted lossy Unicode %x", invalid)
				}
			})
		}
	}
	for _, source := range []string{"emoji:😀", `literal:\ud800`, "quote:\" and slash:\\", "replacement:�"} {
		spec.Source = source
		p, err := signal.NewProjection(spec, candidates)
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := json.Marshal(p)
		if err != nil {
			t.Fatal(err)
		}
		encoded = bytes.ReplaceAll(encoded, []byte("😀"), []byte(`\ud83d\ude00`))
		decoded, err := signal.DecodeProjection(bytes.NewReader(encoded))
		if err != nil || decoded.Spec().Source != source {
			t.Fatalf("valid Unicode altered or rejected: %q, %v", source, err)
		}
	}
}
