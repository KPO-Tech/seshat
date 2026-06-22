package providers

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type stubProvider struct {
	name       string
	configured bool
	output     ProviderOutput
	err        error
}

func (p stubProvider) Name() string { return p.name }

func (p stubProvider) IsConfigured() bool { return p.configured }

func (p stubProvider) Search(input SearchInput) (ProviderOutput, error) {
	return p.output, p.err
}

func TestGetProviderChainAutoPrefersConfiguredAPIsBeforeBestEffortProviders(t *testing.T) {
	chain := GetProviderChain(ProviderModeAuto, []SearchProvider{
		stubProvider{name: "searxng", configured: true},
		stubProvider{name: "exa", configured: true},
		stubProvider{name: "tavily", configured: true},
		stubProvider{name: "jina", configured: true},
	})

	got := make([]string, 0, len(chain))
	for _, provider := range chain {
		got = append(got, provider.Name())
	}

	want := []string{"tavily", "exa", "jina", "searxng"}
	if len(got) != len(want) {
		t.Fatalf("unexpected chain length: got %d want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected auto chain order at %d: got %v want %v", i, got, want)
		}
	}
}

func TestRunSearchAutoFallsBackFromEmptySearXNGResult(t *testing.T) {
	output, err := RunSearch(SearchInput{Query: "seshat ai"}, []SearchProvider{
		stubProvider{
			name:       "searxng",
			configured: true,
			output: ProviderOutput{
				Hits:         nil,
				ProviderName: "searxng",
			},
		},
		stubProvider{
			name:       "tavily",
			configured: true,
			output: ProviderOutput{
				Hits:         []SearchHit{{Title: "Result", URL: "https://example.com"}},
				ProviderName: "tavily",
			},
		},
	}, ProviderModeAuto)
	if err != nil {
		t.Fatalf("RunSearch returned error: %v", err)
	}
	if output.ProviderName != "tavily" {
		t.Fatalf("expected tavily fallback result, got %q", output.ProviderName)
	}
	if len(output.Hits) != 1 {
		t.Fatalf("expected 1 hit after fallback, got %d", len(output.Hits))
	}
}

func TestRunSearchAutoDoesNotFallbackFromEmptyAPISearchResult(t *testing.T) {
	output, err := RunSearch(SearchInput{Query: "seshat ai"}, []SearchProvider{
		stubProvider{
			name:       "tavily",
			configured: true,
			output: ProviderOutput{
				Hits:         nil,
				ProviderName: "tavily",
			},
		},
		stubProvider{
			name:       "searxng",
			configured: true,
			output: ProviderOutput{
				Hits:         []SearchHit{{Title: "Fallback", URL: "https://example.com"}},
				ProviderName: "searxng",
			},
		},
	}, ProviderModeAuto)
	if err != nil {
		t.Fatalf("RunSearch returned error: %v", err)
	}
	if output.ProviderName != "tavily" {
		t.Fatalf("expected first robust provider result to be kept, got %q", output.ProviderName)
	}
	if len(output.Hits) != 0 {
		t.Fatalf("expected 0 hits to be preserved for robust provider, got %d", len(output.Hits))
	}
}

func TestRunSearchAutoFallsThroughOnErrors(t *testing.T) {
	output, err := RunSearch(SearchInput{Query: "seshat ai"}, []SearchProvider{
		stubProvider{name: "exa", configured: true, err: errors.New("boom")},
		stubProvider{
			name:       "jina",
			configured: true,
			output: ProviderOutput{
				Hits:         []SearchHit{{Title: "Recovered", URL: "https://example.com"}},
				ProviderName: "jina",
			},
		},
	}, ProviderModeAuto)
	if err != nil {
		t.Fatalf("RunSearch returned error: %v", err)
	}
	if output.ProviderName != "jina" {
		t.Fatalf("expected jina fallback result, got %q", output.ProviderName)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestTavilyProviderUsesPostJSON(t *testing.T) {
	provider := &TavilyProvider{
		apiKey: "tvly-test",
		httpClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.Method != http.MethodPost {
					t.Fatalf("expected POST, got %s", req.Method)
				}
				if got := req.Header.Get("Authorization"); got != "Bearer tvly-test" {
					t.Fatalf("unexpected authorization header: %q", got)
				}
				if got := req.Header.Get("Content-Type"); got != "application/json" {
					t.Fatalf("unexpected content type: %q", got)
				}

				body, err := io.ReadAll(req.Body)
				if err != nil {
					t.Fatalf("read body: %v", err)
				}
				payload := string(body)
				for _, expected := range []string{
					`"query":"latest finance books"`,
					`"include_domains":["example.com"]`,
					`"exclude_domains":["blocked.com"]`,
				} {
					if !strings.Contains(payload, expected) {
						t.Fatalf("expected request body to contain %s, got %s", expected, payload)
					}
				}

				response := `{"results":[{"title":"A","url":"https://example.com","content":"Result body"}]}`
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(response)),
					Header:     make(http.Header),
				}, nil
			}),
		},
	}

	output, err := provider.Search(SearchInput{
		Query:          "latest finance books",
		AllowedDomains: []string{"example.com"},
		BlockedDomains: []string{"blocked.com"},
	})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if output.ProviderName != "tavily" {
		t.Fatalf("unexpected provider name: %q", output.ProviderName)
	}
	if len(output.Hits) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(output.Hits))
	}
}
