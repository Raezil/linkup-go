package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	linkup "github.com/raezil/linkup-go/linkup"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		return
	}
	switch os.Args[1] {
	case "search":
		cmdSearch(os.Args[2:])
	case "fetch":
		cmdFetch(os.Args[2:])
	case "balance":
		cmdBalance(os.Args[2:])
	case "-h", "--help", "help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Println(`linkup CLI (unofficial)
Usage:
  linkup search [flags]
  linkup fetch  [flags]
  linkup balance [flags]

Env:
  LINKUP_API_KEY   Your Linkup API key
`)
}

func cmdSearch(args []string) {
	fs := flag.NewFlagSet("search", flag.ExitOnError)
	q := fs.String("q", "", "query text")
	depth := fs.String("depth", string(linkup.DepthStandard), "depth: standard|deep")
	out := fs.String("output", string(linkup.OutputSearchResults), "output: sourcedAnswer|searchResults|structured")
	from := fs.String("from", "", "from date YYYY-MM-DD")
	to := fs.String("to", "", "to date YYYY-MM-DD")
	include := fs.String("include", "", "comma-separated include domains")
	exclude := fs.String("exclude", "", "comma-separated exclude domains")
	withImgs := fs.Bool("images", false, "include images")
	inlineCite := fs.Bool("inline", false, "include inline citations")
	withSources := fs.Bool("sources", false, "include sources in response")
	schema := fs.String("schema", "", "structured output schema (JSON string)")

	timeout := fs.Duration("timeout", 30*time.Second, "request timeout")
	baseURL := fs.String("base", "", "override base URL (for testing)")
	ua := fs.String("ua", "", "custom user-agent")

	fs.Parse(args)

	apiKey := os.Getenv("LINKUP_API_KEY")
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "missing LINKUP_API_KEY")
		os.Exit(2)
	}

	clientOpts := []linkup.Option{
		linkup.WithRetry(3, 250*time.Millisecond, 4*time.Second),
	}
	if *baseURL != "" {
		clientOpts = append(clientOpts, linkup.WithBaseURL(*baseURL))
	}
	if *ua != "" {
		clientOpts = append(clientOpts, linkup.WithUserAgent(*ua))
	}

	client := linkup.NewClient(apiKey, clientOpts...)
	// Override timeout through context.
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	var schemaPtr *string
	if *schema != "" {
		schemaPtr = schema
	}

	req := linkup.SearchRequest{
		Q:                      *q,
		Depth:                  linkup.Depth(*depth),
		OutputType:             linkup.OutputType(*out),
		IncludeImages:          *withImgs,
		FromDate:               *from,
		ToDate:                 *to,
		ExcludeDomains:         splitCSV(*exclude),
		IncludeDomains:         splitCSV(*include),
		IncludeInlineCitations: *inlineCite,
		StructuredOutputSchema: schemaPtr,
		IncludeSources:         *withSources,
	}

	resp, err := client.Search(ctx, req)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	var buf bytes.Buffer
	if err := json.Indent(&buf, resp.RawJSON(), "", "  "); err == nil {
		fmt.Println(buf.String())
		return
	}
	fmt.Println(string(resp.RawJSON()))
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func cmdFetch(args []string) {
	fs := flag.NewFlagSet("fetch", flag.ExitOnError)
	urlStr := fs.String("url", "", "URL to fetch")
	raw := fs.Bool("rawhtml", false, "include raw HTML")
	render := fs.Bool("render", false, "render JavaScript")
	images := fs.Bool("images", false, "extract images")
	timeout := fs.Duration("timeout", 30*time.Second, "request timeout")
	baseURL := fs.String("base", "", "override base URL (for testing)")
	ua := fs.String("ua", "", "custom user-agent")
	fs.Parse(args)

	apiKey := os.Getenv("LINKUP_API_KEY")
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "missing LINKUP_API_KEY")
		os.Exit(2)
	}
	clientOpts := []linkup.Option{}
	if *baseURL != "" {
		clientOpts = append(clientOpts, linkup.WithBaseURL(*baseURL))
	}
	if *ua != "" {
		clientOpts = append(clientOpts, linkup.WithUserAgent(*ua))
	}

	client := linkup.NewClient(apiKey, clientOpts...)
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	resp, err := client.Fetch(ctx, linkup.FetchRequest{
		URL:            *urlStr,
		IncludeRawHTML: *raw,
		RenderJS:       *render,
		ExtractImages:  *images,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	var buf bytes.Buffer
	if err := json.Indent(&buf, resp.RawJSON(), "", "  "); err == nil {
		fmt.Println(buf.String())
		return
	}
	fmt.Println(string(resp.RawJSON()))
}

func cmdBalance(args []string) {
	fs := flag.NewFlagSet("balance", flag.ExitOnError)
	baseURL := fs.String("base", "", "override base URL (for testing)")
	ua := fs.String("ua", "", "custom user-agent")
	timeout := fs.Duration("timeout", 15*time.Second, "request timeout")
	fs.Parse(args)

	apiKey := os.Getenv("LINKUP_API_KEY")
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "missing LINKUP_API_KEY")
		os.Exit(2)
	}
	clientOpts := []linkup.Option{}
	if *baseURL != "" {
		clientOpts = append(clientOpts, linkup.WithBaseURL(*baseURL))
	}
	if *ua != "" {
		clientOpts = append(clientOpts, linkup.WithUserAgent(*ua))
	}

	client := linkup.NewClient(apiKey, clientOpts...)
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	bal, err := client.GetBalance(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	out, _ := json.MarshalIndent(bal, "", "  ")
	fmt.Println(string(out))
}
