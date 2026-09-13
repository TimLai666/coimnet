package download

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	testPartSuffix     = ".part"
	testMetadataSuffix = ".part.meta.json"
	testLockSuffix     = ".lock"
)

func TestFetchFreshReturnsReceiptAndVerifiesHashes(t *testing.T) {
	body := []byte("male-cns-download-fixture")
	sha, crc := fixtureHashes(body)
	etag := `"fixture-v1"`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.Header().Set("X-Goog-Hash", "crc32c="+crc)
			writeHead(w, int64(len(body)), etag)
			return
		}
		if r.Method != http.MethodGet || r.Header.Get("Range") != "" {
			t.Errorf("unexpected request: %s Range=%q", r.Method, r.Header.Get("Range"))
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		writeBody(w, http.StatusOK, body, etag)
	}))
	defer server.Close()

	path := filepath.Join(t.TempDir(), "source.bin")
	receipt, err := Fetch(context.Background(), server.Client(), server.URL+"/source", path, Options{
		MaxBytes:       int64(len(body)),
		ExpectedSHA256: sha,
		ExpectedCRC32C: crc,
	})
	if err != nil {
		t.Fatal(err)
	}
	if receipt.SchemaVersion != SchemaVersion || receipt.URL != server.URL+"/source" || receipt.ETag != etag || receipt.Bytes != int64(len(body)) || receipt.SHA256 != sha || receipt.UpstreamCRC32C != crc || receipt.HashStatus != "upstream_verified" {
		t.Fatalf("unexpected receipt: %#v", receipt)
	}
	if _, err := time.Parse(time.RFC3339Nano, receipt.AcquiredAt); err != nil {
		t.Fatalf("invalid receipt acquisition time %q: %v", receipt.AcquiredAt, err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, body) {
		t.Fatalf("downloaded bytes = %q, want %q", got, body)
	}
	assertNoDownloadArtifacts(t, path)
}

func TestFetchRejectsInvalidResponseReadCount(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invalid-read-count.bin")
	client := &http.Client{Transport: invalidReadCountTransport{}}
	if _, err := Fetch(context.Background(), client, "http://invalid-read-count.test/source", path, Options{MaxBytes: 40000}); err == nil {
		t.Fatal("accepted an invalid response read count")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("target exists after invalid response read count, stat error = %v", err)
	}
}

func TestFetchPropagatesResponseBodyCloseErrors(t *testing.T) {
	body := []byte("response-body-close")
	cases := []struct {
		name      string
		headClose bool
		getClose  bool
	}{
		{name: "head", headClose: true},
		{name: "get", getClose: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "close-error.bin")
			client := &http.Client{Transport: closeErrorTransport{body: body, headClose: tc.headClose, getClose: tc.getClose}}
			if _, err := Fetch(context.Background(), client, "http://close-error.test/source", path, Options{MaxBytes: int64(len(body))}); err == nil || !strings.Contains(err.Error(), "close") {
				t.Fatalf("Fetch error = %v, want response body close error", err)
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatalf("target exists after response body close error, stat error = %v", err)
			}
		})
	}
}

func TestFetchReceiptRecordsReproducibleResponseHeaders(t *testing.T) {
	body := []byte("response-header-receipt")
	sha, crc := fixtureHashes(body)
	md5Sum := md5.Sum(body)
	md5Text := base64.StdEncoding.EncodeToString(md5Sum[:])
	etag := `"header-receipt-v1"`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Last-Modified", "Wed, 03 Jun 2026 13:54:38 GMT")
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("X-Goog-Hash", "crc32c="+crc+", md5="+md5Text)
		w.Header().Set("X-Private-Header", "must-not-be-recorded")
		if r.Method == http.MethodHead {
			writeHead(w, int64(len(body)), etag)
			return
		}
		writeBody(w, http.StatusOK, body, etag)
	}))
	defer server.Close()

	path := filepath.Join(t.TempDir(), "headers.bin")
	receipt, err := Fetch(context.Background(), server.Client(), server.URL, path, Options{MaxBytes: int64(len(body)), ExpectedSHA256: sha})
	if err != nil {
		t.Fatal(err)
	}
	if receipt.StatusCode != http.StatusOK || receipt.ContentLength != int64(len(body)) || receipt.ContentType != "application/octet-stream" || receipt.LastModified != "Wed, 03 Jun 2026 13:54:38 GMT" || receipt.AcceptRanges != "bytes" || receipt.ETag != etag || receipt.ProviderHashes["crc32c"] != crc || receipt.ProviderHashes["md5"] != md5Text {
		t.Fatalf("receipt response headers = %#v", receipt)
	}
	encoded, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "must-not-be-recorded") || strings.Contains(string(encoded), "x-private-header") {
		t.Fatalf("receipt recorded a non-allowlisted header: %s", encoded)
	}
}

