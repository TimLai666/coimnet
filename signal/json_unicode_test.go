package signal

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestSignalJSONRejectsLossyUnicode(t *testing.T) {
	spec := testSignalSpec(KindContinuous, 1, 0)
	spec.Channel = "replacement:�"
	s, err := NewSignal(spec)
	if err != nil {
		t.Fatal(err)
	}
	mapping, err := NewMapping(MappingSpec{SchemaVersion: testVersion(), Source: "replacement:�", InputShape: []int{1}, Neurons: []NeuronID{{Namespace: "fixture", ExternalID: "a"}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		value  any
		decode func([]byte) error
	}{
		{"signal", s, func(b []byte) error { _, err := DecodeSignal(bytes.NewReader(b)); return err }},
		{"mapping", mapping, func(b []byte) error { _, err := DecodeMapping(bytes.NewReader(b)); return err }},
		{"neuron", NeuronID{Namespace: "fixture", ExternalID: "replacement:�"}, func(b []byte) error { var id NeuronID; return json.Unmarshal(b, &id) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original, err := json.Marshal(tc.value)
			if err != nil {
				t.Fatal(err)
			}
			for _, replacement := range [][]byte{{255}, []byte(`\ud800`), []byte(`\udfff`)} {
				bad := bytes.ReplaceAll(original, []byte("�"), replacement)
				if bytes.Equal(original, bad) {
					t.Fatal("fixture unchanged")
				}
				if err := tc.decode(bad); err == nil {
					t.Fatalf("accepted lossy JSON text: %x", replacement)
				}
			}
			if err := tc.decode(original); err != nil {
				t.Fatal("rejected valid replacement rune", err)
			}
		})
	}
}

func TestNeuronIDRejectsInvalidUTF8BeforePersistence(t *testing.T) {
	for _, id := range []NeuronID{{Namespace: "fixture", ExternalID: string([]byte{255})}, {Namespace: string([]byte{255}), ExternalID: "a"}} {
		if _, err := json.Marshal(id); err == nil {
			t.Error("invalid UTF-8 neuron ID was silently normalized during marshal")
		}
		if _, err := NewMapping(MappingSpec{SchemaVersion: testVersion(), Source: "fixture", InputShape: []int{1}, Neurons: []NeuronID{id}}); err == nil {
			t.Error("mapping accepted invalid UTF-8 neuron ID")
		}
	}
}
