// Package download retrieves versioned source files with bounded, resumable
// writes. It never replaces an existing complete target.
package download

import (
	"bytes"
	"context"
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
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/TimLai666/coimnet/internal/jsonkey"
)

const (
	// SchemaVersion identifies the returned source-download receipt.
	SchemaVersion = "coimnet-download-receipt/v1"

	partialSchemaVersion = "coimnet-download-part/v1"
	partSuffix           = ".part"
	metadataSuffix       = ".part.meta.json"
	lockSuffix           = ".lock"

	maxMetadataBytes = 64 << 10
	maxHashReadBytes = 32 << 10
	maxLockBytes     = 1 << 10

	// defaultCheckpointBytes is how much a transfer may advance before the
	// partial file is fsynced and its byte count written back, so a killed
	// process loses at most this much progress.
	defaultCheckpointBytes = 64 << 20
)

// Options bounds a source transfer and optionally supplies authoritative
// checksums. ExpectedCRC32C is the base64 encoding used by GCS X-Goog-Hash.
type Options struct {
	MaxBytes       int64
	Retries        int
	ExpectedSHA256 string
	ExpectedCRC32C string
	// CheckpointBytes is how many transferred bytes may accumulate before the
	// partial file is fsynced and its metadata rewritten. Zero selects
	// defaultCheckpointBytes; negative values are rejected.
	CheckpointBytes int64
}

// Receipt records the source identity, selected public response metadata, the
// local SHA-256 and verification status. Only the provider hashes allowlist is
// retained; arbitrary response headers are never copied into a receipt.
type Receipt struct {
	SchemaVersion  string            `json:"schema_version"`
	URL            string            `json:"url"`
	ETag           string            `json:"etag"`
	StatusCode     int               `json:"status_code"`
	ContentLength  int64             `json:"content_length"`
	ContentType    string            `json:"content_type,omitempty"`
	LastModified   string            `json:"last_modified,omitempty"`
	AcceptRanges   string            `json:"accept_ranges,omitempty"`
	ProviderHashes map[string]string `json:"provider_hashes,omitempty"`
	AcquiredAt     string            `json:"acquired_at"`
	SHA256         string            `json:"sha256"`
	HashStatus     string            `json:"hash_status"`
	Bytes          int64             `json:"bytes"`
	// UpstreamCRC32C is kept for compatibility with the v1 receipt fields and
	// mirrors ProviderHashes["crc32c"] when the provider supplied that hash.
	UpstreamCRC32C string `json:"upstream_crc32c,omitempty"`
	// ResumedFromBytes is the checkpointed byte count a resumed transfer
	// continued from, TruncatedBytes the unsynced tail discarded before it and
	// ReclaimedStaleLock records that a lock left by a dead process was taken
	// over. All three are written only when they happened.
	ResumedFromBytes   int64 `json:"resumed_from_bytes,omitempty"`
	TruncatedBytes     int64 `json:"truncated_bytes,omitempty"`
	ReclaimedStaleLock bool  `json:"reclaimed_stale_lock,omitempty"`
}

type partialMetadata struct {
	SchemaVersion string `json:"schema_version"`
	URL           string `json:"url"`
	ETag          string `json:"etag"`
	Length        int64  `json:"length"`
	Bytes         int64  `json:"bytes"`
}

type sourceInfo struct {
	url            string
	etag           string
	length         int64
	statusCode     int
	contentType    string
	lastModified   string
	acceptRanges   string
	providerHashes map[string]string
	upstreamCRC32C string
	crcBytes       []byte
}

type validatedOptions struct {
	maxBytes        int64
	retries         int
	expectedSHA256  []byte
	expectedCRC32C  []byte
	checkpointBytes int64
}

var (
	errRetryable       = errors.New("retryable download failure")
	errResponseTooLong = errors.New("response exceeds declared length")
	diskFree           = diskFreeBytes
)

