package searxng

// WebResult mirrors SearXNGWebResult from the MCP types.ts.
type WebResult struct {
	Title         string   `json:"title"`
	URL           string   `json:"url"`
	Content       string   `json:"content"`
	Score         float64  `json:"score,omitempty"`
	Engine        string   `json:"engine,omitempty"`
	Engines       []string `json:"engines,omitempty"`
	Category      string   `json:"category,omitempty"`
	PublishedDate string   `json:"publishedDate,omitempty"`
	Thumbnail     string   `json:"thumbnail,omitempty"`
}

// InfoboxURL is a link entry inside an infobox.
type InfoboxURL struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

// Infobox mirrors SearXNGWebInfobox.
type Infobox struct {
	Infobox string       `json:"infobox"`
	Content string       `json:"content,omitempty"`
	URLs    []InfoboxURL `json:"urls,omitempty"`
}

// SearchResponse is the full JSON payload from SearXNG /search?format=json.
type SearchResponse struct {
	Query           string      `json:"query"`
	NumberOfResults int         `json:"number_of_results"`
	Results         []WebResult `json:"results"`
	Suggestions     []string    `json:"suggestions,omitempty"`
	Corrections     []string    `json:"corrections,omitempty"`
	Answers         []string    `json:"answers,omitempty"`
	Infoboxes       []Infobox   `json:"infoboxes,omitempty"`

	// SourceFormat is set internally: "json" when parsed from JSON,
	// "html" when obtained via the HTML fallback path.
	SourceFormat string `json:"-"`
}

// SearchInput holds all parameters for a search request.
// Zero values are omitted from the query string.
type SearchInput struct {
	// Query is the search string. Required.
	Query string
	// PageNo is the page number (default 1).
	PageNo int
	// TimeRange restricts results to a time window: "day", "week", "month", "year".
	TimeRange string
	// Language is the BCP-47 language code, e.g. "en", "fr". "all" means no restriction.
	Language string
	// Safesearch filter: 0 = none, 1 = moderate, 2 = strict.
	Safesearch int
	// MinScore filters results whose score is below this threshold (0.0–1.0).
	// Zero means no filter.
	MinScore float64
	// NumResults caps the number of returned results (1–20). Zero means server default.
	NumResults int
	// Categories is a comma-separated list of SearXNG categories, e.g. "general,news".
	Categories string
	// Engines is a comma-separated list of engine names, e.g. "google,bing,ddg".
	Engines string
}
