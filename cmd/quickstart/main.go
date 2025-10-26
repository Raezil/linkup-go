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