// Fetch obtains url into path, resuming only a recognized partial transfer.
// Before publication, errors leave the complete target unpublished. Cleanup,
// durability, or cancellation errors after the hardlink publication report
// that the target was published. Existing partial data and metadata are
// retained for inspection or a later resume.
func Fetch(ctx context.Context, client *http.Client, sourceURL, targetPath string, options Options) (receipt Receipt, retErr error) {
	if err := contextError(ctx); err != nil {
		return Receipt{}, err
	}
	if client == nil {
		return Receipt{}, fmt.Errorf("nil HTTP client")
	}
	validated, err := validateOptions(options)
	if err != nil {
		return Receipt{}, err
	}
	if sourceURL == "" {
		return Receipt{}, fmt.Errorf("source URL must not be empty")
	}
	parsed, err := url.Parse(sourceURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return Receipt{}, fmt.Errorf("source URL must be an HTTP or HTTPS URL")
	}
	if targetPath == "" {
		return Receipt{}, fmt.Errorf("target path must not be empty")
	}
	targetPath = filepath.Clean(targetPath)
	dir := filepath.Dir(targetPath)
	if err := validateDirectory(dir); err != nil {
		return Receipt{}, err
	}
	if err := rejectExistingTarget(targetPath); err != nil {
		return Receipt{}, err
	}

	lock, reclaimedStaleLock, err := acquireLock(targetPath + lockSuffix)
	if err != nil {
		return Receipt{}, err
	}
	defer func() {
		lockPath := targetPath + lockSuffix
		removeErr := os.Remove(lockPath)
		if removeErr != nil && !os.IsNotExist(removeErr) {
			retErr = errors.Join(retErr, fmt.Errorf("remove download lock: %w", removeErr))
		}
		if closeErr := lock.Close(); closeErr != nil {
			retErr = errors.Join(retErr, fmt.Errorf("close download lock: %w", closeErr))
		}
		if removeErr != nil && !os.IsNotExist(removeErr) {
			if retryErr := os.Remove(lockPath); retryErr != nil && !os.IsNotExist(retryErr) {
				retErr = errors.Join(retErr, fmt.Errorf("remove download lock after close: %w", retryErr))
			}
		}
	}()
	// A different process could have published after the first target check,
	// before this process acquired the lock.
	if err := rejectExistingTarget(targetPath); err != nil {
		return Receipt{}, err
	}

	metadata, partBytes, hasPartial, err := inspectPartial(targetPath, sourceURL)
	if err != nil {
		return Receipt{}, err
	}
	var resumedFromBytes, truncatedBytes int64
	partPath := targetPath + partSuffix
	metadataPath := targetPath + metadataSuffix
	var source sourceInfo
	haveSource := false
	var lastHeadErr error
	var part *os.File
	defer func() {
		if part != nil {
			if closeErr := part.Close(); closeErr != nil {
				retErr = errors.Join(retErr, fmt.Errorf("close partial download: %w", closeErr))
			}
		}
	}()

	for attempt := 0; attempt <= validated.retries; attempt++ {
		if err := contextError(ctx); err != nil {
			return Receipt{}, err
		}
		current, err := headSource(ctx, client, sourceURL)
		if err != nil {
			lastHeadErr = err
			if !isRetryable(err) || attempt == validated.retries {
				return Receipt{}, err
			}
			continue
		}
		if current.length > validated.maxBytes {
			return Receipt{}, fmt.Errorf("source length %d exceeds MaxBytes %d", current.length, validated.maxBytes)
		}
		if haveSource {
			if err := sameSource(source, current); err != nil {
				return Receipt{}, err
			}
			if source.upstreamCRC32C == "" && current.upstreamCRC32C != "" {
				source.upstreamCRC32C = current.upstreamCRC32C
				source.crcBytes = current.crcBytes
			}
		} else {
			source = current
			haveSource = true
			if validated.expectedCRC32C != nil && source.crcBytes != nil && !bytes.Equal(validated.expectedCRC32C, source.crcBytes) {
				return Receipt{}, fmt.Errorf("upstream CRC32C does not match ExpectedCRC32C")
			}
			if hasPartial {
				if metadata.Length != source.length || metadata.ETag != source.etag {
					return Receipt{}, fmt.Errorf("partial metadata does not match current source ETag or length")
				}
				// The bytes past the last checkpoint were never fsynced, so
				// they are discarded rather than trusted.
				if partBytes > metadata.Bytes {
					if err := truncatePart(partPath, metadata.Bytes); err != nil {
						return Receipt{}, err
					}
					truncatedBytes = partBytes - metadata.Bytes
				}
				resumedFromBytes = metadata.Bytes
			} else {
				metadata = partialMetadata{SchemaVersion: partialSchemaVersion, URL: sourceURL, ETag: source.etag, Length: source.length, Bytes: 0}
				if err := createPart(partPath); err != nil {
					return Receipt{}, err
				}
				if err := writeMetadata(context.Background(), metadataPath, metadata); err != nil {
					cleanupErr := os.Remove(partPath)
					return Receipt{}, errors.Join(err, wrapError("remove unregistered partial", cleanupErr))
				}
				hasPartial = true
			}
		}
		if part == nil {
			part, err = openPartForWrite(partPath, metadata.Bytes)
			if err != nil {
				return Receipt{}, fmt.Errorf("open partial download: %w", err)
			}
		}
		if metadata.Bytes > source.length {
			return Receipt{}, fmt.Errorf("partial byte count %d exceeds source length %d", metadata.Bytes, source.length)
		}
		if metadata.Bytes < source.length {
			if err := checkDiskSpace(dir, source.length-metadata.Bytes); err != nil {
				return Receipt{}, err
			}
			attemptErr := downloadAttempt(ctx, client, part, &source, &metadata, metadataPath, validated.checkpointBytes)
			if attemptErr != nil {
				if ctxErr := contextError(ctx); ctxErr != nil {
					return Receipt{}, errors.Join(ctxErr, attemptErr)
				}
				if !isRetryable(attemptErr) || attempt == validated.retries {
					return Receipt{}, attemptErr
				}
				continue
			}
		}
		break
	}

	if !haveSource {
		if lastHeadErr != nil {
			return Receipt{}, lastHeadErr
		}
		return Receipt{}, fmt.Errorf("internal error: source metadata is unavailable")
	}
	if part == nil {
		return Receipt{}, fmt.Errorf("internal error: partial download is not open")
	}
	if err := part.Sync(); err != nil {
		return Receipt{}, fmt.Errorf("sync complete partial download: %w", err)
	}
	if err := part.Close(); err != nil {
		part = nil
		return Receipt{}, fmt.Errorf("close complete partial download: %w", err)
	}
	part = nil
	if err := contextError(ctx); err != nil {
		return Receipt{}, err
	}
	shaText, crcText, err := hashFile(ctx, partPath, source.length)
	if err != nil {
		return Receipt{}, err
	}
	upstreamVerified := false
	if source.crcBytes != nil {
		decoded, decodeErr := base64.StdEncoding.DecodeString(crcText)
		if decodeErr != nil || !bytes.Equal(source.crcBytes, decoded) {
			return Receipt{}, fmt.Errorf("download CRC32C does not match provider X-Goog-Hash")
		}
		upstreamVerified = true
	}
	if validated.expectedSHA256 != nil {
		decoded, decodeErr := hex.DecodeString(shaText)
		if decodeErr != nil || !bytes.Equal(validated.expectedSHA256, decoded) {
			return Receipt{}, fmt.Errorf("download SHA-256 does not match ExpectedSHA256")
		}
		upstreamVerified = true
	}
	if validated.expectedCRC32C != nil {
		decoded, decodeErr := base64.StdEncoding.DecodeString(crcText)
		if decodeErr != nil || !bytes.Equal(validated.expectedCRC32C, decoded) {
			return Receipt{}, fmt.Errorf("download CRC32C does not match ExpectedCRC32C")
		}
		upstreamVerified = true
	}
	if err := contextError(ctx); err != nil {
		return Receipt{}, err
	}

	if err := os.Link(partPath, targetPath); err != nil {
		return Receipt{}, fmt.Errorf("publish downloaded file without overwrite: %w", err)
	}
	removePartErr := os.Remove(partPath)
	// The complete target now owns the data. A failed cleanup is reported below
	// while retaining the published target.
	removeMetadataErr := os.Remove(metadataPath)
	syncErr := syncDirectory(context.Background(), dir)
	contextErr := contextError(ctx)
	if removePartErr != nil {
		retErr = errors.Join(retErr, fmt.Errorf("download published but partial cleanup failed: %w", removePartErr))
	}
	if removeMetadataErr != nil && !os.IsNotExist(removeMetadataErr) {
		retErr = errors.Join(retErr, fmt.Errorf("download published but metadata cleanup failed: %w", removeMetadataErr))
	}
	if syncErr != nil {
		retErr = errors.Join(retErr, fmt.Errorf("download published but durability unconfirmed: %w", syncErr))
	}
	if contextErr != nil {
		retErr = errors.Join(retErr, fmt.Errorf("download published; context ended after publication: %w", contextErr))
	}
	if retErr != nil {
		return Receipt{}, retErr
	}

	receipt = Receipt{
		SchemaVersion:  SchemaVersion,
		URL:            sourceURL,
		ETag:           source.etag,
		StatusCode:     source.statusCode,
		ContentLength:  source.length,
		ContentType:    source.contentType,
		LastModified:   source.lastModified,
		AcceptRanges:   source.acceptRanges,
		ProviderHashes: cloneProviderHashes(source.providerHashes),
		AcquiredAt:     time.Now().UTC().Format(time.RFC3339Nano),
		SHA256:         shaText,
		HashStatus:     "locally_recorded",
		Bytes:          source.length,
		UpstreamCRC32C: source.upstreamCRC32C,

		ResumedFromBytes:   resumedFromBytes,
		TruncatedBytes:     truncatedBytes,
		ReclaimedStaleLock: reclaimedStaleLock,
	}
	if upstreamVerified {
		receipt.HashStatus = "upstream_verified"
	}
	return receipt, nil
}

