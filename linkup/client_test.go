package linkup

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

func newTestClient(t *testing.T, handler http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	c := NewClient("test-key",
		WithBaseURL(srv.URL),
		WithRetry(2, 1*time.Millisecond, 5*time.Millisecond),
	)
	return c, srv
}

func TestSearch_Success(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("auth header = %q", got)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Fatalf("content-type = %q", ct)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true,"data":[{"title":"A"}]}`))
	}
	client, srv := newTestClient(t, handler)
	defer srv.Close()

	resp, err := client.Search(context.Background(), SearchRequest{
		Q:          "hello",
		Depth:      DepthStandard,
		OutputType: OutputSearchResults,
	})
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	var got map[string]any
	if err := resp.DecodeInto(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if ok, _ := got["ok"].(bool); !ok {
		t.Fatalf("unexpected body: %v", got)
	}
}

func TestSearch_RetryOn429_WithRetryAfter(t *testing.T) {
	var calls int32
	handler := func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.Header().Set("Retry-After", "0")
			http.Error(w, "too many", http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}
	client, srv := newTestClient(t, handler)
	defer srv.Close()

	start := time.Now()
	_, err := client.Search(context.Background(), SearchRequest{Q: "q", Depth: DepthStandard, OutputType: OutputSearchResults})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if atomic.LoadInt32(&calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", calls)
	}
	if time.Since(start) < 0 {
		t.Fatalf("timing logic broke")
	}
}

func TestSearch_RetryOn5xx(t *testing.T) {
	var calls int32
	handler := func(w http.ResponseWriter, r *http.Request) {
		if n := atomic.AddInt32(&calls, 1); n < 2 {
			http.Error(w, "boom", http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}
	client, srv := newTestClient(t, handler)
	defer srv.Close()

	_, err := client.Search(context.Background(), SearchRequest{Q: "q", Depth: DepthDeep, OutputType: OutputSearchResults})
	if err != nil {
		t.Fatalf("unexpected error after retry: %v", err)
	}
	if atomic.LoadInt32(&calls) != 2 {
		t.Fatalf("expected 2 attempts, got %d", calls)
	}
}

func TestSearch_UnauthorizedAndForbidden(t *testing.T) {
	statuses := []int{http.StatusUnauthorized, http.StatusForbidden}
	for _, code := range statuses {
		t.Run(strconv.Itoa(code), func(t *testing.T) {
			handler := func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "nope", code)
			}
			client, srv := newTestClient(t, handler)
			defer srv.Close()

			_, err := client.Search(context.Background(), SearchRequest{Q: "x", Depth: DepthStandard, OutputType: OutputSearchResults})
			if code == http.StatusUnauthorized && err != ErrUnauthorized {
				t.Fatalf("want ErrUnauthorized, got %v", err)
			}
			if code == http.StatusForbidden && err != ErrForbidden {
				t.Fatalf("want ErrForbidden, got %v", err)
			}
		})
	}
}

func TestSearch_APIErrorPayload(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(422)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"message":"bad param"}`))
	}
	client, srv := newTestClient(t, handler)
	defer srv.Close()

	_, err := client.Search(context.Background(), SearchRequest{Q: "x", Depth: DepthStandard, OutputType: OutputSearchResults})
	if err == nil {
		t.Fatal("expected error")
	}
	if _, ok := err.(*APIError); !ok {
		t.Fatalf("expected APIError, got %T", err)
	}
}

func TestSearchStructured_Helper(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"answer":"42","sources":[{"title":"Doc","url":"https://example.com"}]}`))
	}
	client, srv := newTestClient(t, handler)
	defer srv.Close()

	type Result struct {
		Answer  string `json:"answer"`
		Sources []struct {
			Title string `json:"title"`
			URL   string `json:"url"`
		} `json:"sources"`
	}

	got, err := SearchStructured[Result](context.Background(), client, SearchRequest{Q: "life", Depth: DepthStandard, OutputType: OutputStructured})
	if err != nil {
		t.Fatalf("SearchStructured error: %v", err)
	}
	if got.Answer != "42" || len(got.Sources) != 1 {
		t.Fatalf("unexpected %+v", got)
	}
}

func TestFetch_Success(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/fetch" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		var req FetchRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if req.URL == "" {
			t.Fatalf("missing url in body")
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"markdown":"Hello"}`))
	}
	client, srv := newTestClient(t, handler)
	defer srv.Close()

	resp, err := client.Fetch(context.Background(), FetchRequest{URL: "https://example.com"})
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}
	var got map[string]any
	if err := resp.DecodeInto(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["markdown"] != "Hello" {
		t.Fatalf("unexpected body: %v", got)
	}
}

func TestGetBalance_Success(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/credits/balance" || r.Method != "GET" {
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"balance": 123.45}`))
	}
	client, srv := newTestClient(t, handler)
	defer srv.Close()

	bal, err := client.GetBalance(context.Background())
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if bal.Balance != 123.45 {
		t.Fatalf("got %.2f", bal.Balance)
	}
}

func TestClient_DefaultsAndOptions(t *testing.T) {
	c := NewClient("k", WithUserAgent("ua/1.0"), WithBaseURL("http://x/"), WithRetry(5, 2*time.Millisecond, 4*time.Millisecond))
	if c.baseURL == "" || c.ua != "ua/1.0" {
		t.Fatalf("options not applied")
	}
	if c.maxRetries != 5 {
		t.Fatalf("retries not applied: %d", c.maxRetries)
	}
	// ensure no trailing slash duplication
	if c.baseURL[len(c.baseURL)-1] == '/' {
		t.Fatalf("baseURL has trailing slash: %q", c.baseURL)
	}
}

func TestSearch_MissingKey(t *testing.T) {
	c := NewClient("")
	_, err := c.Search(context.Background(), SearchRequest{})
	if err == nil {
		t.Fatal("expected error for empty key")
	}
}
