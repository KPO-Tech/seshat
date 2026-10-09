package research

import (
	"os"
	"strings"
	"testing"
)

// The App Store needs no key, so its tools can be checked against the real service. Not run by default (it
// uses the network): SESHAT_LIVE_RESEARCH=1 go test ./internal/tools/research -run Live -v
func TestLiveAppStore(t *testing.T) {
	if os.Getenv("SESHAT_LIVE_RESEARCH") == "" {
		t.Skip("set SESHAT_LIVE_RESEARCH=1 to check the App Store tools against the real service")
	}
	tools := appStoreTools(newAppStoreClient(), NewKeys(nil))

	found, err := call(t, byName(t, tools, "appstore_search"), map[string]any{"query": "Duolingo", "limit": float64(2)})
	if err != nil {
		t.Fatal(err)
	}
	t.Log("\n" + found)
	if !strings.Contains(found, "Duolingo") || !strings.Contains(found, "(id ") {
		t.Fatalf("the search found nothing useful:\n%s", found)
	}

	reviews, err := call(t, byName(t, tools, "appstore_reviews"), map[string]any{"app_id": "570060128", "country": "gb", "limit": float64(60)})
	if err != nil {
		t.Fatal(err)
	}
	t.Log("\n" + shorten(reviews, 1500))
	if !strings.Contains(reviews, "reviews of app 570060128") || !strings.Contains(reviews, "★") {
		t.Fatalf("no review was read:\n%s", shorten(reviews, 500))
	}
}