func validateOptions(options Options) (validatedOptions, error) {
	if options.MaxBytes <= 0 {
		return validatedOptions{}, fmt.Errorf("MaxBytes must be positive")
	}
	if options.Retries < 0 || options.Retries > 3 {
		return validatedOptions{}, fmt.Errorf("Retries must be between 0 and 3")
	}
	if options.CheckpointBytes < 0 {
		return validatedOptions{}, fmt.Errorf("CheckpointBytes must not be negative")
	}
	checkpointBytes := options.CheckpointBytes
	if checkpointBytes == 0 {
		checkpointBytes = defaultCheckpointBytes
	}
	validated := validatedOptions{maxBytes: options.MaxBytes, retries: options.Retries, checkpointBytes: checkpointBytes}
	if options.ExpectedSHA256 != "" {
		if len(options.ExpectedSHA256) != sha256.Size*2 {
			return validatedOptions{}, fmt.Errorf("ExpectedSHA256 must be 64 hexadecimal characters")
		}
		decoded, err := hex.DecodeString(options.ExpectedSHA256)
		if err != nil {
			return validatedOptions{}, fmt.Errorf("invalid ExpectedSHA256: %w", err)
		}
		validated.expectedSHA256 = decoded
	}
	if options.ExpectedCRC32C != "" {
		decoded, err := decodeCRC32C(options.ExpectedCRC32C)
		if err != nil {
			return validatedOptions{}, fmt.Errorf("invalid ExpectedCRC32C: %w", err)
		}
		validated.expectedCRC32C = decoded
	}
	return validated, nil
}

func headSource(ctx context.Context, client *http.Client, sourceURL string) (source sourceInfo, retErr error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodHead, sourceURL, nil)
	if err != nil {
		return sourceInfo{}, fmt.Errorf("create HEAD request: %w", err)
	}
	request.Header.Set("Accept-Encoding", "identity")
	response, err := client.Do(request)
	if err != nil {
		if ctxErr := contextError(ctx); ctxErr != nil {
			return sourceInfo{}, ctxErr
		}
		return sourceInfo{}, retryablef("HEAD request failed: %v", err)
	}
	if response.Body != nil {
		defer func() {
			if closeErr := response.Body.Close(); closeErr != nil {
				retErr = errors.Join(retErr, fmt.Errorf("close HEAD response body: %w", closeErr))
			}
		}()
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		if retryableStatus(response.StatusCode) {
			return sourceInfo{}, retryablef("HEAD returned HTTP %d", response.StatusCode)
		}
		return sourceInfo{}, fmt.Errorf("HEAD returned HTTP %d", response.StatusCode)
	}
	length, err := responseLength(response)
	if err != nil {
		return sourceInfo{}, fmt.Errorf("invalid HEAD Content-Length: %w", err)
	}
	etag := strings.TrimSpace(response.Header.Get("ETag"))
	if etag == "" {
		return sourceInfo{}, fmt.Errorf("HEAD response must include ETag")
	}
	upstream, crcBytes, err := upstreamCRC(response.Header)
	if err != nil {
		return sourceInfo{}, fmt.Errorf("invalid upstream CRC32C: %w", err)
	}
	hashes, err := providerHashes(response.Header)
	if err != nil {
		return sourceInfo{}, fmt.Errorf("invalid provider hashes: %w", err)
	}
	return sourceInfo{
		url:            sourceURL,
		etag:           etag,
		length:         length,
		statusCode:     response.StatusCode,
		contentType:    strings.TrimSpace(response.Header.Get("Content-Type")),
		lastModified:   strings.TrimSpace(response.Header.Get("Last-Modified")),
		acceptRanges:   strings.TrimSpace(response.Header.Get("Accept-Ranges")),
		providerHashes: hashes,
		upstreamCRC32C: upstream,
		crcBytes:       crcBytes,
	}, nil
}

func sameSource(first, second sourceInfo) error {
	if first.etag != second.etag || first.length != second.length {
		return fmt.Errorf("source ETag or length changed during download")
	}
	if first.upstreamCRC32C != "" && second.upstreamCRC32C != "" && first.upstreamCRC32C != second.upstreamCRC32C {
		return fmt.Errorf("source CRC32C changed during download")
	}
	return nil
}