func TestFetchAutomaticallyVerifiesProviderCRC32C(t *testing.T) {
	body := []byte("provider-crc32c-body")
	_, correctCRC := fixtureHashes(body)
	wrongBytes := []byte{0, 1, 2, 3}
	wrongCRC := base64.StdEncoding.EncodeToString(wrongBytes)
	etag := `"provider-crc-v1"`
	for _, tc := range []struct {
		name string
		crc  string
		want string
	}{
		{name: "matching", crc: correctCRC, want: "upstream_verified"},
		{name: "mismatch", crc: wrongCRC, want: "error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodHead {
					w.Header().Set("X-Goog-Hash", "crc32c="+tc.crc)
					writeHead(w, int64(len(body)), etag)
					return
				}
				writeBody(w, http.StatusOK, body, etag)
			}))
			defer server.Close()

			path := filepath.Join(t.TempDir(), "provider-crc.bin")
			receipt, err := Fetch(context.Background(), server.Client(), server.URL, path, Options{MaxBytes: int64(len(body))})
			if tc.want == "error" {
				if err == nil {
					t.Fatal("accepted a provider CRC32C mismatch")
				}
				if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
					t.Fatalf("target exists after provider CRC32C mismatch, stat error = %v", statErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if receipt.HashStatus != tc.want || receipt.UpstreamCRC32C != correctCRC {
				t.Fatalf("receipt = %#v, want status %q and CRC %q", receipt, tc.want, correctCRC)
			}
		})
	}
}

func TestFetchCancellationRetainsPartialForResume(t *testing.T) {
	body := []byte("cancel-and-resume-body")
	etag := `"resume-v1"`
	var requests sync.Mutex
	gets := 0
	started := make(chan struct{})
	var startedOnce sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			writeHead(w, int64(len(body)), etag)
			return
		}
		requests.Lock()
		gets++
		attempt := gets
		requests.Unlock()
		if attempt == 1 {
			w.Header().Set("ETag", etag)
			w.Header().Set("Content-Length", strconv.Itoa(len(body)))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(body[:3])
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			startedOnce.Do(func() { close(started) })
			<-r.Context().Done()
			return
		}
		if r.Header.Get("Range") != "bytes=3-" {
			t.Errorf("resume Range = %q, want bytes=3-", r.Header.Get("Range"))
		}
		w.Header().Set("ETag", etag)
		w.Header().Set("Content-Length", strconv.Itoa(len(body)-3))
		w.Header().Set("Content-Range", fmt.Sprintf("bytes 3-%d/%d", len(body)-1, len(body)))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(body[3:])
	}))
	defer server.Close()

	path := filepath.Join(t.TempDir(), "source.bin")
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := Fetch(ctx, server.Client(), server.URL+"/source", path, Options{MaxBytes: int64(len(body))})
		result <- err
	}()
	select {
	case <-started:
		deadline := time.Now().Add(5 * time.Second)
		for {
			info, statErr := os.Stat(path + testPartSuffix)
			if statErr == nil && info.Size() == 3 {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("partial did not reach the flushed prefix, stat error = %v", statErr)
			}
			time.Sleep(time.Millisecond)
		}
		cancel()
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("download did not start")
	}
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Fetch error = %v, want context.Canceled", err)
	}
	part, err := os.ReadFile(path + testPartSuffix)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(part, body[:3]) {
		t.Fatalf("retained partial = %q, want %q", part, body[:3])
	}
	metadata := readTestMetadata(t, path)
	if metadata.Bytes != 3 || metadata.URL != server.URL+"/source" || metadata.ETag != etag || metadata.Length != int64(len(body)) {
		t.Fatalf("unexpected retained metadata: %#v", metadata)
	}

	if _, err := Fetch(context.Background(), server.Client(), server.URL+"/source", path, Options{MaxBytes: int64(len(body))}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, body) {
		t.Fatalf("resumed bytes = %q, want %q", got, body)
	}
	requests.Lock()
	defer requests.Unlock()
	if gets != 2 {
		t.Fatalf("GET count = %d, want 2", gets)
	}
}

