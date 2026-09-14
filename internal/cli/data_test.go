package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"hash/crc32"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/download"
)

func TestDataSourcesReportsAuditedMaleCNSSources(t *testing.T) {
	var out, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"data", "sources"}, &out, &stderr); err != nil {
		t.Fatalf("data sources: %v; stderr=%s", err, stderr.String())
	}
	var report struct {
		SchemaVersion string `json:"schema_version"`
		Dataset       string `json:"dataset"`
		Version       string `json:"version"`
		LicenseURL    string `json:"license_url"`
		AuditDate     string `json:"audit_date"`
		Sources       []struct {
			Filename     string `json:"filename"`
			URL          string `json:"url"`
			Role         string `json:"role"`
			LicenseURL   string `json:"license_url"`
			AuditDate    string `json:"audit_date"`
			CheckedAt    string `json:"checked_at"`
			LastModified string `json:"last_modified"`
			ETag         string `json:"etag"`
			SizeBytes    int64  `json:"size_bytes"`
			CRC32C       string `json:"crc32c"`
		} `json:"sources"`
	}
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("invalid sources JSON: %v\n%s", err, out.String())
	}
	if report.SchemaVersion == "" || report.Dataset != "MaleCNS" || report.Version != "v1.0" || report.LicenseURL != "https://creativecommons.org/licenses/by/4.0/" || report.AuditDate != "2026-09-13" {
		t.Fatalf("unexpected report identity: %#v", report)
	}
	if len(report.Sources) != 7 {
		t.Fatalf("source count = %d, want 7", len(report.Sources))
	}
	want := map[string]struct {
		url, role, etag, crc, lastModified, checkedAt string
		sizeBytes                                     int64
		auditDate                                     string
	}{
		"connectome-weights-male-cns-v1.0-minconf-0.5.feather": {
			url:          "https://storage.googleapis.com/flyem-male-cns/v1.0/connectome-data/flat-connectome/connectome-weights-male-cns-v1.0-minconf-0.5.feather",
			role:         "segment-to-segment connection weights",
			etag:         "f30e9dcca25cfd021bf1e7b3d975599e",
			crc:          "dKRPVQ==",
			sizeBytes:    1051241946,
			lastModified: "Wed, 03 Jun 2026 13:54:47 GMT",
			checkedAt:    "2026-09-13T08:24:01Z",
		},
		"body-annotations-male-cns-v1.0-minconf-0.5.feather": {
			url:          "https://storage.googleapis.com/flyem-male-cns/v1.0/connectome-data/flat-connectome/body-annotations-male-cns-v1.0-minconf-0.5.feather",
			role:         "curated cell class, type and side annotations",
			etag:         "50a7718770c57220f160ba4f431ab89e",
			crc:          "vjz9cg==",
			sizeBytes:    14483314,
			lastModified: "Wed, 03 Jun 2026 13:54:38 GMT",
			checkedAt:    "2026-09-13T08:24:04Z",
		},
		"body-neurotransmitters-male-cns-v1.0.feather": {
			url:          "https://storage.googleapis.com/flyem-male-cns/v1.0/connectome-data/flat-connectome/body-neurotransmitters-male-cns-v1.0.feather",
			role:         "aggregate neurotransmitter predictions per neuron",
			etag:         "3d842b12fe5c49eefade528d7dd24a1f",
			crc:          "jcpNFg==",
			sizeBytes:    43282834,
			lastModified: "Mon, 08 Jun 2026 05:01:39 GMT",
			checkedAt:    "2026-09-13T08:24:07Z",
		},
		"body-stats-male-cns-v1.0-minconf-0.5.feather": {
			url:          "https://storage.googleapis.com/flyem-male-cns/v1.0/connectome-data/flat-connectome/body-stats-male-cns-v1.0-minconf-0.5.feather",
			role:         "per body pre, post, downstream, synweight and rank counts",
			etag:         "404c3349c28580148e16815eb99f382a",
			crc:          "MGOCPQ==",
			sizeBytes:    778062826,
			lastModified: "Wed, 03 Jun 2026 13:54:48 GMT",
			checkedAt:    "2026-09-14T15:32:37Z",
			auditDate:    "2026-09-14",
		},
		"tbar-neurotransmitters-male-cns-v1.0.feather": {
			url:          "https://storage.googleapis.com/flyem-male-cns/v1.0/connectome-data/flat-connectome/tbar-neurotransmitters-male-cns-v1.0.feather",
			role:         "per synapse neurotransmitter probabilities at the presynaptic site",
			etag:         "51b02c11690662aedef28f86d394ff0d",
			crc:          "RrR5/g==",
			sizeBytes:    2651680218,
			lastModified: "Mon, 08 Jun 2026 05:02:07 GMT",
			checkedAt:    "2026-09-14T15:36:33Z",
			auditDate:    "2026-09-14",
		},
		"syn-partners-male-cns-v1.0-minconf-0.5.feather": {
			url:          "https://storage.googleapis.com/flyem-male-cns/v1.0/connectome-data/flat-connectome/syn-partners-male-cns-v1.0-minconf-0.5.feather",
			role:         "per synapse pre and post body, confidence and primary post ROI",
			etag:         "58efcf712f8c4d4de5f2ad51e97def76",
			crc:          "jTlNIA==",
			sizeBytes:    6777179098,
			lastModified: "Wed, 03 Jun 2026 13:55:42 GMT",
			checkedAt:    "2026-09-14T16:06:12Z",
			auditDate:    "2026-09-14",
		},
		"Neuprint_Meta.csv": {
			url:          "https://storage.googleapis.com/flyem-male-cns/v1.0/database/neuprint-inputs/Neuprint_Meta.csv",
			role:         "ROI hierarchy, ROI synapse counts and confidence thresholds",
			etag:         "ee9e55000a7e81813c959a88cbc47886",
			crc:          "qW0Qsg==",
			sizeBytes:    1247784,
			lastModified: "Mon, 08 Jun 2026 05:07:35 GMT",
			checkedAt:    "2026-09-14T15:32:40Z",
			auditDate:    "2026-09-14",
		},
	}
	for _, source := range report.Sources {
		wantSource, ok := want[source.Filename]
		if wantSource.auditDate == "" {
			wantSource.auditDate = report.AuditDate
		}
		if !ok || source.URL != wantSource.url || source.Role != wantSource.role || source.ETag != wantSource.etag || source.CRC32C != wantSource.crc || source.SizeBytes != wantSource.sizeBytes || source.LastModified != wantSource.lastModified || source.CheckedAt != wantSource.checkedAt || source.LicenseURL != report.LicenseURL || source.AuditDate != wantSource.auditDate {
			t.Fatalf("incomplete source record: %#v", source)
		}
		if !strings.HasPrefix(source.URL, "https://storage.googleapis.com/flyem-male-cns/v1.0/") {
			t.Fatalf("source URL is not official MaleCNS v1.0: %q", source.URL)
		}
	}
}

