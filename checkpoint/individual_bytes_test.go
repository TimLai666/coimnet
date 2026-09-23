package checkpoint

import (
	"crypto/sha256"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/learning"
)

func TestIndividualBytesRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		want func(*testing.T) learning.IndividualSnapshot
	}{
		{name: "continuous", want: func(t *testing.T) learning.IndividualSnapshot {
			return newCheckpointIndividual(t, false).Snapshot()
		}},
		{name: "lif", want: func(t *testing.T) learning.IndividualSnapshot {
			return newCheckpointLIFIndividual(t).Snapshot()
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want := tc.want(t)
			encoded, err := EncodeIndividual(want)
			if err != nil {
				t.Fatalf("EncodeIndividual: %v", err)
			}
			if len(encoded) > maxCheckpointBytes {
				t.Fatalf("encoded individual size = %d, exceeds %d-byte limit", len(encoded), maxCheckpointBytes)
			}
			got, err := DecodeIndividual(encoded)
			if err != nil {
				t.Fatalf("DecodeIndividual: %v", err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatal("bytes round trip changed individual snapshot")
			}
		})
	}
}

func TestDecodeIndividualBytesRejectsMalformedJSON(t *testing.T) {
	payload, err := EncodeIndividual(newCheckpointIndividual(t, false).Snapshot())
	if err != nil {
		t.Fatalf("EncodeIndividual: %v", err)
	}

	mutatePayload := func(t *testing.T, mutate func(map[string]json.RawMessage)) []byte {
		t.Helper()
		var outer testEnvelope
		if err := json.Unmarshal(payload, &outer); err != nil {
			t.Fatalf("decode encoded envelope: %v", err)
		}
		var object map[string]json.RawMessage
		if err := json.Unmarshal(outer.Payload, &object); err != nil {
			t.Fatalf("decode encoded payload: %v", err)
		}
		mutate(object)
		changed, err := json.Marshal(object)
		if err != nil {
			t.Fatalf("marshal changed payload: %v", err)
		}
		return envelopeJSON(IndividualSchemaVersion, changed, checksumHex(changed))
	}

	cases := []struct {
		name string
		data func(*testing.T) []byte
		want string
	}{
		{
			name: "missing numeric",
			data: func(t *testing.T) []byte {
				return mutatePayload(t, func(object map[string]json.RawMessage) {
					optimizer := rawObject(t, object["optimizer"])
					options := rawObject(t, optimizer["options"])
					delete(options, "learning_rate")
					optimizer["options"] = mustRawJSON(t, options)
					object["optimizer"] = mustRawJSON(t, optimizer)
				})
			},
			want: "learning_rate",
		},
		{
			name: "null numeric",
			data: func(t *testing.T) []byte {
				return mutatePayload(t, func(object map[string]json.RawMessage) {
					optimizer := rawObject(t, object["optimizer"])
					options := rawObject(t, optimizer["options"])
					options["learning_rate"] = json.RawMessage(`null`)
					optimizer["options"] = mustRawJSON(t, options)
					object["optimizer"] = mustRawJSON(t, optimizer)
				})
			},
			want: "learning_rate",
		},
		{
			name: "corrupt checksum",
			data: func(t *testing.T) []byte {
				var outer testEnvelope
				if err := json.Unmarshal(payload, &outer); err != nil {
					t.Fatalf("decode encoded envelope: %v", err)
				}
				outer.Checksum = strings.Repeat("0", sha256.Size*2)
				return mustJSON(t, outer)
			},
			want: "checksum",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := DecodeIndividual(tc.data(t)); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("DecodeIndividual error = %v, want one mentioning %q", err, tc.want)
			}
		})
	}
}

func TestDecodeIndividualBytesEnforcesSizeLimit(t *testing.T) {
	oversized := make([]byte, maxCheckpointBytes+1)
	for i := range oversized {
		oversized[i] = ' '
	}
	if _, err := DecodeIndividual(oversized); err == nil || !strings.Contains(err.Error(), "64") {
		t.Fatalf("DecodeIndividual oversized input error = %v, want 64 MiB limit rejection", err)
	}
}

func rawObject(t *testing.T, raw json.RawMessage) map[string]json.RawMessage {
	t.Helper()
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		t.Fatalf("decode JSON object: %v", err)
	}
	return object
}

func mustRawJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal JSON object: %v", err)
	}
	return data
}
