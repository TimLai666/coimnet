package download

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// rangeServer serves body for HEAD and for both plain and ranged GET requests,
// recording the Range header of every GET so a resume can be checked.
func rangeServer(t *testing.T, body []byte, etag string, ranges *[]string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Accept-Ranges", "bytes")
		if r.Method == http.MethodHead {
			writeHead(w, int64(len(body)), etag)
			return
		}
		if r.Method != http.MethodGet {
			t.Errorf("unexpected request method %s", r.Method)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		rangeHeader := r.Header.Get("Range")
		if ranges != nil {
			*ranges = append(*ranges, rangeHeader)
		}
		if rangeHeader == "" {
			writeBody(w, http.StatusOK, body, etag)
			return
		}
		offset, err := strconv.ParseInt(strings.TrimSuffix(strings.TrimPrefix(rangeHeader, "bytes="), "-"), 10, 64)
		if err != nil || offset < 0 || offset >= int64(len(body)) {
			t.Errorf("unsupported Range header %q", rangeHeader)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", offset, len(body)-1, len(body)))
		writeBody(w, http.StatusPartialContent, body[offset:], etag)
	}))
}

func seedMismatchedPartial(t *testing.T, target, sourceURL, etag string, length int64, part []byte, metadataBytes int64) {
	t.Helper()
	if err := os.WriteFile(target+testPartSuffix, part, 0600); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(testMetadata{SchemaVersion: partialSchemaVersion, URL: sourceURL, ETag: etag, Length: length, Bytes: metadataBytes})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target+testMetadataSuffix, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func resumeFixtureBody(length int) []byte {
	body := make([]byte, length)
	for i := range body {
		body[i] = byte('a' + i%26)
	}
	return body
}

// TestFetchTruncatesPartLongerThanMetadataAndResumes covers the killed
// mid-transfer case: the .part carries bytes past the last checkpoint that were
// never fsynced, so they are discarded and the transfer resumes from the
// checkpoint. The published bytes must be identical to one uninterrupted run.
func TestFetchTruncatesPartLongerThanMetadataAndResumes(t *testing.T) {
	body := resumeFixtureBody(4096)
	etag := `"resume-truncate-v1"`
	var ranges []string
	server := rangeServer(t, body, etag, &ranges)
	defer server.Close()

	wholePath := filepath.Join(t.TempDir(), "whole.bin")
	whole, err := Fetch(context.Background(), server.Client(), server.URL+"/source", wholePath, Options{MaxBytes: int64(len(body))})
	if err != nil {
		t.Fatal(err)
	}
	if whole.ResumedFromBytes != 0 || whole.TruncatedBytes != 0 || whole.ReclaimedStaleLock {
		t.Fatalf("uninterrupted receipt carries resume fields: %#v", whole)
	}
	for _, field := range []string{"resumed_from_bytes", "truncated_bytes", "reclaimed_stale_lock"} {
		if strings.Contains(string(marshalReceipt(t, whole)), field) {
			t.Fatalf("uninterrupted receipt JSON carries %q", field)
		}
	}

	const checkpoint = 1024
	const unsynced = 700
	resumePath := filepath.Join(t.TempDir(), "resume.bin")
	part := append(append([]byte(nil), body[:checkpoint]...), bytes.Repeat([]byte{0xff}, unsynced)...)
	seedMismatchedPartial(t, resumePath, server.URL+"/source", etag, int64(len(body)), part, checkpoint)

	ranges = nil
	resumed, err := Fetch(context.Background(), server.Client(), server.URL+"/source", resumePath, Options{MaxBytes: int64(len(body))})
	if err != nil {
		t.Fatal(err)
	}
	if resumed.SHA256 != whole.SHA256 {
		t.Fatalf("resumed SHA-256 = %s, want %s", resumed.SHA256, whole.SHA256)
	}
	if resumed.ResumedFromBytes != checkpoint {
		t.Fatalf("receipt resumed_from_bytes = %d, want %d", resumed.ResumedFromBytes, checkpoint)
	}
	if resumed.TruncatedBytes != unsynced {
		t.Fatalf("receipt truncated_bytes = %d, want %d", resumed.TruncatedBytes, unsynced)
	}
	if len(ranges) != 1 || ranges[0] != fmt.Sprintf("bytes=%d-", checkpoint) {
		t.Fatalf("resume Range headers = %q, want one bytes=%d- request", ranges, checkpoint)
	}
	encoded := string(marshalReceipt(t, resumed))
	for _, field := range []string{"resumed_from_bytes", "truncated_bytes"} {
		if !strings.Contains(encoded, field) {
			t.Fatalf("resumed receipt JSON omits %q: %s", field, encoded)
		}
	}
	if strings.Contains(encoded, "reclaimed_stale_lock") {
		t.Fatalf("resumed receipt JSON carries reclaimed_stale_lock: %s", encoded)
	}
	got, err := os.ReadFile(resumePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, body) {
		t.Fatal("resumed bytes differ from the source body")
	}
	assertNoDownloadArtifacts(t, resumePath)
}

func marshalReceipt(t *testing.T, receipt Receipt) []byte {
	t.Helper()
	data, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// TestFetchRefusesPartShorterThanMetadata keeps the data-loss case a refusal:
// bytes the metadata claims were fsynced are missing, so nothing is resumed and
// nothing on disk is touched.
func TestFetchRefusesPartShorterThanMetadata(t *testing.T) {
	body := resumeFixtureBody(2048)
	etag := `"resume-short-v1"`
	server := rangeServer(t, body, etag, nil)
	defer server.Close()

	path := filepath.Join(t.TempDir(), "short.bin")
	part := body[:512]
	seedMismatchedPartial(t, path, server.URL+"/source", etag, int64(len(body)), part, 1024)

	if _, err := Fetch(context.Background(), server.Client(), server.URL+"/source", path, Options{MaxBytes: int64(len(body))}); err == nil || !strings.Contains(err.Error(), "preserved") {
		t.Fatalf("Fetch error = %v, want a refusal that preserves the partial", err)
	}
	got, err := os.ReadFile(path + testPartSuffix)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, part) {
		t.Fatal("short partial was changed")
	}
	if readTestMetadata(t, path).Bytes != 1024 {
		t.Fatal("short partial metadata was changed")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("target exists after refusal, stat error = %v", err)
	}
}

// deadPid runs and reaps a trivial child so its pid is known to be gone.
func deadPid(t *testing.T) int {
	t.Helper()
	command := exec.Command("/bin/sh", "-c", "exit 0")
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	pid := command.Process.Pid
	if err := command.Wait(); err != nil {
		t.Fatal(err)
	}
	return pid
}

func TestFetchReclaimsLockOfDeadProcess(t *testing.T) {
	body := resumeFixtureBody(256)
	etag := `"reclaim-v1"`
	server := rangeServer(t, body, etag, nil)
	defer server.Close()

	path := filepath.Join(t.TempDir(), "reclaim.bin")
	if err := os.WriteFile(path+testLockSuffix, []byte(fmt.Sprintf("pid=%d\n", deadPid(t))), 0600); err != nil {
		t.Fatal(err)
	}
	receipt, err := Fetch(context.Background(), server.Client(), server.URL+"/source", path, Options{MaxBytes: int64(len(body))})
	if err != nil {
		t.Fatal(err)
	}
	if !receipt.ReclaimedStaleLock {
		t.Fatalf("receipt reclaimed_stale_lock = false, want true: %#v", receipt)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, body) {
		t.Fatal("reclaimed download published the wrong bytes")
	}
	assertNoDownloadArtifacts(t, path)
}

func TestFetchRefusesLockOfLiveProcess(t *testing.T) {
	body := resumeFixtureBody(256)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("request reached despite a live lock holder: %s", r.Method)
	}))
	defer server.Close()

	path := filepath.Join(t.TempDir(), "live.bin")
	content := fmt.Appendf(nil, "pid=%d\n", os.Getpid())
	if err := os.WriteFile(path+testLockSuffix, content, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Fetch(context.Background(), server.Client(), server.URL+"/source", path, Options{MaxBytes: int64(len(body))}); err == nil || !strings.Contains(err.Error(), "lock") {
		t.Fatalf("Fetch error = %v, want a lock refusal", err)
	}
	got, err := os.ReadFile(path + testLockSuffix)
	if err != nil {
		t.Fatalf("live lock was removed: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Fatal("live lock content was changed")
	}
}

// TestFetchCheckpointsMetadataDuringTransfer holds the response open after the
// first chunks so the checkpoint written mid-transfer can be read from disk.
func TestFetchCheckpointsMetadataDuringTransfer(t *testing.T) {
	body := resumeFixtureBody(8192)
	etag := `"checkpoint-v1"`
	const checkpointBytes = 512
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			writeHead(w, int64(len(body)), etag)
			return
		}
		w.Header().Set("ETag", etag)
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Error("test server response is not flushable")
			return
		}
		_, _ = w.Write(body[:4096])
		flusher.Flush()
		<-release
		_, _ = w.Write(body[4096:])
		flusher.Flush()
	}))
	defer server.Close()

	path := filepath.Join(t.TempDir(), "checkpoint.bin")
	result := make(chan error, 1)
	go func() {
		_, err := Fetch(context.Background(), server.Client(), server.URL+"/source", path, Options{MaxBytes: int64(len(body)), CheckpointBytes: checkpointBytes})
		result <- err
	}()

	var observed int64
	deadline := time.Now().Add(10 * time.Second)
	for {
		data, err := os.ReadFile(path + testMetadataSuffix)
		if err == nil {
			var metadata testMetadata
			if json.Unmarshal(data, &metadata) == nil && metadata.Bytes >= checkpointBytes {
				observed = metadata.Bytes
				break
			}
		}
		if time.Now().After(deadline) {
			close(release)
			<-result
			t.Fatalf("no mid-transfer checkpoint reached %d bytes, last read error = %v", checkpointBytes, err)
		}
		time.Sleep(time.Millisecond)
	}
	close(release)
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	if observed >= int64(len(body)) {
		t.Fatalf("checkpoint observed %d bytes, want a mid-transfer count below %d", observed, len(body))
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, body) {
		t.Fatal("checkpointed download published the wrong bytes")
	}
	assertNoDownloadArtifacts(t, path)
}
