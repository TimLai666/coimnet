package distill

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"

	"github.com/TimLai666/coimnet/teacher"
)

// Collect asks the teacher exactly once per train hash (RequestID
// "distill-"+hash, InputHash hash, Kind, Fields, ModelVersion) in sorted hash
// order and never for a holdout hash. teacher.ErrNoAnswer skips the input
// (NoAnswer++), any other error aborts. A text answer whose SHA-256 (lower hex
// of the decoded string's bytes) is a holdout hash is dropped (Deduplicated++):
// teacher text that repeats held-out content never becomes a target. Kind
// distribution is ErrUnsupportedKind here (the distribution distiller is the
// next ticket).
func (d LabelDistiller) Collect(ctx context.Context, split Split) ([]Example, CollectReport, error) {
	if err := split.Validate(); err != nil {
		return nil, CollectReport{}, err
	}
	if d.Kind == teacher.KindDistribution {
		return nil, CollectReport{}, ErrUnsupportedKind
	}
	if d.Teacher == nil {
		return nil, CollectReport{}, errors.New("distill: nil teacher")
	}
	if d.Encoder == nil {
		return nil, CollectReport{}, errors.New("distill: nil student encoder")
	}
	if ctx == nil {
		return nil, CollectReport{}, errors.New("distill: nil context")
	}
	if err := ctx.Err(); err != nil {
		return nil, CollectReport{}, err
	}

	holdout := make(map[string]bool, len(split.Holdout))
	for _, h := range split.Holdout {
		holdout[h] = true
	}
	train := append([]string(nil), split.Train...)
	sort.Strings(train)

	rep := CollectReport{Holdout: len(split.Holdout)}
	var examples []Example
	for _, hash := range train {
		if err := ctx.Err(); err != nil {
			return nil, rep, err
		}
		rep.Asked++
		rep.AskedHashes = append(rep.AskedHashes, hash)
		resp, err := d.Teacher.Ask(ctx, teacher.Request{
			RequestID:    "distill-" + hash,
			InputHash:    hash,
			Kind:         d.Kind,
			Fields:       d.Fields,
			ModelVersion: d.ModelVersion,
		})
		if errors.Is(err, teacher.ErrNoAnswer) {
			rep.NoAnswer++
			continue
		}
		if err != nil {
			return nil, rep, fmt.Errorf("distill: ask %s: %w", hash, err)
		}
		rep.Answered++
		if d.Kind == teacher.KindText {
			if text, derr := decodeStringAnswer(teacher.KindText, resp.Answer); derr == nil {
				sum := sha256.Sum256([]byte(text))
				if holdout[hex.EncodeToString(sum[:])] {
					rep.Deduplicated++
					continue
				}
			}
		}
		tgt, err := d.Encoder.Encode(d.Kind, resp.Answer)
		if err != nil {
			return nil, rep, fmt.Errorf("distill: encode %s: %w", hash, err)
		}
		examples = append(examples, Example{InputHash: hash, Target: tgt, Response: resp})
	}
	return examples, rep, nil
}