func TestDataDownloadRunE2EAndReceipt(t *testing.T) {
	body := []byte("coimnet-male-cns-cli-fixture")
	sha := sha256.Sum256(body)
	crc := testCRC32C(body)
	server := newDataDownloadServer(t, body)
	defer server.Close()

	path := filepath.Join(t.TempDir(), "source.bin")
	args := []string{"data", "download", "--url", server.URL, "--out", path, "--max-bytes", strconv.Itoa(len(body)), "--retries", "2", "--sha256", hex.EncodeToString(sha[:]), "--crc32c", crc, "--timeout", "15s"}
	var out, stderr bytes.Buffer
	if err := Run(context.Background(), args, &out, &stderr); err != nil {
		t.Fatalf("data download: %v; stderr=%s", err, stderr.String())
	}
	var receipt download.Receipt
	if err := json.Unmarshal(out.Bytes(), &receipt); err != nil {
		t.Fatalf("invalid receipt JSON: %v\n%s", err, out.String())
	}
	if receipt.SchemaVersion != download.SchemaVersion || receipt.URL != server.URL || receipt.Bytes != int64(len(body)) || receipt.SHA256 != hex.EncodeToString(sha[:]) || receipt.UpstreamCRC32C != crc || receipt.HashStatus != "upstream_verified" {
		t.Fatalf("unexpected receipt: %#v", receipt)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, body) {
		t.Fatalf("downloaded bytes = %q, want %q", got, body)
	}
}

