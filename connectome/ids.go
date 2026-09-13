package connectome

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/TimLai666/coimnet/signal"
)

// ExternalID is a lossless external neuron identifier. Source files carry
// signed int64 IDs; negative values are rejected before this type exists.
type ExternalID struct {
	Namespace string
	Value     uint64
}

// ParseExternalID parses the canonical decimal form: digits only, no sign,
// no leading zeros except "0", within uint64.
func ParseExternalID(namespace, text string) (ExternalID, error) {
	if strings.TrimSpace(namespace) == "" {
		return ExternalID{}, fmt.Errorf("connectome: namespace must not be empty")
	}
	if text == "" || len(text) > 20 {
		return ExternalID{}, fmt.Errorf("connectome: external ID %q must be 1..20 decimal digits", text)
	}
	if text[0] == '0' && len(text) > 1 {
		return ExternalID{}, fmt.Errorf("connectome: external ID %q must not have leading zeros", text)
	}
	for _, r := range text {
		if r < '0' || r > '9' {
			return ExternalID{}, fmt.Errorf("connectome: external ID %q must contain only decimal digits", text)
		}
	}
	value, err := strconv.ParseUint(text, 10, 64)
	if err != nil {
		return ExternalID{}, fmt.Errorf("connectome: external ID %q is out of range: %w", text, err)
	}
	return ExternalID{Namespace: namespace, Value: value}, nil
}

// NeuronID returns the opaque signal identifier with the lossless decimal
// string, suitable for signal mappings and JSON.
func (id ExternalID) NeuronID() signal.NeuronID {
	return signal.NeuronID{Namespace: id.Namespace, ExternalID: strconv.FormatUint(id.Value, 10)}
}

// CanonicalKey returns the stable sort key: namespace, then the zero-padded
// decimal, so lexicographic order equals numeric order inside a namespace.
func CanonicalKey(namespace string, value uint64) string {
	return fmt.Sprintf("%s/%020d", namespace, value)
}

// signalID checks the namespace and parses the external ID of a signal
// identifier.
func signalID(id signal.NeuronID, namespace string) (uint64, error) {
	if id.Namespace != namespace {
		return 0, fmt.Errorf("connectome: neuron namespace %q does not match graph namespace %q", id.Namespace, namespace)
	}
	parsed, err := ParseExternalID(namespace, id.ExternalID)
	if err != nil {
		return 0, err
	}
	return parsed.Value, nil
}