func TestFetchRetriesTransientInterruption(t *testing.T) {
	body := []byte("retry-body")
	etag := `"retry-v1"`
	var mu sync.Mutex
	gets := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			writeHead(w, int64(len(body)), etag)
			return
		}
		mu.Lock()
		gets++
		attempt := gets
		mu.Unlock()
		if attempt == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		writeBody(w, http.StatusOK, body, etag)
	}))
	defer server.Close()

	path := filepath.Join(t.TempDir(), "retry.bin")
	receipt, err := Fetch(context.Background(), server.Client(), server.URL, path, Options{MaxBytes: int64(len(body)), Retries: 1})
	if err != nil {
		t.Fatal(err)
	}
	if receipt.HashStatus != "locally_recorded" {
		t.Fatalf("hash status = %q, want locally_recorded", receipt.HashStatus)
	}
	mu.Lock()
	defer mu.Unlock()
	if gets != 2 {
		t.Fatalf("GET count = %d, want 2", gets)
	}
}

func TestFetchRejectsBadResumeResponsesWithoutPublishing(t *testing.T) {
	body := []byte("range-response-body")
	etag := `"range-v1"`
	cases := []struct {
		name         string
		status       int
		responseETag string
		contentRange string
	}{
		{name: "ignored-range", status: http.StatusOK, responseETag: etag},
		{name: "wrong-range", status: http.StatusPartialContent, responseETag: etag, contentRange: "bytes 4-17/18"},
		{name: "changed-get-etag", status: http.StatusPartialContent, responseETag: `"range-v2"`, contentRange: "bytes 3-17/18"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodHead {
					writeHead(w, int64(len(body)), etag)
					return
				}
				w.Header().Set("ETag", tc.responseETag)
				w.Header().Set("Content-Length", strconv.Itoa(len(body)-3))
				if tc.contentRange != "" {
					w.Header().Set("Content-Range", tc.contentRange)
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write(body[3:])
			}))
			defer server.Close()

			path := filepath.Join(t.TempDir(), "range.bin")
			seedTestPartial(t, path, server.URL, etag, int64(len(body)), body[:3])
			if _, err := Fetch(context.Background(), server.Client(), server.URL, path, Options{MaxBytes: int64(len(body))}); err == nil {
				t.Fatal("accepted invalid resume response")
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatalf("target exists after invalid response, stat error = %v", err)
			}
			part, err := os.ReadFile(path + testPartSuffix)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(part, body[:3]) {
				t.Fatalf("partial changed after invalid response: %q", part)
			}
		})
	}
}

