package main

import (
	"context"
	"fmt"
	"os"

	linkup "github.com/raezil/linkup-go/linkup"
)

func main() {
	apiKey := os.Getenv("LINKUP_API_KEY")
	client := linkup.NewClient(apiKey)

	// basic search
	resp, err := client.Search(context.Background(), linkup.SearchRequest{
		Q:          "What is Microsoft's revenue and operating income for 2024?",
		Depth:      linkup.DepthStandard,       // or linkup.DepthDeep
		OutputType: linkup.OutputSearchResults, // or OutputSourcedAnswer / OutputStructured
	})
	if err != nil {
		panic(err)
	}

	fmt.Printf("%s\n", resp.RawJSON())
}