func TestDataDownloadRejectsOverwriteChecksumAndSize(t *testing.T) {
	body := []byte("bounded-data-fixture")
	server := newDataDownloadServer(t, body)
	defer server.Close()

	dir := t.TempDir()
	path := filepath.Join(dir, "source.bin")
	args := []string{"data", "download", "--url", server.URL, "--out", path, "--max-bytes", strconv.Itoa(len(body)), "--timeout", "15s"}
	var out, stderr bytes.Buffer
	if err := Run(context.Background(), args, &out, &stderr); err != nil {
		t.Fatalf("initial data download: %v; stderr=%s", err, stderr.String())
	}
	if err := Run(context.Background(), args, &out, &stderr); err == nil {
		t.Fatal("accepted overwrite of complete target")
	}

	wrongPath := filepath.Join(dir, "wrong.bin")
	wrongArgs := []string{"data", "download", "--url", server.URL, "--out", wrongPath, "--max-bytes", strconv.Itoa(len(body)), "--sha256", strings.Repeat("0", sha256.Size*2), "--timeout", "15s"}
	if err := Run(context.Background(), wrongArgs, &out, &stderr); err == nil {
		t.Fatal("accepted wrong checksum")
	}
	if _, err := os.Stat(wrongPath); !os.IsNotExist(err) {
		t.Fatalf("checksum failure published target, stat error=%v", err)
	}

	largePath := filepath.Join(dir, "too-large.bin")
	largeArgs := []string{"data", "download", "--url", server.URL, "--out", largePath, "--max-bytes", strconv.Itoa(len(body) - 1), "--timeout", "15s"}
	if err := Run(context.Background(), largeArgs, &out, &stderr); err == nil {
		t.Fatal("accepted source over max-bytes")
	}
	if _, err := os.Stat(largePath); !os.IsNotExist(err) {
		t.Fatalf("max-bytes failure published target, stat error=%v", err)
	}
}

func TestDataDownloadValidatesOptionsAndHelp(t *testing.T) {
	server := newDataDownloadServer(t, []byte("options"))
	defer server.Close()
	dir := t.TempDir()
	base := []string{"data", "download", "--url", server.URL, "--out", filepath.Join(dir, "source.bin")}
	cases := [][]string{
		append(append([]string{}, base...), "--retries", "4", "--max-bytes", "1"),
		append(append([]string{}, base...), "--timeout", "0s", "--max-bytes", "1"),
		append(append([]string{}, base...), "--max-bytes", "0"),
		append(append([]string{}, base...), "--max-bytes", "1", "extra"),
	}
	for _, args := range cases {
		var out, stderr bytes.Buffer
		if err := Run(context.Background(), args, &out, &stderr); err == nil {
			t.Fatalf("accepted invalid arguments %v", args)
		}
	}
	for _, args := range [][]string{{"data", "--help"}, {"data", "sources", "--help"}, {"data", "download", "--help"}} {
		var out, stderr bytes.Buffer
		if err := Run(context.Background(), args, &out, &stderr); err != nil || out.Len() == 0 {
			t.Fatalf("help %v: err=%v output=%q stderr=%q", args, err, out.String(), stderr.String())
		}
	}
	writeErr := errors.New("data help output unavailable")
	for _, args := range [][]string{{"data", "--help"}, {"data", "sources", "--help"}, {"data", "download", "--help"}} {
		if err := Run(context.Background(), args, failDataWriter{err: writeErr}, io.Discard); !errors.Is(err, writeErr) {
			t.Fatalf("help %v error = %v, want %v", args, err, writeErr)
		}
	}
}

func TestDataDownloadReportsPublishedReceiptWriteFailure(t *testing.T) {
	body := []byte("receipt-write-fixture")
	server := newDataDownloadServer(t, body)
	defer server.Close()
	path := filepath.Join(t.TempDir(), "source.bin")
	args := []string{"data", "download", "--url", server.URL, "--out", path, "--max-bytes", strconv.Itoa(len(body)), "--timeout", "15s"}
	writeErr := errors.New("stdout unavailable")
	err := Run(context.Background(), args, failDataWriter{err: writeErr}, io.Discard)
	if !errors.Is(err, writeErr) || !strings.Contains(err.Error(), "published") || !strings.Contains(err.Error(), "receipt write failed") {
		t.Fatalf("error = %v, want published receipt-write context and writer error", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("published file missing after receipt write failure: %v", err)
	}
}

func newDataDownloadServer(t *testing.T, body []byte) *httptest.Server {
	t.Helper()
	etag := `"data-fixture-v1"`
	crc := testCRC32C(body)
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", etag)
		w.Header().Set("X-Goog-Hash", "crc32c="+crc)
		if r.Method == http.MethodHead {
			w.Header().Set("Content-Length", strconv.Itoa(len(body)))
			return
		}
		if r.Method != http.MethodGet || r.Header.Get("Range") != "" {
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
}

func testCRC32C(body []byte) string {
	sum := crc32.Checksum(body, crc32.MakeTable(crc32.Castagnoli))
	var encoded [4]byte
	binary.BigEndian.PutUint32(encoded[:], sum)
	return base64.StdEncoding.EncodeToString(encoded[:])
}

type failDataWriter struct{ err error }

func (w failDataWriter) Write([]byte) (int, error) { return 0, w.err }