func TestFetchRejectsChangedHeadETagAndContentLength(t *testing.T) {
	body := []byte("content-length-body")
	getCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			writeHead(w, int64(len(body)), `"new"`)
			return
		}
		getCalls++
	}))
	path := filepath.Join(t.TempDir(), "changed.bin")
	seedTestPartial(t, path, server.URL, `"old"`, int64(len(body)), body[:3])
	if _, err := Fetch(context.Background(), server.Client(), server.URL, path, Options{MaxBytes: int64(len(body))}); err == nil {
		t.Fatal("accepted changed source ETag")
	}
	server.Close()
	if getCalls != 0 {
		t.Fatalf("GET calls after changed HEAD ETag = %d, want 0", getCalls)
	}

	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			writeHead(w, int64(len(body)), `"length-v1"`)
			return
		}
		w.Header().Set("ETag", `"length-v1"`)
		w.Header().Set("Content-Length", strconv.Itoa(len(body)-1))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body[:len(body)-1])
	}))
	defer server.Close()
	freshPath := filepath.Join(t.TempDir(), "length.bin")
	if _, err := Fetch(context.Background(), server.Client(), server.URL, freshPath, Options{MaxBytes: int64(len(body))}); err == nil {
		t.Fatal("accepted GET Content-Length mismatch")
	}
	if _, err := os.Stat(freshPath); !os.IsNotExist(err) {
		t.Fatalf("target exists after content length mismatch, stat error = %v", err)
	}
}

func TestFetchRejectsWrongChecksumAndMaxBytes(t *testing.T) {
	body := []byte("checksum-body")
	etag := `"checksum-v1"`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			writeHead(w, int64(len(body)), etag)
			return
		}
		writeBody(w, http.StatusOK, body, etag)
	}))
	defer server.Close()

	wrongPath := filepath.Join(t.TempDir(), "wrong.bin")
	if _, err := Fetch(context.Background(), server.Client(), server.URL, wrongPath, Options{MaxBytes: int64(len(body)), ExpectedSHA256: strings.Repeat("0", sha256.Size*2)}); err == nil {
		t.Fatal("accepted wrong SHA-256")
	}
	if _, err := os.Stat(wrongPath); !os.IsNotExist(err) {
		t.Fatalf("target exists after wrong checksum, stat error = %v", err)
	}

	tooSmallPath := filepath.Join(t.TempDir(), "too-small.bin")
	if _, err := Fetch(context.Background(), server.Client(), server.URL, tooSmallPath, Options{MaxBytes: int64(len(body) - 1)}); err == nil {
		t.Fatal("accepted a source larger than MaxBytes")
	}
	if _, err := os.Stat(tooSmallPath); !os.IsNotExist(err) {
		t.Fatalf("target exists after MaxBytes rejection, stat error = %v", err)
	}
}

func TestFetchRejectsEmptyBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			writeHead(w, 3, `"empty-v1"`)
			return
		}
		w.Header().Set("Content-Length", "3")
		w.Header().Set("ETag", `"empty-v1"`)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	path := filepath.Join(t.TempDir(), "empty.bin")
	if _, err := Fetch(context.Background(), server.Client(), server.URL, path, Options{MaxBytes: 3}); err == nil {
		t.Fatal("accepted an empty response body")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("target exists after empty response, stat error = %v", err)
	}
}

func TestFetchRejectsInsufficientDiskSpace(t *testing.T) {
	body := []byte("disk-space-body")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			writeHead(w, int64(len(body)), `"disk-v1"`)
			return
		}
		t.Errorf("GET reached despite insufficient disk space")
	}))
	defer server.Close()
	old := diskFree
	diskFree = func(string) (uint64, error) { return 0, nil }
	defer func() { diskFree = old }()

	path := filepath.Join(t.TempDir(), "disk.bin")
	if _, err := Fetch(context.Background(), server.Client(), server.URL, path, Options{MaxBytes: int64(len(body))}); err == nil {
		t.Fatal("accepted insufficient disk space")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("target exists after disk rejection, stat error = %v", err)
	}
}

