package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/TimLai666/coimnet/download"
)

const (
	dataSourcesSchemaVersion = "coimnet-data-sources/v1"
	maleCNSDataset           = "MaleCNS"
	maleCNSVersion           = "v1.0"
	maleCNSLicenseURL        = "https://creativecommons.org/licenses/by/4.0/"
	maleCNSAuditDate         = "2026-09-13"
	// maleCNSParameterAuditDate is the audit date of the four files added for
	// the parameter adapter; the report's top level AuditDate stays the date
	// of the first audit, and each source carries its own.
	maleCNSParameterAuditDate = "2026-09-14"
)

// DataSourcesReport describes the fixed MaleCNS source metadata checked on
// AuditDate. Provider checksums and source timestamps are evidence snapshots;
// they are not claims about a changing latest version.
type DataSourcesReport struct {
	SchemaVersion string       `json:"schema_version"`
	Dataset       string       `json:"dataset"`
	Version       string       `json:"version"`
	LicenseURL    string       `json:"license_url"`
	AuditDate     string       `json:"audit_date"`
	Sources       []DataSource `json:"sources"`
}

// DataSource records one official source file and the provider metadata seen
// during the audit. A later download always performs a fresh HEAD request.
type DataSource struct {
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
}

var maleCNSSources = []DataSource{
	{
		Filename:     "connectome-weights-male-cns-v1.0-minconf-0.5.feather",
		URL:          "https://storage.googleapis.com/flyem-male-cns/v1.0/connectome-data/flat-connectome/connectome-weights-male-cns-v1.0-minconf-0.5.feather",
		Role:         "segment-to-segment connection weights",
		LicenseURL:   maleCNSLicenseURL,
		AuditDate:    maleCNSAuditDate,
		CheckedAt:    "2026-09-13T08:24:01Z",
		LastModified: "Wed, 03 Jun 2026 13:54:47 GMT",
		ETag:         "f30e9dcca25cfd021bf1e7b3d975599e",
		SizeBytes:    1051241946,
		CRC32C:       "dKRPVQ==",
	},
	{
		Filename:     "body-annotations-male-cns-v1.0-minconf-0.5.feather",
		URL:          "https://storage.googleapis.com/flyem-male-cns/v1.0/connectome-data/flat-connectome/body-annotations-male-cns-v1.0-minconf-0.5.feather",
		Role:         "curated cell class, type and side annotations",
		LicenseURL:   maleCNSLicenseURL,
		AuditDate:    maleCNSAuditDate,
		CheckedAt:    "2026-09-13T08:24:04Z",
		LastModified: "Wed, 03 Jun 2026 13:54:38 GMT",
		ETag:         "50a7718770c57220f160ba4f431ab89e",
		SizeBytes:    14483314,
		CRC32C:       "vjz9cg==",
	},
	{
		Filename:     "body-neurotransmitters-male-cns-v1.0.feather",
		URL:          "https://storage.googleapis.com/flyem-male-cns/v1.0/connectome-data/flat-connectome/body-neurotransmitters-male-cns-v1.0.feather",
		Role:         "aggregate neurotransmitter predictions per neuron",
		LicenseURL:   maleCNSLicenseURL,
		AuditDate:    maleCNSAuditDate,
		CheckedAt:    "2026-09-13T08:24:07Z",
		LastModified: "Mon, 08 Jun 2026 05:01:39 GMT",
		ETag:         "3d842b12fe5c49eefade528d7dd24a1f",
		SizeBytes:    43282834,
		CRC32C:       "jcpNFg==",
	},
	// Downloaded and scanned on 2026-09-14 for the parameter adapter; the
	// ETag, CRC32C and checked_at values below are the download receipts in
	// evidence/malecns-source-20260914/.
	{
		Filename:     "body-stats-male-cns-v1.0-minconf-0.5.feather",
		URL:          "https://storage.googleapis.com/flyem-male-cns/v1.0/connectome-data/flat-connectome/body-stats-male-cns-v1.0-minconf-0.5.feather",
		Role:         "per body pre, post, downstream, synweight and rank counts",
		LicenseURL:   maleCNSLicenseURL,
		AuditDate:    maleCNSParameterAuditDate,
		CheckedAt:    "2026-09-14T15:32:37Z",
		LastModified: "Wed, 03 Jun 2026 13:54:48 GMT",
		ETag:         "404c3349c28580148e16815eb99f382a",
		SizeBytes:    778062826,
		CRC32C:       "MGOCPQ==",
	},
	{
		Filename:     "tbar-neurotransmitters-male-cns-v1.0.feather",
		URL:          "https://storage.googleapis.com/flyem-male-cns/v1.0/connectome-data/flat-connectome/tbar-neurotransmitters-male-cns-v1.0.feather",
		Role:         "per synapse neurotransmitter probabilities at the presynaptic site",
		LicenseURL:   maleCNSLicenseURL,
		AuditDate:    maleCNSParameterAuditDate,
		CheckedAt:    "2026-09-14T15:36:33Z",
		LastModified: "Mon, 08 Jun 2026 05:02:07 GMT",
		ETag:         "51b02c11690662aedef28f86d394ff0d",
		SizeBytes:    2651680218,
		CRC32C:       "RrR5/g==",
	},
	{
		Filename:     "syn-partners-male-cns-v1.0-minconf-0.5.feather",
		URL:          "https://storage.googleapis.com/flyem-male-cns/v1.0/connectome-data/flat-connectome/syn-partners-male-cns-v1.0-minconf-0.5.feather",
		Role:         "per synapse pre and post body, confidence and primary post ROI",
		LicenseURL:   maleCNSLicenseURL,
		AuditDate:    maleCNSParameterAuditDate,
		CheckedAt:    "2026-09-14T16:06:12Z",
		LastModified: "Wed, 03 Jun 2026 13:55:42 GMT",
		ETag:         "58efcf712f8c4d4de5f2ad51e97def76",
		SizeBytes:    6777179098,
		CRC32C:       "jTlNIA==",
	},
	{
		Filename:     "Neuprint_Meta.csv",
		URL:          "https://storage.googleapis.com/flyem-male-cns/v1.0/database/neuprint-inputs/Neuprint_Meta.csv",
		Role:         "ROI hierarchy, ROI synapse counts and confidence thresholds",
		LicenseURL:   maleCNSLicenseURL,
		AuditDate:    maleCNSParameterAuditDate,
		CheckedAt:    "2026-09-14T15:32:40Z",
		LastModified: "Mon, 08 Jun 2026 05:07:35 GMT",
		ETag:         "ee9e55000a7e81813c959a88cbc47886",
		SizeBytes:    1247784,
		CRC32C:       "qW0Qsg==",
	},
}