func downloadAttempt(ctx context.Context, client *http.Client, part *os.File, source *sourceInfo, metadata *partialMetadata, metadataPath string, checkpointBytes int64) (retErr error) {
	offset := metadata.Bytes
	if offset < 0 || offset > source.length {
		return fmt.Errorf("partial offset %d is outside source length %d", offset, source.length)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, source.url, nil)
	if err != nil {
		return fmt.Errorf("create GET request: %w", err)
	}
	request.Header.Set("Accept-Encoding", "identity")
	request.Header.Set("If-Match", source.etag)
	if offset > 0 {
		request.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
	}
	response, err := client.Do(request)
	if err != nil {
		if ctxErr := contextError(ctx); ctxErr != nil {
			return ctxErr
		}
		return retryablef("GET request failed: %v", err)
	}
	if response.Body == nil {
		return fmt.Errorf("GET response has no body")
	}
	defer func() {
		if closeErr := response.Body.Close(); closeErr != nil {
			retErr = errors.Join(retErr, fmt.Errorf("close GET response body: %w", closeErr))
		}
	}()
	if offset == 0 {
		if response.StatusCode != http.StatusOK {
			if retryableStatus(response.StatusCode) {
				return retryablef("GET returned HTTP %d", response.StatusCode)
			}
			return fmt.Errorf("GET returned HTTP %d, want 200", response.StatusCode)
		}
		if response.Header.Get("Content-Range") != "" {
			return fmt.Errorf("unexpected Content-Range on a fresh GET")
		}
	} else {
		if response.StatusCode != http.StatusPartialContent {
			if retryableStatus(response.StatusCode) {
				return retryablef("range GET returned HTTP %d", response.StatusCode)
			}
			return fmt.Errorf("range GET returned HTTP %d, want 206", response.StatusCode)
		}
		start, end, total, err := parseContentRange(response.Header.Get("Content-Range"))
		if err != nil {
			return fmt.Errorf("invalid Content-Range: %w", err)
		}
		if start != offset || end != source.length-1 || total != source.length {
			return fmt.Errorf("Content-Range bytes %d-%d/%d, want %d-%d/%d", start, end, total, offset, source.length-1, source.length)
		}
	}
	etag := strings.TrimSpace(response.Header.Get("ETag"))
	if etag == "" || etag != source.etag {
		return fmt.Errorf("GET ETag does not match HEAD ETag")
	}
	length, err := responseLength(response)
	if err != nil {
		return fmt.Errorf("invalid GET Content-Length: %w", err)
	}
	wantLength := source.length - offset
	if length != wantLength {
		return fmt.Errorf("GET Content-Length %d, want %d", length, wantLength)
	}
	upstream, crcBytes, err := upstreamCRC(response.Header)
	if err != nil {
		return fmt.Errorf("invalid GET upstream CRC32C: %w", err)
	}
	if source.upstreamCRC32C != "" && upstream != "" && source.upstreamCRC32C != upstream {
		return fmt.Errorf("GET upstream CRC32C does not match HEAD")
	}
	if source.upstreamCRC32C == "" && upstream != "" {
		source.upstreamCRC32C = upstream
		source.crcBytes = crcBytes
	}
	hashes, err := providerHashes(response.Header)
	if err != nil {
		return fmt.Errorf("invalid provider hashes: %w", err)
	}
	if err := mergeProviderHashes(&source.providerHashes, hashes); err != nil {
		return err
	}
	if _, err := part.Seek(offset, io.SeekStart); err != nil {
		return fmt.Errorf("seek partial download: %w", err)
	}
	checkpoint := func(written int64) error {
		return retainPartial(part, metadataPath, metadata, offset+written)
	}
	written, copyErr := copyResponse(ctx, response.Body, part, wantLength, checkpointBytes, checkpoint)
	if errors.Is(copyErr, errResponseTooLong) {
		rollbackErr := rollbackPartial(part, offset)
		// A checkpoint may already have recorded a byte count this rollback
		// just discarded, so the metadata is put back with the partial.
		var metadataErr error
		if metadata.Bytes != offset {
			metadata.Bytes = offset
			metadataErr = writeMetadata(context.Background(), metadataPath, *metadata)
		}
		return errors.Join(copyErr, rollbackErr, metadataErr)
	}
	retainErr := retainPartial(part, metadataPath, metadata, offset+written)
	if copyErr != nil {
		if ctxErr := contextError(ctx); ctxErr != nil {
			return errors.Join(ctxErr, retainErr)
		}
		return errors.Join(copyErr, retainErr)
	}
	if retainErr != nil {
		return retainErr
	}
	if written != wantLength {
		return retryablef("GET body ended at %d bytes, want %d", written, wantLength)
	}
	return nil
}

// copyResponse streams the response body into the partial file, calling
// checkpoint once every checkpointBytes of progress so a killed process can
// resume from the last durable byte count.
func copyResponse(ctx context.Context, reader io.Reader, writer io.Writer, expected, checkpointBytes int64, checkpoint func(int64) error) (int64, error) {
	var written, sinceCheckpoint int64
	buffer := make([]byte, maxHashReadBytes)
	for {
		count, readErr := reader.Read(buffer)
		if count < 0 || count > len(buffer) {
			return written, fmt.Errorf("invalid response read count %d for buffer length %d", count, len(buffer))
		}
		if count > 0 {
			if int64(count) > expected-written {
				return written, fmt.Errorf("%w %d", errResponseTooLong, expected)
			}
			writtenCount, writeErr := writer.Write(buffer[:count])
			if writtenCount < 0 || writtenCount > count {
				return written, fmt.Errorf("invalid partial write count %d", writtenCount)
			}
			written += int64(writtenCount)
			sinceCheckpoint += int64(writtenCount)
			if writeErr != nil {
				return written, fmt.Errorf("write partial download: %w", writeErr)
			}
			if writtenCount != count {
				return written, io.ErrShortWrite
			}
			if checkpointBytes > 0 && sinceCheckpoint >= checkpointBytes && written < expected {
				if err := checkpoint(written); err != nil {
					return written, err
				}
				sinceCheckpoint = 0
			}
		}
		if err := contextError(ctx); err != nil {
			return written, err
		}
		if readErr == io.EOF {
			if written != expected {
				return written, retryablef("GET body ended at %d bytes, want %d", written, expected)
			}
			return written, nil
		}
		if readErr != nil {
			if ctxErr := contextError(ctx); ctxErr != nil {
				return written, ctxErr
			}
			return written, retryablef("GET body interrupted: %v", readErr)
		}
	}
}