func TestFetchInvalidOptions(t *testing.T) {
	validURL := "http://example.invalid/source"
	validPath := filepath.Join(t.TempDir(), "invalid.bin")
	cases := []struct {
		name string
		ctx  context.Context
		cli  *http.Client
		url  string
		path string
		opt  Options
	}{
		{name: "nil-context", ctx: nil, cli: http.DefaultClient, url: validURL, path: validPath, opt: Options{MaxBytes: 1}},
		{name: "nil-client", ctx: context.Background(), cli: nil, url: validURL, path: validPath, opt: Options{MaxBytes: 1}},
		{name: "zero-max", ctx: context.Background(), cli: http.DefaultClient, url: validURL, path: validPath, opt: Options{}},
		{name: "negative-max", ctx: context.Background(), cli: http.DefaultClient, url: validURL, path: validPath, opt: Options{MaxBytes: -1}},
		{name: "negative-retries", ctx: context.Background(), cli: http.DefaultClient, url: validURL, path: validPath, opt: Options{MaxBytes: 1, Retries: -1}},
		{name: "too-many-retries", ctx: context.Background(), cli: http.DefaultClient, url: validURL, path: validPath, opt: Options{MaxBytes: 1, Retries: 4}},
		{name: "bad-sha", ctx: context.Background(), cli: http.DefaultClient, url: validURL, path: validPath, opt: Options{MaxBytes: 1, ExpectedSHA256: "bad"}},
		{name: "bad-crc", ctx: context.Background(), cli: http.DefaultClient, url: validURL, path: validPath, opt: Options{MaxBytes: 1, ExpectedCRC32C: "bad"}},
		{name: "empty-url", ctx: context.Background(), cli: http.DefaultClient, url: "", path: validPath, opt: Options{MaxBytes: 1}},
		{name: "empty-path", ctx: context.Background(), cli: http.DefaultClient, url: validURL, path: "", opt: Options{MaxBytes: 1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Fetch(tc.ctx, tc.cli, tc.url, tc.path, tc.opt); err == nil {
				t.Fatal("accepted invalid options")
			}
		})
	}
}

func TestFetchLockOverwriteAndConcurrentRace(t *testing.T) {
	body := []byte("exclusive-target-body")
	etag := `"exclusive-v1"`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			writeHead(w, int64(len(body)), etag)
			return
		}
		time.Sleep(50 * time.Millisecond)
		writeBody(w, http.StatusOK, body, etag)
	}))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "exclusive.bin")
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := Fetch(context.Background(), server.Client(), server.URL, path, Options{MaxBytes: int64(len(body))})
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	var successes int
	for err := range results {
		if err == nil {
			successes++
		} else if !strings.Contains(err.Error(), "lock") {
			t.Fatalf("concurrent loser error = %v, want lock error", err)
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent successes = %d, want 1", successes)
	}
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Fetch(context.Background(), server.Client(), server.URL, path, Options{MaxBytes: int64(len(body))}); err == nil {
		t.Fatal("overwrote an existing complete target")
	}
	current, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(current, original) {
		t.Fatal("failed overwrite changed target")
	}
	assertNoDownloadArtifacts(t, path)
}

func TestFetchPreservesUnknownPartialAndStaleLock(t *testing.T) {
	body := []byte("unknown-partial")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("request reached with unknown partial: %s", r.Method)
	}))
	defer server.Close()

	unknownPath := filepath.Join(t.TempDir(), "unknown.bin")
	unknown := []byte("do-not-delete")
	if err := os.WriteFile(unknownPath+testPartSuffix, unknown, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Fetch(context.Background(), server.Client(), server.URL, unknownPath, Options{MaxBytes: int64(len(body))}); err == nil {
		t.Fatal("accepted an unrecognized partial file")
	}
	got, err := os.ReadFile(unknownPath + testPartSuffix)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, unknown) {
		t.Fatal("unknown partial was changed")
	}

	lockPath := filepath.Join(t.TempDir(), "locked.bin")
	if err := os.WriteFile(lockPath+testLockSuffix, []byte("stale"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Fetch(context.Background(), server.Client(), server.URL, lockPath, Options{MaxBytes: int64(len(body))}); err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("stale lock error = %v", err)
	}
	if _, err := os.Stat(lockPath + testLockSuffix); err != nil {
		t.Fatalf("stale lock was removed: %v", err)
	}
}

