package linkup

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	defaultBaseURL = "https://api.linkup.so/v1"
	defaultUA      = "linkup-go/0.1 (+github.com/raezil/linkup-go)"
)

// Client is a minimal HTTP client for Linkup Search API.
type Client struct {
	apiKey    string
	baseURL   string
	ua        string
	http      *http.Client
	maxRetries int
	minBackoff time.Duration
	maxBackoff time.Duration
}

// Option configures the Client.
type Option func(*Client)

// WithBaseURL overrides the API base URL (useful for testing).
func WithBaseURL(u string) Option {
	return func(c *Client) { c.baseURL = strings.TrimRight(u, "/") }
}

// WithHTTPClient sets a custom http.Client (e.g., with proxy or custom transport).
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) { c.http = h }
}

// WithUserAgent sets a custom User-Agent header.
func WithUserAgent(ua string) Option {
	return func(c *Client) { c.ua = ua }
}

// WithRetry configures retry policy for 429/5xx.
func WithRetry(maxRetries int, minBackoff, maxBackoff time.Duration) Option {
	return func(c *Client) {
		if maxRetries >= 0 {
			c.maxRetries = maxRetries
		}
		if minBackoff > 0 {
			c.minBackoff = minBackoff
		}
		if maxBackoff >= c.minBackoff {
			c.maxBackoff = maxBackoff
		}
	}
}

// NewClient constructs a Client with sane defaults.
func NewClient(apiKey string, opts ...Option) *Client {
	c := &Client{
		apiKey:     apiKey,
		baseURL:    defaultBaseURL,
		ua:         defaultUA,
		http:       &http.Client{Timeout: 30 * time.Second},
		maxRetries: 3,
		minBackoff: 250 * time.Millisecond,
		maxBackoff: 4 * time.Second,
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

var (
	// ErrUnauthorized indicates a 401 response.
	ErrUnauthorized = errors.New("linkup: unauthorized (check API key)")
	// ErrForbidden indicates a 403 response.
	ErrForbidden = errors.New("linkup: forbidden")
)

// Depth defines Linkup depth parameter.
type Depth string

const (
	DepthStandard Depth = "standard"
	DepthDeep     Depth = "deep"
)

// OutputType defines desired output format.
type OutputType string

const (
	OutputSourcedAnswer  OutputType = "sourcedAnswer"
	OutputSearchResults  OutputType = "searchResults"
	OutputStructured     OutputType = "structured"
)

// SearchRequest models the request body for /search.
type SearchRequest struct {
	Q                      string    `json:"q"`
	Depth                  Depth     `json:"depth"`                 // "standard" | "deep"
	OutputType             OutputType`json:"outputType"`            // "sourcedAnswer" | "searchResults" | "structured"
	IncludeImages          bool      `json:"includeImages,omitempty"`
	FromDate               string    `json:"fromDate,omitempty"`       // YYYY-MM-DD
	ToDate                 string    `json:"toDate,omitempty"`         // YYYY-MM-DD
	ExcludeDomains         []string  `json:"excludeDomains,omitempty"` // e.g. ["wikipedia.com"]
	IncludeDomains         []string  `json:"includeDomains,omitempty"` // e.g. ["microsoft.com"]
	IncludeInlineCitations bool      `json:"includeInlineCitations,omitempty"`
	StructuredOutputSchema *string   `json:"structuredOutputSchema,omitempty"`
	IncludeSources         bool      `json:"includeSources,omitempty"`
}

// APIError models an error payload from the API, if any.
type APIError struct {
	Status  int             `json:"status,omitempty"`
	Message string          `json:"message,omitempty"`
	Details json.RawMessage `json:"details,omitempty"`
}

func (e *APIError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Message != "" {
		return fmt.Sprintf("linkup api error: %s (status=%d)", e.Message, e.Status)
	}
	return fmt.Sprintf("linkup api error (status=%d)", e.Status)
}

// SearchResponse wraps the raw JSON with quick helpers.
type SearchResponse struct {
	// Raw is the exact JSON returned by the API.
	Raw json.RawMessage
}

// RawJSON returns a compact JSON string.
func (r SearchResponse) RawJSON() []byte {
	return r.Raw
}

// DecodeInto unmarshals the response into v.
func (r SearchResponse) DecodeInto(v any) error {
	return json.Unmarshal(r.Raw, v)
}

// Search calls POST /search and returns the raw JSON payload for maximum flexibility.
func (c *Client) Search(ctx context.Context, req SearchRequest) (SearchResponse, error) {
	if c.apiKey == "" {
		return SearchResponse{}, errors.New("linkup: API key is empty")
	}
	body, err := json.Marshal(req)
	if err != nil {
		return SearchResponse{}, err
	}

	url := c.baseURL + "/search"
		retries := c.maxRetries

	for attempt := 0; ; attempt++ {
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			return SearchResponse{}, err
		}
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("User-Agent", c.ua)

		res, err := c.http.Do(httpReq)
		if err != nil {
			// Only retry transient network issues.
			if attempt < retries {
				sleep := backoff(attempt, c.minBackoff, c.maxBackoff)
				time.Sleep(sleep)
								continue
			}
			return SearchResponse{}, err
		}

		defer res.Body.Close()

		// Handle non-2xx
		if res.StatusCode < 200 || res.StatusCode >= 300 {
			// Read body (bounded) to attempt decoding API error.
			b, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20)) // 1 MiB
			apiErr := &APIError{Status: res.StatusCode}
			_ = json.Unmarshal(b, apiErr)

			// Decide retry based on code.
			shouldRetry := res.StatusCode == http.StatusTooManyRequests || (res.StatusCode >= 500 && res.StatusCode <= 599)
			if shouldRetry && attempt < retries {
				// Honor Retry-After if present.
				if ra := res.Header.Get("Retry-After"); ra != "" {
					if secs, err := strconv.Atoi(ra); err == nil && secs > 0 {
						time.Sleep(time.Duration(secs) * time.Second)
					} else {
						time.Sleep(backoff(attempt, c.minBackoff, c.maxBackoff))
					}
				} else {
					time.Sleep(backoff(attempt, c.minBackoff, c.maxBackoff))
				}
								continue
			}

			switch res.StatusCode {
			case http.StatusUnauthorized:
				return SearchResponse{}, ErrUnauthorized
			case http.StatusForbidden:
				return SearchResponse{}, ErrForbidden
			default:
				if apiErr.Message != "" {
					return SearchResponse{}, apiErr
				}
				return SearchResponse{}, fmt.Errorf("linkup: http %d", res.StatusCode)
			}
		}

		// Success
		b, err := io.ReadAll(res.Body)
		if err != nil {
			return SearchResponse{}, err
		}
		return SearchResponse{Raw: append([]byte(nil), b...)}, nil
	}
	// unreachable
	
}