func runData(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 || (len(args) == 1 && (args[0] == "--help" || args[0] == "-h")) {
		return writeDataUsage(stdout)
	}
	switch args[0] {
	case "sources":
		return runDataSources(args[1:], stdout, stderr)
	case "download":
		return runDataDownload(ctx, args[1:], stdout, stderr)
	case "inspect":
		return runDataInspect(ctx, args[1:], stdout, stderr)
	case "import":
		return runDataImport(ctx, args[1:], stdout, stderr)
	case "derive":
		return runDataDerive(ctx, args[1:], stdout, stderr)
	case "validate":
		return runDataValidate(ctx, args[1:], stdout, stderr)
	default:
		return fmt.Errorf("unknown data command; use data --help")
	}
}

func writeDataUsage(w io.Writer) error {
	_, err := fmt.Fprintln(w, "Usage: coimnet data sources | download [flags] | inspect [flags] | import [flags] | derive [flags] | validate [flags]\nList audited MaleCNS sources, download a versioned file, inspect every batch of a local Feather file, build graph views from a dataset manifest, derive dynamics parameters from the release files, or verify a graph store or parameter set.\nExamples:\n  coimnet data sources\n  coimnet data download --url URL --out FILE --max-bytes N\n  coimnet data inspect --input FILE --max-bytes N\n  coimnet data import --manifest FILE --out-store STORE\n  coimnet data derive --store STORE --rules RULES --out PARAMS\n  coimnet data validate --store STORE\n  coimnet data validate --params PARAMS --store STORE\nUse each command's --help for limits and errors. Unknown commands return an error.")
	return err
}

