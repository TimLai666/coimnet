package teacher

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

// RecordSchemaVersion is the JSONL record file schema.
const RecordSchemaVersion = "coimnet-teacher-records/v1"

// Record is one line of a record file: the request, the response and when it
// was recorded (the response's own Time).
type Record struct {
	SchemaVersion string   `json:"schema_version"`
	Request       Request  `json:"request"`
	Response      Response `json:"response"`
}

// RecordStore reads and appends JSONL records. The path of the record file must
// not be inside the directory of any declared test-input file: a store built
// with test-input hashes refuses to hold a record whose InputHash is one of
// them.
type RecordStore struct {
	path            string
	testInputHashes map[string]bool
	records         []Record
}

const recordLineBufSize = 4 * 1024 * 1024

// OpenRecordStore reads every line of path (a missing file is an empty store),
// validating each record (Record.SchemaVersion must equal RecordSchemaVersion,
// Request.Validate and Response.Validate must pass, and the response's
// RequestID and InputHash must equal the request's). testInputHashes are the
// SHA-256 hex hashes of the held-out test inputs; a record whose InputHash is
// in that set makes OpenRecordStore fail with an error naming the hash.
func OpenRecordStore(path string, testInputHashes []string) (*RecordStore, error) {
	hashes := make(map[string]bool, len(testInputHashes))
	for _, h := range testInputHashes {
		hashes[h] = true
	}
	s := &RecordStore{path: path, testInputHashes: hashes}

	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("record store: open %s: %w", path, err)
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
			return nil, fmt.Errorf("record store %s line %d: %w", path, lineNo, err)
		}
		if err := rec.validate(); err != nil {
			return nil, fmt.Errorf("record store %s line %d: %w", path, lineNo, err)
		}
		if hashes[rec.Request.InputHash] {
			return nil, fmt.Errorf("record store %s line %d: input hash %s is a held-out test input", path, lineNo, rec.Request.InputHash)
		}
		s.records = append(s.records, rec)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("record store %s: read: %w", path, err)
	}
	return s, nil
}

// Append validates and appends one record line (fsync not required) and keeps
// it in memory; the same test-input refusal applies.
func (s *RecordStore) Append(r Record) error {
	if err := r.validate(); err != nil {
		return err
	}
	if s.testInputHashes[r.Request.InputHash] {
		return fmt.Errorf("record store: input hash %s is a held-out test input", r.Request.InputHash)
	}
	line, err := json.Marshal(r)
	if err != nil {
		return fmt.Errorf("record store: marshal record: %w", err)
	}
	line = append(line, '\n')

	f, err := os.OpenFile(s.path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("record store: append %s: %w", s.path, err)
	}
	if _, err := f.Write(line); err != nil {
		f.Close()
		return fmt.Errorf("record store: append %s: %w", s.path, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("record store: append %s: %w", s.path, err)
	}
	s.records = append(s.records, r)
	return nil
}

// Lookup returns the newest record for an input hash and whether one exists.
func (s *RecordStore) Lookup(inputHash string) (Record, bool) {
	for i := len(s.records) - 1; i >= 0; i-- {
		if s.records[i].Request.InputHash == inputHash {
			return s.records[i], true
		}
	}
	return Record{}, false
}

// Len returns the number of records held.
func (s *RecordStore) Len() int {
	return len(s.records)
}

func (r Record) validate() error {
	if r.SchemaVersion != RecordSchemaVersion {
		return fmt.Errorf("schema_version %q does not match %q", r.SchemaVersion, RecordSchemaVersion)
	}
	if err := r.Request.Validate(); err != nil {
		return fmt.Errorf("request: %w", err)
	}
	if err := r.Response.Validate(); err != nil {
		return fmt.Errorf("response: %w", err)
	}
	if r.Response.RequestID != r.Request.RequestID {
		return fmt.Errorf("response request_id %q does not match request request_id %q", r.Response.RequestID, r.Request.RequestID)
	}
	if r.Response.InputHash != r.Request.InputHash {
		return fmt.Errorf("response input_hash %q does not match request input_hash %q", r.Response.InputHash, r.Request.InputHash)
	}
	return nil
}
