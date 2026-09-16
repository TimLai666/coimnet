package teacher

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/TimLai666/coimnet/config"
)

// Budget bounds what a teacher may consume across its whole life.
type Budget struct {
	MaxRequests int     `json:"max_requests"`
	MaxCost     float64 `json:"max_cost"`
}

// HTTP posts a Request as JSON to Endpoint and decodes a Response. Every
// remote call is bounded: Timeout per attempt, MaxRetries only for 5xx and
// transport errors with exponential backoff starting at Backoff, RateLimit
// requests per second (0 = unlimited), Budget counted across the teacher's
// life (zero budget refuses every call), AllowedFields is the only set of
// request field names ever sent, and Secret is a reference sent as the
// Authorization bearer value read from the environment at call time and never
// stored or logged. Calls with the same RequestID return the cached response
// and count nothing.
type HTTP struct {
	Endpoint      string           `json:"endpoint"`
	Timeout       time.Duration    `json:"timeout"`
	MaxRetries    int              `json:"max_retries"`
	Backoff       time.Duration    `json:"backoff"`
	RateLimit     float64          `json:"rate_limit"`
	Budget        Budget           `json:"budget"`
	AllowedFields []string         `json:"allowed_fields"`
	Secret        config.SecretRef `json:"secret"`
	ID            string           `json:"id"`
	Version       string           `json:"version"`

	Client *http.Client        // nil means a client with Timeout; tests inject httptest clients
	Env    func(string) string // nil means os.Getenv

	mu       sync.Mutex
	cache    map[string]Response
	used     Budget // requests and cost consumed so far
	lastSend time.Time
}

// ErrBudgetExhausted reports that the teacher's budget has run out.
var ErrBudgetExhausted = errors.New("teacher: budget exhausted")

// ErrRateLimited reports that a call arrived sooner than the rate limit allows.
// The caller decides what to do; no waiting happens here.
var ErrRateLimited = errors.New("teacher: rate limit reached")