// SearchStructured calls c.Search and decodes into a typed struct.
// Note: Go does not allow methods with type parameters; use this free function instead.
func SearchStructured[T any](ctx context.Context, c *Client, req SearchRequest) (T, error) {
	var zero T
	resp, err := c.Search(ctx, req)
	if err != nil {
		return zero, err
	}
	if err := json.Unmarshal(resp.Raw, &zero); err != nil {
		return zero, err
	}
	return zero, nil
}

func backoff(attempt int, min, max time.Duration) time.Duration {
	// Exponential backoff with jitter.
	d := min * (1 << attempt)
	if d > max {
		d = max
	}
	// jitter +/- 20%
	j := time.Duration(float64(d) * (0.8 + 0.4*randFloat()))
	return j
}

// randFloat returns [0,1). Simple LCG to avoid extra deps and keep deterministic-ish behavior per process.
var lcg = uint64(time.Now().UnixNano())

func randFloat() float64 {
	lcg = lcg*2862933555777941757 + 3037000493
	return float64(lcg%10000) / 10000.0
}


// FetchRequest models POST /fetch.
type FetchRequest struct {
	URL            string `json:"url"`
	IncludeRawHTML bool   `json:"includeRawHtml,omitempty"`
	RenderJS       bool   `json:"renderJs,omitempty"`
	ExtractImages  bool   `json:"extractImages,omitempty"`
}

// Fetch calls POST /fetch and returns raw JSON (usually includes markdown).
func (c *Client) Fetch(ctx context.Context, req FetchRequest) (SearchResponse, error) {
	if c.apiKey == "" {
		return SearchResponse{}, errors.New("linkup: API key is empty")
	}
	if req.URL == "" {
		return SearchResponse{}, errors.New("linkup: fetch url is empty")
	}
	body, err := json.Marshal(req)
	if err != nil {
		return SearchResponse{}, err
	}
	url := c.baseURL + "/fetch"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return SearchResponse{}, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("User-Agent", c.ua)

	res, err := c.http.Do(httpReq)
	if err != nil {
		return SearchResponse{}, err
	}
	defer res.Body.Close()

	if res.StatusCode == http.StatusUnauthorized {
		return SearchResponse{}, ErrUnauthorized
	}
	if res.StatusCode == http.StatusForbidden {
		return SearchResponse{}, ErrForbidden
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
		apiErr := &APIError{Status: res.StatusCode}
		_ = json.Unmarshal(b, apiErr)
		if apiErr.Message != "" {
			return SearchResponse{}, apiErr
		}
		return SearchResponse{}, fmt.Errorf("linkup: http %d", res.StatusCode)
	}
	b, err := io.ReadAll(res.Body)
	if err != nil {
		return SearchResponse{}, err
	}
	return SearchResponse{Raw: append([]byte(nil), b...)}, nil
}

// BalanceResponse models GET /credits/balance response.
type BalanceResponse struct {
	Balance float64 `json:"balance"`
}

// GetBalance calls GET /credits/balance and returns credits balance.
func (c *Client) GetBalance(ctx context.Context) (BalanceResponse, error) {
	if c.apiKey == "" {
		return BalanceResponse{}, errors.New("linkup: API key is empty")
	}
	url := c.baseURL + "/credits/balance"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return BalanceResponse{}, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("User-Agent", c.ua)

	res, err := c.http.Do(httpReq)
	if err != nil {
		return BalanceResponse{}, err
	}
	defer res.Body.Close()

	if res.StatusCode == http.StatusUnauthorized {
		return BalanceResponse{}, ErrUnauthorized
	}
	if res.StatusCode == http.StatusForbidden {
		return BalanceResponse{}, ErrForbidden
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
		apiErr := &APIError{Status: res.StatusCode}
		_ = json.Unmarshal(b, apiErr)
		if apiErr.Message != "" {
			return BalanceResponse{}, apiErr
		}
		return BalanceResponse{}, fmt.Errorf("linkup: http %d", res.StatusCode)
	}
	var out BalanceResponse
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return BalanceResponse{}, err
	}
	return out, nil
}