func retainPartial(part *os.File, metadataPath string, metadata *partialMetadata, bytesWritten int64) error {
	if err := part.Sync(); err != nil {
		return fmt.Errorf("sync partial download: %w", err)
	}
	info, err := part.Stat()
	if err != nil {
		return fmt.Errorf("stat partial download: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() != bytesWritten {
		return fmt.Errorf("partial size %d does not match transferred bytes %d", info.Size(), bytesWritten)
	}
	if info.Size() < 0 || info.Size() > metadata.Length {
		return fmt.Errorf("partial size %d exceeds source length %d", info.Size(), metadata.Length)
	}
	metadata.Bytes = info.Size()
	if err := writeMetadata(context.Background(), metadataPath, *metadata); err != nil {
		return err
	}
	return nil
}

func rollbackPartial(part *os.File, offset int64) error {
	if err := part.Truncate(offset); err != nil {
		return fmt.Errorf("rollback partial download: %w", err)
	}
	if err := part.Sync(); err != nil {
		return fmt.Errorf("sync rolled-back partial download: %w", err)
	}
	return nil
}

func parseContentRange(value string) (int64, int64, int64, error) {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "bytes ") {
		return 0, 0, 0, fmt.Errorf("range must start with bytes")
	}
	parts := strings.Split(value[len("bytes "):], "/")
	if len(parts) != 2 {
		return 0, 0, 0, fmt.Errorf("range must contain one slash")
	}
	span := strings.Split(parts[0], "-")
	if len(span) != 2 || parts[1] == "*" {
		return 0, 0, 0, fmt.Errorf("range must contain start, end and total")
	}
	start, err := strconv.ParseInt(strings.TrimSpace(span[0]), 10, 64)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid range start")
	}
	end, err := strconv.ParseInt(strings.TrimSpace(span[1]), 10, 64)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid range end")
	}
	total, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
	if err != nil || start < 0 || end < start || total <= end {
		return 0, 0, 0, fmt.Errorf("invalid range values")
	}
	return start, end, total, nil
}

func responseLength(response *http.Response) (int64, error) {
	text := strings.TrimSpace(response.Header.Get("Content-Length"))
	if text == "" {
		return 0, fmt.Errorf("missing Content-Length")
	}
	length, err := strconv.ParseInt(text, 10, 64)
	if err != nil || length < 0 {
		return 0, fmt.Errorf("invalid length %q", text)
	}
	if response.ContentLength >= 0 && response.ContentLength != length {
		return 0, fmt.Errorf("header length %d differs from response length %d", length, response.ContentLength)
	}
	return length, nil
}

// inspectPartial reads the partial metadata and returns it with the observed
// .part size. A .part longer than the recorded byte count carries a tail that
// was never fsynced and is truncated later, once the source has been confirmed;
// a shorter one has lost fsynced bytes and is refused here.
func inspectPartial(targetPath, sourceURL string) (partialMetadata, int64, bool, error) {
	partPath := targetPath + partSuffix
	metadataPath := targetPath + metadataSuffix
	partInfo, partErr := os.Lstat(partPath)
	metaInfo, metaErr := os.Lstat(metadataPath)
	partExists := partErr == nil
	metaExists := metaErr == nil
	if partErr != nil && !os.IsNotExist(partErr) {
		return partialMetadata{}, 0, false, fmt.Errorf("inspect partial download: %w", partErr)
	}
	if metaErr != nil && !os.IsNotExist(metaErr) {
		return partialMetadata{}, 0, false, fmt.Errorf("inspect partial metadata: %w", metaErr)
	}
	if !partExists && !metaExists {
		return partialMetadata{}, 0, false, nil
	}
	if partExists != metaExists {
		return partialMetadata{}, 0, false, fmt.Errorf("partial download and metadata are incomplete; existing files preserved")
	}
	if !partInfo.Mode().IsRegular() || !metaInfo.Mode().IsRegular() {
		return partialMetadata{}, 0, false, fmt.Errorf("partial download artifacts must be regular files")
	}
	if metaInfo.Size() < 0 || metaInfo.Size() > maxMetadataBytes {
		return partialMetadata{}, 0, false, fmt.Errorf("partial metadata exceeds %d byte limit", maxMetadataBytes)
	}
	data, err := readRegularBounded(context.Background(), metadataPath, maxMetadataBytes)
	if err != nil {
		return partialMetadata{}, 0, false, fmt.Errorf("read partial metadata: %w", err)
	}
	var metadata partialMetadata
	if err := decodeStrictJSON(data, &metadata); err != nil {
		return partialMetadata{}, 0, false, fmt.Errorf("decode partial metadata: %w", err)
	}
	if metadata.SchemaVersion != partialSchemaVersion || metadata.URL != sourceURL || metadata.ETag == "" || metadata.Length < 0 || metadata.Bytes < 0 || metadata.Bytes > metadata.Length {
		return partialMetadata{}, 0, false, fmt.Errorf("partial metadata is invalid or belongs to another source; existing files preserved")
	}
	if partInfo.Size() < metadata.Bytes {
		return partialMetadata{}, 0, false, fmt.Errorf("partial length %d is below metadata bytes %d; existing files preserved", partInfo.Size(), metadata.Bytes)
	}
	if partInfo.Size() > metadata.Length {
		return partialMetadata{}, 0, false, fmt.Errorf("partial length %d exceeds source length %d; existing files preserved", partInfo.Size(), metadata.Length)
	}
	return metadata, partInfo.Size(), true, nil
}