// Validate checks every bounded knob: the endpoint must parse as http(s), the
// timeouts and budgets must be sane and finite, AllowedFields must be non-empty
// and duplicate-free, and the identity fields must be non-blank.
func (h *HTTP) Validate() error {
	if strings.TrimSpace(h.Endpoint) == "" {
		return fmt.Errorf("teacher http: endpoint must be non-blank")
	}
	u, err := url.Parse(h.Endpoint)
	if err != nil {
		return fmt.Errorf("teacher http: endpoint: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("teacher http: endpoint scheme %q must be http or https", u.Scheme)
	}
	if h.Timeout <= 0 {
		return fmt.Errorf("teacher http: timeout must be greater than zero")
	}
	if h.MaxRetries < 0 {
		return fmt.Errorf("teacher http: max_retries must be >= 0")
	}
	if h.Backoff < 0 {
		return fmt.Errorf("teacher http: backoff must be >= 0")
	}
	if math.IsNaN(h.RateLimit) || math.IsInf(h.RateLimit, 0) || h.RateLimit < 0 {
		return fmt.Errorf("teacher http: rate_limit must be a finite value >= 0")
	}
	if h.Budget.MaxRequests < 0 {
		return fmt.Errorf("teacher http: budget max_requests must be >= 0")
	}
	if math.IsNaN(h.Budget.MaxCost) || math.IsInf(h.Budget.MaxCost, 0) || h.Budget.MaxCost < 0 {
		return fmt.Errorf("teacher http: budget max_cost must be a finite value >= 0")
	}
	if len(h.AllowedFields) == 0 {
		return fmt.Errorf("teacher http: allowed_fields must not be empty")
	}
	seen := make(map[string]bool, len(h.AllowedFields))
	for _, name := range h.AllowedFields {
		if seen[name] {
			return fmt.Errorf("teacher http: allowed_fields contains duplicate %q", name)
		}
		seen[name] = true
	}
	if strings.TrimSpace(h.ID) == "" {
		return fmt.Errorf("teacher http: id must be non-blank")
	}
	if strings.TrimSpace(h.Version) == "" {
		return fmt.Errorf("teacher http: version must be non-blank")
	}
	return nil
}

// Ask answers one request following, in order: request and teacher validation,
// the context check, cache lookup, budget check, rate limit check, request
// building, the bounded send with retries, and finally the consumption commit.
func (h *HTTP) Ask(ctx context.Context, r Request) (Response, error) {
	if err := r.Validate(); err != nil {
		return Response{}, fmt.Errorf("teacher http: request: %w", err)
	}
	if err := h.Validate(); err != nil {
		return Response{}, err
	}
	if ctx == nil {
		return Response{}, fmt.Errorf("teacher http: nil context")
	}
	if err := ctx.Err(); err != nil {
		return Response{}, err
	}

	h.mu.Lock()
	if h.cache == nil {
		h.cache = make(map[string]Response)
	}
	if cached, ok := h.cache[r.RequestID]; ok {
		h.mu.Unlock()
		return cached, nil
	}
	if h.used.MaxRequests >= h.Budget.MaxRequests ||
		(h.Budget.MaxCost > 0 && h.used.MaxCost >= h.Budget.MaxCost) {
		h.mu.Unlock()
		return Response{}, ErrBudgetExhausted
	}
	if h.RateLimit > 0 && time.Since(h.lastSend) < rateInterval(h.RateLimit) {
		h.mu.Unlock()
		return Response{}, ErrRateLimited
	}
	h.mu.Unlock()

	body, auth, err := h.buildRequest(r)
	if err != nil {
		return Response{}, err
	}

	resp, err := h.request(ctx, r, body, auth)
	if err != nil {
		return Response{}, err
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	if existing, ok := h.cache[r.RequestID]; ok {
		return existing, nil
	}
	h.cache[r.RequestID] = resp
	h.used.MaxRequests++
	if resp.Usage != nil {
		h.used.MaxCost += resp.Usage.Cost
	}
	h.lastSend = time.Now()
	return resp, nil
}

// Describe identifies this teacher as an HTTP teacher.
func (h *HTTP) Describe() Descriptor {
	return Descriptor{ID: h.ID, Version: h.Version, Kind: DescriptorHTTP}
}

// Used returns a copy of the consumption so far.
func (h *HTTP) Used() Budget {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.used
}

// rateInterval is the minimum delay between sends for a rate in requests/sec.
func rateInterval(perSecond float64) time.Duration {
	return time.Duration(float64(time.Second) / perSecond)
}

// buildRequest keeps only AllowedFields keys from r.Fields, marshals the
// request, and resolves the secret. The returned auth is empty when no secret
// is configured; a configured but unset secret is an error before any request
// is sent.
func (h *HTTP) buildRequest(r Request) ([]byte, string, error) {
	allowed := make(map[string]bool, len(h.AllowedFields))
	for _, name := range h.AllowedFields {
		allowed[name] = true
	}
	fields := make(map[string]string)
	for k, v := range r.Fields {
		if allowed[k] {
			fields[k] = v
		}
	}
	payload := Request{
		RequestID:    r.RequestID,
		InputHash:    r.InputHash,
		Kind:         r.Kind,
		Fields:       fields,
		ModelVersion: r.ModelVersion,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, "", fmt.Errorf("teacher http: marshal request: %w", err)
	}
	var auth string
	if h.Secret.Env != "" {
		value := h.env()(h.Secret.Env)
		if value == "" {
			return nil, "", fmt.Errorf("teacher http: secret %s is not set", h.Secret.Env)
		}
		auth = value
	}
	return body, auth, nil
}

// env returns the resolver for secret references.
func (h *HTTP) env() func(string) string {
	if h.Env != nil {
		return h.Env
	}
	return os.Getenv
}

// request sends the body up to 1+MaxRetries times. Each attempt has its own
// Timeout. 2xx decodes and validates the response; 4xx and unexpected statuses
// fail immediately; 5xx and transport errors wait Backoff*2^attempt and retry,
// finally returning the last error.
func (h *HTTP) request(ctx context.Context, r Request, body []byte, auth string) (Response, error) {
	var lastErr error
	for attempt := 0; attempt <= h.MaxRetries; attempt++ {
		attemptCtx, cancel := context.WithTimeout(ctx, h.Timeout)
		resp, err := h.do(attemptCtx, r, body, auth)
		if err != nil {
			cancel()
			lastErr = err
		} else {
			status := resp.StatusCode
			if status >= 200 && status < 300 {
				decoded, decodeErr := decodeResponse(resp, r)
				cancel()
				if decodeErr != nil {
					return Response{}, decodeErr
				}
				return decoded, nil
			}
			resp.Body.Close()
			cancel()
			switch {
			case status >= 400 && status < 500:
				return Response{}, fmt.Errorf("teacher http: endpoint returned status %s", resp.Status)
			case status >= 500:
				lastErr = fmt.Errorf("teacher http: endpoint returned status %s", resp.Status)
			default:
				return Response{}, fmt.Errorf("teacher http: endpoint returned unexpected status %s", resp.Status)
			}
		}
		if attempt < h.MaxRetries {
			if werr := h.wait(ctx, attempt); werr != nil {
				return Response{}, werr
			}
		}
	}
	return Response{}, lastErr
}

// do builds and performs one POST with the per-attempt context.
func (h *HTTP) do(ctx context.Context, r Request, body []byte, auth string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.Endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if auth != "" {
		req.Header.Set("Authorization", "Bearer "+auth)
	}
	client := h.Client
	if client == nil {
		client = &http.Client{Timeout: h.Timeout}
	}
	return client.Do(req)
}

// decodeResponse strictly decodes a 2xx body and cross-checks the echoed
// RequestID and InputHash against the request.
func decodeResponse(resp *http.Response, r Request) (Response, error) {
	defer resp.Body.Close()
	dec := json.NewDecoder(resp.Body)
	dec.DisallowUnknownFields()
	var out Response
	if err := dec.Decode(&out); err != nil {
		return Response{}, fmt.Errorf("teacher http: decode response: %w", err)
	}
	if err := out.Validate(); err != nil {
		return Response{}, fmt.Errorf("teacher http: response: %w", err)
	}
	if out.RequestID != r.RequestID {
		return Response{}, fmt.Errorf("teacher http: response request_id %q does not match request request_id %q", out.RequestID, r.RequestID)
	}
	if out.InputHash != r.InputHash {
		return Response{}, fmt.Errorf("teacher http: response input_hash %q does not match request input_hash %q", out.InputHash, r.InputHash)
	}
	return out, nil
}

// wait sleeps for the exponential backoff of an attempt, cancelling early when
// the caller's context is done. The shift is capped so a wildly large retry
// budget cannot overflow.
func (h *HTTP) wait(ctx context.Context, attempt int) error {
	const maxShift = 20
	shift := attempt
	if shift > maxShift {
		shift = maxShift
	}
	timer := time.NewTimer(h.Backoff * time.Duration(1<<shift))
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
