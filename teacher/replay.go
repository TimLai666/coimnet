package teacher

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

// Replay answers only from responses already recorded for the exact RequestID.
// It is the teacher a precise-resume run uses: the same request id yields the
// same response, a request id with no record is ErrNoAnswer, and nothing is
// ever fetched. The store is read-only through it.
type Replay struct {
	store   *RecordStore
	id      string
	version string
	byID    map[string]Response
}

// NewReplay opens the record file at path (validating every record exactly as
// OpenRecordStore does), then re-reads it once to index each response by its
// RequestID; when several records share a RequestID the last one wins. The file
// being read twice is deliberate: OpenRecordStore refuses held-out hashes and
// publishes no iteration over the store, so the index is built from the file.
// A missing file is an empty replay.
func NewReplay(path, id, version string) (*Replay, error) {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(version) == "" {
		return nil, fmt.Errorf("replay teacher: id and version must be non-blank")
	}

	store, err := OpenRecordStore(path, nil)
	if err != nil {
		return nil, err
	}

	byID, err := replayIndex(path)
	if err != nil {
		return nil, err
	}

	return &Replay{store: store, id: id, version: version, byID: byID}, nil
}

// Ask returns the recorded response for the request's RequestID; a RequestID
// with no record is ErrNoAnswer. The response is returned exactly as recorded:
// under a lawful record the RequestID and InputHash already equal the request's.
func (r Replay) Ask(ctx context.Context, req Request) (Response, error) {
	if ctx == nil {
		return Response{}, fmt.Errorf("replay teacher: nil context")
	}
	if err := ctx.Err(); err != nil {
		return Response{}, err
	}
	if err := req.Validate(); err != nil {
		return Response{}, fmt.Errorf("replay teacher: %w", err)
	}
	resp, ok := r.byID[req.RequestID]
	if !ok {
		return Response{}, ErrNoAnswer
	}
	return resp, nil
}

// Describe returns {ID, Version, DescriptorReplay}.
func (r *Replay) Describe() Descriptor {
	return Descriptor{ID: r.id, Version: r.version, Kind: DescriptorReplay}
}

// replayIndex re-reads path line by line and maps each response to its
// RequestID, keeping the last occurrence. Blank lines are skipped; any other
// bad line fails with an error naming the line number.
func replayIndex(path string) (map[string]Response, error) {
	byID := make(map[string]Response)

	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return byID, nil
	}
	if err != nil {
		return nil, fmt.Errorf("replay teacher: open %s: %w", path, err)
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), recordLineBufSize)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := sc.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var rec Record
		if err := json.Unmarshal(line, &rec); err != nil {
			return nil, fmt.Errorf("replay teacher %s line %d: %w", path, lineNo, err)
		}
		if err := rec.validate(); err != nil {
			return nil, fmt.Errorf("replay teacher %s line %d: %w", path, lineNo, err)
		}
		byID[rec.Request.RequestID] = rec.Response
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("replay teacher %s: read: %w", path, err)
	}
	return byID, nil
}