func readRegularBounded(ctx context.Context, path string, limit int64) (data []byte, retErr error) {
	if limit < 0 {
		return nil, fmt.Errorf("negative read limit")
	}
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	lstat, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !lstat.Mode().IsRegular() {
		return nil, fmt.Errorf("path is not a regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			retErr = errors.Join(retErr, fmt.Errorf("close regular file: %w", closeErr))
		}
	}()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("opened path is not a regular file")
	}
	if info.Size() < 0 || info.Size() > limit {
		return nil, fmt.Errorf("file exceeds %d byte limit", limit)
	}
	data = make([]byte, 0, info.Size())
	buffer := make([]byte, maxHashReadBytes)
	for {
		if err := contextError(ctx); err != nil {
			return nil, err
		}
		readSize := len(buffer)
		if remaining := limit - int64(len(data)); remaining < int64(readSize) {
			if remaining > 0 {
				readSize = int(remaining)
			} else {
				readSize = 1
			}
		}
		count, readErr := file.Read(buffer[:readSize])
		if count > 0 {
			data = append(data, buffer[:count]...)
			if int64(len(data)) > limit {
				return nil, fmt.Errorf("file exceeds %d byte limit", limit)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, readErr
		}
		if count == 0 {
			return nil, io.ErrNoProgress
		}
	}
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	finalInfo, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !finalInfo.Mode().IsRegular() || finalInfo.Size() != int64(len(data)) {
		return nil, fmt.Errorf("file changed while reading")
	}
	if finalInfo.Size() != info.Size() {
		return nil, fmt.Errorf("file size changed while reading")
	}
	return data, nil
}

func openPartForWrite(path string, expectedBytes int64) (*os.File, error) {
	if expectedBytes < 0 {
		return nil, fmt.Errorf("negative partial byte count")
	}
	lstat, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !lstat.Mode().IsRegular() {
		return nil, fmt.Errorf("partial download must be a regular file")
	}
	part, err := os.OpenFile(path, os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	info, err := part.Stat()
	if err != nil {
		_ = part.Close()
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() != expectedBytes {
		_ = part.Close()
		return nil, fmt.Errorf("partial file changed or is not regular")
	}
	return part, nil
}

// truncatePart drops the unsynced tail of a partial file and fsyncs the result
// so the recorded byte count and the file agree before the transfer resumes.
func truncatePart(path string, size int64) (retErr error) {
	if size < 0 {
		return fmt.Errorf("negative partial byte count")
	}
	lstat, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect partial download: %w", err)
	}
	if !lstat.Mode().IsRegular() {
		return fmt.Errorf("partial download must be a regular file")
	}
	file, err := os.OpenFile(path, os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("open partial download for truncation: %w", err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			retErr = errors.Join(retErr, fmt.Errorf("close truncated partial download: %w", closeErr))
		}
	}()
	if err := file.Truncate(size); err != nil {
		return fmt.Errorf("truncate partial download to %d bytes: %w", size, err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync truncated partial download: %w", err)
	}
	return nil
}

func createPart(path string) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("create partial download: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return fmt.Errorf("sync partial download: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return fmt.Errorf("close partial download: %w", err)
	}
	return nil
}

func writeMetadata(ctx context.Context, path string, metadata partialMetadata) (retErr error) {
	if err := contextError(ctx); err != nil {
		return err
	}
	data, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("marshal partial metadata: %w", err)
	}
	if len(data) > maxMetadataBytes {
		return fmt.Errorf("partial metadata exceeds %d byte limit", maxMetadataBytes)
	}
	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create partial metadata temporary file: %w", err)
	}
	tempPath := temp.Name()
	removeTemp := true
	closed := false
	defer func() {
		if !closed {
			closed = true
			if closeErr := temp.Close(); closeErr != nil {
				retErr = errors.Join(retErr, fmt.Errorf("close partial metadata temporary file: %w", closeErr))
			}
		}
		if removeTemp {
			if removeErr := os.Remove(tempPath); removeErr != nil && !os.IsNotExist(removeErr) {
				retErr = errors.Join(retErr, fmt.Errorf("remove partial metadata temporary file: %w", removeErr))
			}
		}
	}()
	if err := writeContext(ctx, temp, data); err != nil {
		return err
	}
	if err := temp.Sync(); err != nil {
		return fmt.Errorf("sync partial metadata: %w", err)
	}
	if err := temp.Close(); err != nil {
		closed = true
		return fmt.Errorf("close partial metadata: %w", err)
	}
	closed = true
	if err := contextError(ctx); err != nil {
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("publish partial metadata: %w", err)
	}
	removeTemp = false
	if err := syncDirectory(context.Background(), dir); err != nil {
		return fmt.Errorf("partial metadata published but durability unconfirmed: %w", err)
	}
	return contextError(ctx)
}

