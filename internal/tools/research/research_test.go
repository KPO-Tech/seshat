package research

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tool "github.com/KPO-Tech/seshat/internal/tools/registry"
)

// call runs a tool and returns its text, or the error it reported.
func call(t *testing.T, tl tool.Tool, args map[string]any) (string, error) {
	t.Helper()
	res, err := tl.Call(context.Background(), tool.CallInput{Parsed: args}, nil)
	if err != nil {
		t.Fatalf("a tool must report its failures in the result, not as an error: %v", err)
	}
	if res.Error != nil {
		return "", res.Error
	}
	return res.Content, nil
}

func byName(t *testing.T, tools []tool.Tool, name string) tool.Tool {
	t.Helper()
	for _, tl := range tools {
		if tl.Definition().Name == name {
			return tl
		}
	}
	t.Fatalf("no tool named %s", name)
	return nil
}

func serve(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv
}

func TestExplicitKeysAreTheOnlyKeysUsedAndTheEnvironmentIsNotRead(t *testing.T) {
	t.Setenv("YOUTUBE_API_KEY", "from-the-environment")

	if got := NewKeys(map[string]string{KeyRedditClientID: "id"}).Get(KeyYouTubeAPIKey); got != "" {
		t.Fatalf("a shared server's run must never see the process environment, got %q", got)
	}
	if got := NewKeys(nil).Get(KeyYouTubeAPIKey); got != "from-the-environment" {
		t.Fatalf("a single-user host reads the environment, got %q", got)
	}
	if got := NewKeys(map[string]string{KeyYouTubeAPIKey: " key "}).Get(KeyYouTubeAPIKey); got != "key" {
		t.Fatalf("keys are trimmed, got %q", got)
	}
}

func TestAToolWithoutItsKeysIsNotOfferedAndOneWithThemIs(t *testing.T) {
	without := Tools(NewKeys(map[string]string{"unrelated": "x"}))
	with := Tools(NewKeys(map[string]string{
		KeyRedditClientID: "id", KeyRedditClientSecret: "secret", KeyYouTubeAPIKey: "k", KeyGooglePlacesAPIKey: "k", KeyTrustpilotAPIKey: "k",
	}))

	enabled := func(tools []tool.Tool) map[string]bool {
		found := map[string]bool{}
		for _, tl := range tools {
			found[tl.Definition().Name] = tl.IsEnabled()
		}
		return found
	}
	for name, on := range enabled(without) {
		freeOfKeys := strings.HasPrefix(name, "appstore_")
		if on != freeOfKeys {
			t.Errorf("%s without keys: enabled=%v, want %v", name, on, freeOfKeys)
		}
	}
	for name, on := range enabled(with) {
		if !on {
			t.Errorf("%s with its keys must be enabled", name)
		}
	}
	want := []string{"appstore_search", "appstore_reviews", "reddit_search", "reddit_posts", "reddit_post", "youtube_search", "youtube_comments",
		"places_search", "places_reviews", "trustpilot_company", "trustpilot_reviews"}
	if got := enabled(with); len(got) != len(want) {
		t.Errorf("expected %d research tools, got %d: %v", len(want), len(got), got)
	}
	for _, name := range want {
		if _, ok := enabled(with)[name]; !ok {
			t.Errorf("missing tool %s", name)
		}
	}
}

func TestEveryToolIsReadOnlyAndRejectsMissingRequiredInput(t *testing.T) {
	keys := NewKeys(map[string]string{KeyRedditClientID: "id", KeyRedditClientSecret: "s", KeyYouTubeAPIKey: "k", KeyGooglePlacesAPIKey: "k", KeyTrustpilotAPIKey: "k"})
	for _, tl := range Tools(keys) {
		def := tl.Definition()
		if !def.IsReadOnly || def.IsDestructive || def.RequiresPermission || def.Category != "research" {
			t.Errorf("%s must be a read-only research tool: %+v", def.Name, def)
		}
		if _, err := call(t, tl, map[string]any{}); err == nil {
			t.Errorf("%s must refuse a call without its required input", def.Name)
		}
	}
}

func TestAnAPIRefusalIsWordedSoTheModelKnowsWhatToDo(t *testing.T) {
	cases := []struct {
		err  *apiError
		want string
	}{
		{&apiError{source: "YouTube", status: 403, detail: "quota"}, "key or the account has no access"},
		{&apiError{source: "Reddit", status: 429, retryAfter: 42e9}, "about 42 seconds"},
		{&apiError{source: "Reddit", status: 429}, "try again later"},
		{&apiError{source: "Trustpilot", status: 404}, "found nothing"},
		{&apiError{source: "Trustpilot", status: 500, detail: "boom"}, "HTTP 500"},
	}
	for _, c := range cases {
		if got := c.err.Error(); !strings.Contains(got, c.want) {
			t.Errorf("%q must contain %q", got, c.want)
		}
	}
}

func TestShortenKeepsWholeCharacters(t *testing.T) {
	if got := shorten("héllo wörld", 5); got != "héllo…" {
		t.Fatalf("got %q", got)
	}
	if got := oneLine("a \n\n b\t c"); got != "a b c" {
		t.Fatalf("got %q", got)
	}
}
