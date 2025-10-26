package linkup

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestSearch_InvalidJSONDecode(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{not-json`))
	}
	client, srv := newTestClient(t, handler)
	defer srv.Close()

	resp, err := client.Search(context.Background(), SearchRequest{Q: "x", Depth: DepthStandard, OutputType: OutputSearchResults})
	if err != nil {
		t.Fatalf("Search err: %v", err)
	}
	var v map[string]any
	if err := resp.DecodeInto(&v); err == nil {
		t.Fatal("expected decode error")
	}
}

func TestFetch_ErrorCases(t *testing.T) {
	// 401
	{
		h := func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "unauth", http.StatusUnauthorized)
		}
		client, srv := newTestClient(t, h)
		defer srv.Close()
		_, err := client.Fetch(context.Background(), FetchRequest{URL: "https://x"})
		if !errors.Is(err, ErrUnauthorized) {
			t.Fatalf("want ErrUnauthorized, got %v", err)
		}
	}
	// 403
	{
		h := func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "forbid", http.StatusForbidden)
		}
		client, srv := newTestClient(t, h)
		defer srv.Close()
		_, err := client.Fetch(context.Background(), FetchRequest{URL: "https://x"})
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("want ErrForbidden, got %v", err)
		}
	}
	// 5xx with API error body
	{
		h := func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(500)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"message": "boom"})
		}
		client, srv := newTestClient(t, h)
		defer srv.Close()
		_, err := client.Fetch(context.Background(), FetchRequest{URL: "https://x"})
		if _, ok := err.(*APIError); !ok {
			t.Fatalf("want APIError, got %T", err)
		}
	}
}

func TestGetBalance_ErrorCases(t *testing.T) {
	// 401
	{
		h := func(w http.ResponseWriter, r *http.Request) { http.Error(w, "unauth", http.StatusUnauthorized) }
		client, srv := newTestClient(t, h)
		defer srv.Close()
		_, err := client.GetBalance(context.Background())
		if !errors.Is(err, ErrUnauthorized) {
			t.Fatalf("want ErrUnauthorized, got %v", err)
		}
	}
	// 403
	{
		h := func(w http.ResponseWriter, r *http.Request) { http.Error(w, "forbid", http.StatusForbidden) }
		client, srv := newTestClient(t, h)
		defer srv.Close()
		_, err := client.GetBalance(context.Background())
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("want ErrForbidden, got %v", err)
		}
	}
	// 5xx returns APIError
	{
		h := func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(503)
			_ = json.NewEncoder(w).Encode(map[string]any{"message": "svc down"})
		}
		client, srv := newTestClient(t, h)
		defer srv.Close()
		_, err := client.GetBalance(context.Background())
		if _, ok := err.(*APIError); !ok {
			t.Fatalf("want APIError, got %T", err)
		}
	}
}

func TestSearch_ContextTimeout(t *testing.T) {
	h := func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}
	client, srv := newTestClient(t, h)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err := client.Search(ctx, SearchRequest{Q: "x", Depth: DepthStandard, OutputType: OutputSearchResults})
	if err == nil {
		t.Fatal("expected context deadline exceeded")
	}
}

// helper because we used it in TestBackoffCapsAndJitter
func intMin(a, b int) int {
	if a < b {
		return a
	}
	return b
}