func hashFile(ctx context.Context, path string, expectedLength int64) (shaText, crcText string, retErr error) {
	if expectedLength < 0 {
		return "", "", fmt.Errorf("negative expected download length")
	}
	lstat, err := os.Lstat(path)
	if err != nil {
		return "", "", fmt.Errorf("inspect completed download: %w", err)
	}
	if !lstat.Mode().IsRegular() {
		return "", "", fmt.Errorf("completed download must be a regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return "", "", fmt.Errorf("open completed download for hashing: %w", err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			retErr = errors.Join(retErr, fmt.Errorf("close completed download: %w", closeErr))
			shaText = ""
			crcText = ""
		}
	}()
	info, err := file.Stat()
	if err != nil {
		return "", "", fmt.Errorf("stat completed download: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() != expectedLength {
		return "", "", fmt.Errorf("completed download length %d does not equal source length %d", info.Size(), expectedLength)
	}
	sha := sha256.New()
	crc := crc32.New(crc32.MakeTable(crc32.Castagnoli))
	writer := io.MultiWriter(sha, crc)
	buffer := make([]byte, maxHashReadBytes)
	var readBytes int64
	for {
		if err := contextError(ctx); err != nil {
			return "", "", err
		}
		readSize := len(buffer)
		if remaining := expectedLength - readBytes; remaining > 0 && remaining < int64(readSize) {
			readSize = int(remaining)
		} else if remaining == 0 {
			readSize = 1
		}
		count, readErr := file.Read(buffer[:readSize])
		if count > 0 {
			readBytes += int64(count)
			if readBytes > expectedLength {
				return "", "", fmt.Errorf("completed download grew beyond source length %d", expectedLength)
			}
			if _, err := writer.Write(buffer[:count]); err != nil {
				return "", "", fmt.Errorf("hash completed download: %w", err)
			}
		}
		if readErr == io.EOF {
			if readBytes != expectedLength {
				return "", "", fmt.Errorf("completed download length %d does not equal source length %d", readBytes, expectedLength)
			}
			break
		}
		if readErr != nil {
			return "", "", fmt.Errorf("read completed download for hashing: %w", readErr)
		}
		if count == 0 {
			return "", "", io.ErrNoProgress
		}
	}
	var crcBytes [4]byte
	binary.BigEndian.PutUint32(crcBytes[:], crc.Sum32())
	return hex.EncodeToString(sha.Sum(nil)), base64.StdEncoding.EncodeToString(crcBytes[:]), nil
}

func upstreamCRC(header http.Header) (string, []byte, error) {
	var encoded string
	var found bool
	for _, value := range header.Values("X-Goog-Hash") {
		for _, item := range strings.Split(value, ",") {
			item = strings.TrimSpace(item)
			parts := strings.SplitN(item, "=", 2)
			key := strings.TrimSpace(parts[0])
			if len(parts) == 1 && strings.EqualFold(key, "crc32c") {
				return "", nil, fmt.Errorf("crc32c value is missing")
			}
			if len(parts) == 2 && strings.EqualFold(key, "crc32c") {
				if found {
					return "", nil, fmt.Errorf("duplicate crc32c value")
				}
				found = true
				encoded = strings.TrimSpace(parts[1])
			}
		}
	}
	if !found {
		return "", nil, nil
	}
	if encoded == "" {
		return "", nil, fmt.Errorf("empty crc32c value")
	}
	decoded, err := decodeCRC32C(encoded)
	if err != nil {
		return "", nil, err
	}
	return base64.StdEncoding.EncodeToString(decoded), decoded, nil
}

func providerHashes(header http.Header) (map[string]string, error) {
	hashes := make(map[string]string)
	for _, value := range header.Values("X-Goog-Hash") {
		for _, item := range strings.Split(value, ",") {
			parts := strings.SplitN(strings.TrimSpace(item), "=", 2)
			key := strings.ToLower(strings.TrimSpace(parts[0]))
			if key != "crc32c" && key != "md5" {
				continue
			}
			if len(parts) != 2 || strings.TrimSpace(parts[1]) == "" {
				return nil, fmt.Errorf("%s value is missing", key)
			}
			encoded := strings.TrimSpace(parts[1])
			if _, exists := hashes[key]; exists {
				return nil, fmt.Errorf("duplicate %s value", key)
			}
			var (
				decoded []byte
				err     error
			)
			switch key {
			case "crc32c":
				decoded, err = decodeCRC32C(encoded)
			case "md5":
				decoded, err = decodeMD5(encoded)
			}
			if err != nil {
				return nil, fmt.Errorf("invalid %s value: %w", key, err)
			}
			hashes[key] = base64.StdEncoding.EncodeToString(decoded)
		}
	}
	return hashes, nil
}

func decodeMD5(encoded string) ([]byte, error) {
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		decoded, err = base64.RawStdEncoding.DecodeString(encoded)
	}
	if err != nil || len(decoded) != 16 {
		return nil, fmt.Errorf("must be sixteen-byte base64")
	}
	return decoded, nil
}

func mergeProviderHashes(destination *map[string]string, current map[string]string) error {
	if len(current) == 0 {
		return nil
	}
	if *destination == nil {
		*destination = make(map[string]string)
	}
	for key, value := range current {
		if previous, exists := (*destination)[key]; exists && previous != value {
			return fmt.Errorf("provider hash %s changed during download", key)
		}
		(*destination)[key] = value
	}
	return nil
}

func cloneProviderHashes(hashes map[string]string) map[string]string {
	if len(hashes) == 0 {
		return nil
	}
	clone := make(map[string]string, len(hashes))
	for key, value := range hashes {
		clone[key] = value
	}
	return clone
}

func decodeCRC32C(encoded string) ([]byte, error) {
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		decoded, err = base64.RawStdEncoding.DecodeString(encoded)
	}
	if err != nil || len(decoded) != 4 {
		return nil, fmt.Errorf("must be four-byte base64")
	}
	return decoded, nil
}

func checkDiskSpace(dir string, needed int64) error {
	if needed < 0 {
		return fmt.Errorf("required disk space overflows")
	}
	free, err := diskFree(dir)
	if err != nil {
		return fmt.Errorf("check free disk space: %w", err)
	}
	if uint64(needed) > free {
		return fmt.Errorf("insufficient disk space: need %d bytes, have %d", needed, free)
	}
	return nil
}

func validateDirectory(dir string) error {
	info, err := os.Lstat(dir)
	if err != nil {
		return fmt.Errorf("stat target directory: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("target parent is not a directory")
	}
	return nil
}

func rejectExistingTarget(path string) error {
	info, err := os.Lstat(path)
	if err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("target exists and is not a regular file")
		}
		return fmt.Errorf("target already exists and will not be overwritten")
	}
	if !os.IsNotExist(err) {
		return fmt.Errorf("inspect target: %w", err)
	}
	return nil
}

// acquireLock creates the exclusive lock file. A lock left behind by a process
// that no longer exists is reclaimed once and reported; a lock held by a live
// process, one whose owner cannot be determined and one that does not record a
// pid at all are all refused, so a running transfer is never disturbed.
func acquireLock(path string) (*os.File, bool, error) {
	lock, err := createLock(path)
	if err == nil {
		return lock, false, nil
	}
	if !os.IsExist(err) {
		return nil, false, fmt.Errorf("create download lock: %w", err)
	}
	pid, pidErr := readLockPID(path)
	if pidErr != nil {
		return nil, false, fmt.Errorf("download lock exists and may be stale: %w", pidErr)
	}
	alive, known := processExists(pid)
	if !known {
		return nil, false, fmt.Errorf("download lock exists and may be stale: process %d cannot be checked", pid)
	}
	if alive {
		return nil, false, fmt.Errorf("download lock is held by live process %d", pid)
	}
	if removeErr := os.Remove(path); removeErr != nil && !os.IsNotExist(removeErr) {
		return nil, false, fmt.Errorf("reclaim download lock of dead process %d: %w", pid, removeErr)
	}
	lock, err = createLock(path)
	if err != nil {
		return nil, false, fmt.Errorf("create download lock after reclaiming process %d: %w", pid, err)
	}
	return lock, true, nil
}

