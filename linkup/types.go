package linkup

// You can add stronger-typed models here if you know the exact response shape
// for each outputType. This SDK returns raw JSON by default to stay forward-compatible.
//
// Example placeholder types:
type (
	// SourcedAnswer is an example of a possible high-level shape you might expect.
	SourcedAnswer struct {
		Answer  string        `json:"answer,omitempty"`
		Sources []AnswerSource `json:"sources,omitempty"`
	}

	AnswerSource struct {
		Title string `json:"title,omitempty"`
		URL   string `json:"url,omitempty"`
		Snippet string `json:"snippet,omitempty"`
	}
)