func TestFetchRejectsStrictMetadata(t *testing.T) {
	body := []byte("strict-metadata")
	path := filepath.Join(t.TempDir(), "strict.bin")
	part := body[:3]
	if err := os.WriteFile(path+testPartSuffix, part, 0600); err != nil {
		t.Fatal(err)
	}
	metadata := fmt.Sprintf(`{"schema_version":%q,"schema_version":%q,"url":%q,"etag":%q,"length":%d,"bytes":%d}`, partialSchemaVersion, partialSchemaVersion, "http://source.invalid", `"v1"`, len(body), len(part))
	if err := os.WriteFile(path+testMetadataSuffix, []byte(metadata), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Fetch(context.Background(), http.DefaultClient, "http://source.invalid", path, Options{MaxBytes: int64(len(body))}); err == nil {
		t.Fatal("accepted duplicate metadata fields")
	}
	if got, err := os.ReadFile(path + testPartSuffix); err != nil || !bytes.Equal(got, part) {
		t.Fatalf("partial after metadata rejection = %q, err=%v", got, err)
	}
}

func TestFetchRejectsUnknownTrailingAndOversizedMetadata(t *testing.T) {
	body := []byte("strict-metadata")
	cases := []struct {
		name string
		data string
	}{
		{name: "unknown-field", data: fmt.Sprintf(`{"schema_version":%q,"url":%q,"etag":%q,"length":%d,"bytes":3,"unknown":true}`, partialSchemaVersion, "http://source.invalid", `"v1"`, len(body))},
		{name: "trailing-value", data: fmt.Sprintf(`{"schema_version":%q,"url":%q,"etag":%q,"length":%d,"bytes":3} false`, partialSchemaVersion, "http://source.invalid", `"v1"`, len(body))},
		{name: "oversized", data: strings.Repeat("x", maxMetadataBytes+1)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "strict.bin")
			if err := os.WriteFile(path+testPartSuffix, body[:3], 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path+testMetadataSuffix, []byte(tc.data), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := Fetch(context.Background(), http.DefaultClient, "http://source.invalid", path, Options{MaxBytes: int64(len(body))}); err == nil {
				t.Fatal("accepted invalid partial metadata")
			}
			got, err := os.ReadFile(path + testPartSuffix)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, body[:3]) {
				t.Fatalf("partial after metadata rejection = %q", got)
			}
		})
	}
}

func TestFetchRejectsNonRegularPartialArtifact(t *testing.T) {
	body := []byte("nonregular-partial")
	path := filepath.Join(t.TempDir(), "nonregular.bin")
	outside := filepath.Join(t.TempDir(), "outside.bin")
	if err := os.WriteFile(outside, []byte("outside"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, path+testPartSuffix); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	metadata := testMetadata{SchemaVersion: partialSchemaVersion, URL: "http://source.invalid", ETag: `"v1"`, Length: int64(len(body)), Bytes: int64(len(body))}
	data, err := json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+testMetadataSuffix, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Fetch(context.Background(), http.DefaultClient, "http://source.invalid", path, Options{MaxBytes: int64(len(body))}); err == nil {
		t.Fatal("accepted a symlink partial artifact")
	}
	info, err := os.Lstat(path + testPartSuffix)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("partial artifact mode = %v, want symlink preserved", info.Mode())
	}
	got, err := os.ReadFile(outside)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, []byte("outside")) {
		t.Fatalf("symlink target changed to %q", got)
	}
}

func TestFetchRejectsEmptyAndNonSuccessHead(t *testing.T) {
	cases := []struct {
		name string
		head func(http.ResponseWriter)
	}{
		{name: "non-success", head: func(w http.ResponseWriter) { w.WriteHeader(http.StatusBadGateway) }},
		{name: "missing-length", head: func(w http.ResponseWriter) { w.Header().Set("ETag", `"v1"`); w.WriteHeader(http.StatusOK) }},
		{name: "missing-etag", head: func(w http.ResponseWriter) { w.Header().Set("Content-Length", "3"); w.WriteHeader(http.StatusOK) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			getCalls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodHead {
					tc.head(w)
					return
				}
				getCalls++
			}))
			path := filepath.Join(t.TempDir(), "head.bin")
			if _, err := Fetch(context.Background(), server.Client(), server.URL, path, Options{MaxBytes: 3}); err == nil {
				t.Fatal("accepted invalid HEAD response")
			}
			server.Close()
			if getCalls != 0 {
				t.Fatalf("GET calls = %d, want 0", getCalls)
			}
		})
	}
}

