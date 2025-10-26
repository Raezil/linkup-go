# linkup-go (Unofficial)
Minimal Go SDK **and** CLI for the [Linkup](https://linkup.so) Search API.  
- Idiomatic `net/http` client with retries for 429/5xx (honors `Retry-After`)  
- Raw JSON passthrough + helpers to decode into your own structs  
- Tiny CLI: `search`, `fetch`, and `balance` with pretty-printed JSON

> ⚠️ Not an official library. Names and endpoints may change.

---

## Installation

### SDK
```bash
go get github.com/raezil/linkup-go
```

Import:
```go
import linkup "github.com/raezil/linkup-go/linkup"
```

---

## Quick Start (SDK)

```go
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	linkup "github.com/raezil/linkup-go/linkup"
)

func main() {
	keyFlag := flag.String("key", "", "Linkup API key (overrides LINKUP_API_KEY)")
	flag.Parse()

	apiKey := *keyFlag
	if apiKey == "" {
		apiKey = os.Getenv("LINKUP_API_KEY")
	}
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "missing API key: set -key or LINKUP_API_KEY")
		os.Exit(2)
	}
	client := linkup.NewClient(apiKey,
		linkup.WithRetry(3, 250*time.Millisecond, 4*time.Second), // optional
	)

	// Simple search
	resp, err := client.Search(context.Background(), linkup.SearchRequest{
		Q:          "Go 1.23 release",
		Depth:      linkup.DepthStandard,
		OutputType: linkup.OutputSearchResults,
	})
	if err != nil {
		panic(err)
	}

	// Decode into a map (or your own struct)
	var m map[string]any
	if err := resp.DecodeInto(&m); err != nil {
		panic(err)
	}
	out, _ := json.MarshalIndent(m, "", "  ")
	fmt.Println(string(out))
}
```

## CLI Usage

Set your API key first:

```bash
export LINKUP_API_KEY=sk_live_...
# Windows (PowerShell): $env:LINKUP_API_KEY="sk_live_..."
```

### Commands
```bash
go run . search  [flags]
go run . fetch   [flags]
go run . balance [flags]
```

#### `search` flags
- `-q` query text
- `-depth` `standard|deep` (default: `standard`)
- `-output` `sourcedAnswer|searchResults|structured` (default: `searchResults`)
- `-from`, `-to` (YYYY-MM-DD)
- `-include`, `-exclude` (comma-separated domains)
- `-images` include images (bool)
- `-inline` inline citations (bool)
- `-sources` include sources (bool)
- `-schema` JSON schema string (for `-output=structured`)
- `-timeout` request timeout (default 30s)
- Debug: `-base` override API base URL, `-ua` custom user agent

Examples:
```bash
go run . search -q "EU AI Act timeline" -depth deep -sources -inline
go run . search -q "Rust 1.82 release notes" -from 2024-10-01 -to 2024-12-31 \
  -include rust-lang.org,blog.rust-lang.org -exclude reddit.com
go run . search -q "top 5 cloud providers 2024" -output structured \
  -schema '{"type":"object","properties":{"items":{"type":"array"}}}'
```

#### `fetch` flags
- `-url` URL to fetch (required)
- `-rawhtml` include raw HTML
- `-render` render JavaScript
- `-images` extract images
- `-timeout`, `-base`, `-ua` (as above)

Examples:
```bash
go run . fetch -url "https://example.com"
go run . fetch -url "https://news.ycombinator.com" -render -rawhtml
```

#### `balance`
```bash
go run . balance
```

All CLI responses are pretty-printed JSON. Pipe to `jq` for filtering:
```bash
go run . search -q "Go 1.23 release" | jq '.results[0]'
```

---

## API Overview

### Client & options
```go
client := linkup.NewClient("YOUR_API_KEY",
	linkup.WithUserAgent("my-app/1.0"),
	linkup.WithBaseURL("http://localhost:8080"), // testing/dev
	linkup.WithRetry(3, 250*time.Millisecond, 4*time.Second),
)
```

### Search
```go
resp, err := client.Search(ctx, linkup.SearchRequest{
	Q:                      "prompt injection defenses",
	Depth:                  linkup.DepthStandard,
	OutputType:             linkup.OutputSearchResults,
	IncludeImages:          false,
	FromDate:               "2024-01-01",
	ToDate:                 "2024-12-31",
	ExcludeDomains:         []string{"reddit.com"},
	IncludeDomains:         []string{"nber.org","arxiv.org"},
	IncludeInlineCitations: true,
	StructuredOutputSchema: nil,
	IncludeSources:         true,
})
```

### Fetch
```go
page, err := client.Fetch(ctx, linkup.FetchRequest{
	URL:            "https://example.com",
	IncludeRawHTML: false,
	RenderJS:       true,
	ExtractImages:  true,
})
```

### Balance
```go
bal, err := client.GetBalance(ctx)
fmt.Println(bal.Balance)
```

### Errors
- `ErrUnauthorized` (401) – check your API key
- `ErrForbidden` (403) – key lacks permission
- `*APIError` – when API returns a JSON error body with a `message`

Retries are applied to 429/5xx, honoring `Retry-After` when present.

---

## Development

Run tests:
```bash
go test ./...
```

Lint (suggested):
```bash
golangci-lint run
```

---

## Notes

- The SDK returns **raw JSON** to stay forward-compatible; use `DecodeInto` to strongly type what you need.
- The CLI is intentionally small and meant as a reference and quick tool.