func createLock(path string) (*os.File, error) {
	lock, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return nil, err
	}
	if _, err := fmt.Fprintf(lock, "pid=%d\n", os.Getpid()); err != nil {
		_ = lock.Close()
		_ = os.Remove(path)
		return nil, fmt.Errorf("write download lock: %w", err)
	}
	return lock, nil
}

// readLockPID accepts only the exact "pid=<digits>" record this package writes.
func readLockPID(path string) (int, error) {
	data, err := readRegularBounded(context.Background(), path, maxLockBytes)
	if err != nil {
		return 0, fmt.Errorf("read download lock: %w", err)
	}
	text := strings.TrimSpace(string(data))
	if !strings.HasPrefix(text, "pid=") {
		return 0, fmt.Errorf("download lock does not record a pid")
	}
	pid, err := strconv.Atoi(strings.TrimPrefix(text, "pid="))
	if err != nil || pid <= 0 {
		return 0, fmt.Errorf("download lock records an unusable pid")
	}
	return pid, nil
}

// processExists reports whether pid is still running and whether that could be
// decided at all. On Unix the zero signal is the kill(pid, 0) liveness probe:
// ESRCH means the process is gone, EPERM means it exists under another user.
func processExists(pid int) (alive, known bool) {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false, false
	}
	err = process.Signal(syscall.Signal(0))
	switch {
	case err == nil:
		return true, true
	case errors.Is(err, os.ErrProcessDone), errors.Is(err, syscall.ESRCH):
		return false, true
	case errors.Is(err, syscall.EPERM):
		return true, true
	default:
		return false, false
	}
}

func retryablef(format string, args ...any) error {
	return fmt.Errorf("%w: %s", errRetryable, fmt.Sprintf(format, args...))
}

func isRetryable(err error) bool { return errors.Is(err, errRetryable) }

func retryableStatus(status int) bool {
	return status == http.StatusRequestTimeout || status == http.StatusTooEarly || status == http.StatusTooManyRequests || status >= 500
}

func wrapError(operation string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func writeContext(ctx context.Context, writer io.Writer, data []byte) error {
	for len(data) > 0 {
		if err := contextError(ctx); err != nil {
			return err
		}
		count := len(data)
		if count > maxHashReadBytes {
			count = maxHashReadBytes
		}
		written, err := writer.Write(data[:count])
		if written < 0 || written > count {
			return fmt.Errorf("invalid write count %d", written)
		}
		if written == 0 && err == nil {
			return io.ErrShortWrite
		}
		data = data[written:]
		if err != nil {
			return fmt.Errorf("write data: %w", err)
		}
	}
	return contextError(ctx)
}

func decodeStrictJSON(data []byte, destination any) error {
	if err := checkUniqueJSON(data); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("trailing JSON value")
		}
		return fmt.Errorf("trailing JSON: %w", err)
	}
	return nil
}

func checkUniqueJSON(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var path []jsonPathPart
	if err := scanJSONValue(decoder, &path); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	if token, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return fmt.Errorf("trailing JSON value %v", token)
		}
		return fmt.Errorf("trailing JSON: %w", err)
	}
	return nil
}

// maxJSONDepth bounds nesting during the duplicate-key walk; json.Decoder.Token
// itself has no nesting limit.
const maxJSONDepth = 64

type jsonPathPart struct {
	key   string
	index int
	array bool
}

// scanJSONValue walks one JSON value. The path is kept as a stack and only
// formatted when an error is reported, so the walk allocates per key, not per
// key times depth.
func scanJSONValue(decoder *json.Decoder, path *[]jsonPathPart) error {
	if len(*path) > maxJSONDepth {
		return fmt.Errorf("JSON nesting exceeds %d levels at %s", maxJSONDepth, formatJSONPath(*path))
	}
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, isDelim := token.(json.Delim)
	if !isDelim {
		switch token.(type) {
		case nil, bool, string, json.Number:
			return nil
		default:
			return fmt.Errorf("unexpected token %T at %s", token, formatJSONPath(*path))
		}
	}
	switch delim {
	case '{':
		seen := make(map[string]string)
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return fmt.Errorf("object key is %T at %s", keyToken, formatJSONPath(*path))
			}
			folded := jsonkey.Fold(key)
			if previous, exists := seen[folded]; exists {
				return fmt.Errorf("duplicate JSON key %q conflicts with %q at %s", key, previous, formatJSONPath(*path))
			}
			seen[folded] = key
			*path = append(*path, jsonPathPart{key: key})
			err = scanJSONValue(decoder, path)
			*path = (*path)[:len(*path)-1]
			if err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil {
			return err
		}
		if end != json.Delim('}') {
			return fmt.Errorf("object ended with %v at %s", end, formatJSONPath(*path))
		}
		return nil
	case '[':
		index := 0
		for decoder.More() {
			*path = append(*path, jsonPathPart{index: index, array: true})
			err = scanJSONValue(decoder, path)
			*path = (*path)[:len(*path)-1]
			if err != nil {
				return err
			}
			index++
		}
		end, err := decoder.Token()
		if err != nil {
			return err
		}
		if end != json.Delim(']') {
			return fmt.Errorf("array ended with %v at %s", end, formatJSONPath(*path))
		}
		return nil
	default:
		return fmt.Errorf("unexpected delimiter %q at %s", delim, formatJSONPath(*path))
	}
}

func formatJSONPath(path []jsonPathPart) string {
	var builder strings.Builder
	builder.WriteByte('$')
	for _, part := range path {
		if part.array {
			builder.WriteByte('[')
			builder.WriteString(strconv.Itoa(part.index))
			builder.WriteByte(']')
			continue
		}
		builder.WriteByte('.')
		builder.WriteString(part.key)
	}
	return builder.String()
}

func syncDirectory(ctx context.Context, path string) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	directory, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open directory for sync: %w", err)
	}
	syncErr := directory.Sync()
	closeErr := directory.Close()
	if syncErr != nil || closeErr != nil {
		return errors.Join(wrapError("sync directory", syncErr), wrapError("close directory", closeErr))
	}
	return contextError(ctx)
}

func contextError(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("nil context")
	}
	return ctx.Err()
}