type testMetadata struct {
	SchemaVersion string `json:"schema_version"`
	URL           string `json:"url"`
	ETag          string `json:"etag"`
	Length        int64  `json:"length"`
	Bytes         int64  `json:"bytes"`
}

func fixtureHashes(body []byte) (string, string) {
	sha := sha256.Sum256(body)
	crc := crc32.Checksum(body, crc32.MakeTable(crc32.Castagnoli))
	var encoded [4]byte
	binary.BigEndian.PutUint32(encoded[:], crc)
	return hex.EncodeToString(sha[:]), base64.StdEncoding.EncodeToString(encoded[:])
}

func writeHead(w http.ResponseWriter, length int64, etag string) {
	w.Header().Set("Content-Length", strconv.FormatInt(length, 10))
	w.Header().Set("ETag", etag)
	w.WriteHeader(http.StatusOK)
}

func writeBody(w http.ResponseWriter, status int, body []byte, etag string) {
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.Header().Set("ETag", etag)
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func seedTestPartial(t *testing.T, target, sourceURL, etag string, length int64, body []byte) {
	t.Helper()
	if err := os.WriteFile(target+testPartSuffix, body, 0600); err != nil {
		t.Fatal(err)
	}
	metadata := testMetadata{SchemaVersion: partialSchemaVersion, URL: sourceURL, ETag: etag, Length: length, Bytes: int64(len(body))}
	data, err := json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target+testMetadataSuffix, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func readTestMetadata(t *testing.T, target string) testMetadata {
	t.Helper()
	data, err := os.ReadFile(target + testMetadataSuffix)
	if err != nil {
		t.Fatal(err)
	}
	var metadata testMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		t.Fatal(err)
	}
	return metadata
}

func assertNoDownloadArtifacts(t *testing.T, target string) {
	t.Helper()
	for _, path := range []string{target + testPartSuffix, target + testMetadataSuffix, target + testLockSuffix} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("download artifact %s remains, stat error = %v", path, err)
		}
	}
}

type invalidReadCountTransport struct{}

func (invalidReadCountTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	header := make(http.Header)
	header.Set("Content-Length", "40000")
	header.Set("ETag", `"invalid-read-count-v1"`)
	var body io.ReadCloser = io.NopCloser(strings.NewReader(""))
	if request.Method == http.MethodGet {
		body = invalidReadCountBody{}
	}
	return &http.Response{StatusCode: http.StatusOK, Header: header, Body: body, ContentLength: 40000, Request: request}, nil
}

type invalidReadCountBody struct{}

func (invalidReadCountBody) Read(buffer []byte) (int, error) { return len(buffer) + 1, nil }

func (invalidReadCountBody) Close() error { return nil }

type closeErrorTransport struct {
	body      []byte
	headClose bool
	getClose  bool
}

func (transport closeErrorTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	header := make(http.Header)
	header.Set("Content-Length", strconv.Itoa(len(transport.body)))
	header.Set("ETag", `"close-error-v1"`)
	var body io.ReadCloser = io.NopCloser(strings.NewReader(""))
	if request.Method == http.MethodHead {
		if transport.headClose {
			body = closeErrorReadCloser{Reader: strings.NewReader("")}
		}
	} else if request.Method == http.MethodGet {
		body = io.NopCloser(strings.NewReader(string(transport.body)))
		if transport.getClose {
			body = closeErrorReadCloser{Reader: strings.NewReader(string(transport.body))}
		}
	}
	return &http.Response{StatusCode: http.StatusOK, Header: header, Body: body, ContentLength: int64(len(transport.body)), Request: request}, nil
}

type closeErrorReadCloser struct {
	io.Reader
}

func (closeErrorReadCloser) Close() error { return errors.New("response body close failed") }
