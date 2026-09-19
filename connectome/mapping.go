package connectome

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

var (
	// ErrMappingEvidenceRequired is returned when an API that would relate the
	// neurons of two datasets is called without complete MappingEvidence.
	ErrMappingEvidenceRequired = errors.New("connectome: relating two datasets needs MappingEvidence")
	// ErrNoFormalMapping is returned when complete evidence was supplied and
	// the call is still refused: this build ships no mapping to apply, so even
	// a well documented claim cannot be turned into neuron correspondences.
	ErrNoFormalMapping = errors.New("connectome: this build provides no formal mapping between datasets")
)

// MappingEvidence is the only thing that can justify relating neurons of two
// datasets. All four fields are required: a mapping without a publisher, a
// method, a coverage statement and a version cannot be audited, so it is not
// evidence. Carrying evidence is necessary, never sufficient; the framework
// still refuses to splice two datasets it has no mapping for.
type MappingEvidence struct {
	Source   string `json:"source"`   // who published the mapping
	Method   string `json:"method"`   // how it was made
	Coverage string `json:"coverage"` // which neurons it covers
	Version  string `json:"version"`
}

// Validate reports every required field that is empty or blank, naming it, so
// a caller learns what the evidence is missing rather than that it is invalid.
func (e MappingEvidence) Validate() error {
	var missing []string
	for _, field := range [][2]string{
		{"source", e.Source},
		{"method", e.Method},
		{"coverage", e.Coverage},
		{"version", e.Version},
	} {
		if strings.TrimSpace(field[1]) == "" {
			missing = append(missing, field[0])
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf("connectome: mapping evidence is missing %s", strings.Join(missing, ", "))
}

// GraphComparison counts what two graphs of the same namespace share.
type GraphComparison struct {
	NamespaceA, NamespaceB         string
	NodesA, NodesB, EdgesA, EdgesB uint64
	SharedIDs                      uint64 // external IDs present in both
}

// Merge never merges: within one namespace it is undefined (rebuild from one
// manifest instead) and across namespaces it needs MappingEvidence and then
// still refuses with ErrNoFormalMapping, because no mapping is shipped. It
// exists so that the refusal is a documented API answer instead of a splice
// somebody writes by hand.
func Merge(ctx context.Context, a, b *Graph, evidence *MappingEvidence) (*Graph, error) {
	if err := checkPair(ctx, a, b); err != nil {
		return nil, err
	}
	if a.Namespace() == b.Namespace() {
		return nil, errors.New("connectome: merge within one namespace is not defined; rebuild from one manifest")
	}
	return nil, crossDatasetRefusal(a, b, evidence)
}

// Compare within one namespace counts nodes, edges and shared external IDs;
// across namespaces it needs MappingEvidence and then still refuses with
// ErrNoFormalMapping. A refused comparison reports no counts at all: two
// datasets that share no ID space have no shared count to report.
func Compare(ctx context.Context, a, b *Graph, evidence *MappingEvidence) (GraphComparison, error) {
	if err := checkPair(ctx, a, b); err != nil {
		return GraphComparison{}, err
	}
	if a.Namespace() != b.Namespace() {
		return GraphComparison{}, crossDatasetRefusal(a, b, evidence)
	}
	return GraphComparison{
		NamespaceA: a.Namespace(),
		NamespaceB: b.Namespace(),
		NodesA:     a.NodeCount(),
		NodesB:     b.NodeCount(),
		EdgesA:     a.EdgeCount(),
		EdgesB:     b.EdgeCount(),
		SharedIDs:  sharedIDs(a.nodeIDs, b.nodeIDs),
	}, nil
}

// checkPair is the precondition both entry points share.
func checkPair(ctx context.Context, a, b *Graph) error {
	if ctx == nil {
		return errors.New("connectome: nil context")
	}
	if a == nil || b == nil {
		return errors.New("connectome: nil graph")
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("connectome: %w", err)
	}
	return nil
}

// crossDatasetRefusal names the two datasets in every refusal, so a caller
// reads which pair was rejected and why: missing or incomplete evidence first,
// then the absence of a mapping to apply.
func crossDatasetRefusal(a, b *Graph, evidence *MappingEvidence) error {
	if evidence == nil {
		return fmt.Errorf("%w: %s and %s", ErrMappingEvidenceRequired, a.Namespace(), b.Namespace())
	}
	if err := evidence.Validate(); err != nil {
		return fmt.Errorf("%w: %s and %s: %w", ErrMappingEvidenceRequired, a.Namespace(), b.Namespace(), err)
	}
	return fmt.Errorf("%w: %s and %s", ErrNoFormalMapping, a.Namespace(), b.Namespace())
}

// sharedIDs counts the external IDs present in both ascending, distinct ID
// lists. It is only called inside one namespace, where the same value is the
// same neuron.
func sharedIDs(a, b []uint64) uint64 {
	var shared uint64
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		switch {
		case a[i] == b[j]:
			shared++
			i++
			j++
		case a[i] < b[j]:
			i++
		default:
			j++
		}
	}
	return shared
}