func runDataSources(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("data sources", flag.ContinueOnError)
	fs.SetOutput(stderr)
	usageOutput := &outputCapture{writer: stdout}
	fs.Usage = func() {
		fmt.Fprintln(usageOutput, "Usage: coimnet data sources\nList the audited official MaleCNS v1.0 source URLs, roles, license and checked provider metadata.\nExample: coimnet data sources\nErrors: positional arguments, unknown flags or output failure.")
	}
	if err := fs.Parse(args); errors.Is(err, flag.ErrHelp) {
		return usageOutput.Err()
	} else if err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("data sources takes no positional arguments")
	}
	report := DataSourcesReport{
		SchemaVersion: dataSourcesSchemaVersion,
		Dataset:       maleCNSDataset,
		Version:       maleCNSVersion,
		LicenseURL:    maleCNSLicenseURL,
		AuditDate:     maleCNSAuditDate,
		Sources:       append([]DataSource(nil), maleCNSSources...),
	}
	return writeJSON(stdout, report)
}

func runDataDownload(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("data download", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var sourceURL, targetPath, expectedSHA256, expectedCRC32C string
	var maxBytes int64
	var retries int
	var timeout time.Duration
	fs.StringVar(&sourceURL, "url", "", "source HTTP or HTTPS URL")
	fs.StringVar(&targetPath, "out", "", "new output file path")
	fs.Int64Var(&maxBytes, "max-bytes", 0, "required positive source size limit")
	fs.IntVar(&retries, "retries", 2, "additional attempts after a retryable failure (0..3)")
	fs.StringVar(&expectedSHA256, "sha256", "", "expected upstream SHA-256 in 64 hexadecimal characters")
	fs.StringVar(&expectedCRC32C, "crc32c", "", "expected upstream provider CRC32C in base64")
	fs.DurationVar(&timeout, "timeout", 15*time.Minute, "HTTP client timeout")
	usageOutput := &outputCapture{writer: stdout}
	fs.Usage = func() {
		fmt.Fprintln(usageOutput, "Usage: coimnet data download --url URL --out FILE --max-bytes N [flags]\nDownloads a bounded source, verifies provider CRC32C and optional authoritative checksums, and writes receipt JSON to stdout. SHA-256 is always computed locally; hash_status is upstream_verified when a provider or supplied authoritative checksum matches. Existing complete files are never overwritten.\nExample: coimnet data download --url https://storage.googleapis.com/flyem-male-cns/v1.0/connectome-data/flat-connectome/body-annotations-male-cns-v1.0-minconf-0.5.feather --out body-annotations-male-cns-v1.0-minconf-0.5.feather --max-bytes 20000000 --timeout 15m\nErrors: missing or invalid options, cancellation, checksum or transfer failure, file conflict or receipt output failure.\nOptions:")
		fs.SetOutput(usageOutput)
		fs.PrintDefaults()
		fs.SetOutput(stderr)
	}
	if err := fs.Parse(args); errors.Is(err, flag.ErrHelp) {
		return usageOutput.Err()
	} else if err != nil {
		return err
	}
	if fs.NArg() != 0 || sourceURL == "" || targetPath == "" || maxBytes <= 0 || retries < 0 || retries > 3 || timeout <= 0 {
		return fmt.Errorf("url, out and positive max-bytes are required; retries must be 0..3 and timeout must be positive; use data download --help")
	}
	client := &http.Client{Timeout: timeout}
	receipt, err := download.Fetch(ctx, client, sourceURL, targetPath, download.Options{
		MaxBytes:       maxBytes,
		Retries:        retries,
		ExpectedSHA256: expectedSHA256,
		ExpectedCRC32C: expectedCRC32C,
	})
	if err != nil {
		return err
	}
	if err := writeJSON(stdout, receipt); err != nil {
		return fmt.Errorf("download published at %q but receipt write failed: %w", targetPath, err)
	}
	return nil
}
