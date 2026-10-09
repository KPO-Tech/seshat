package research

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func appStoreFixture(t *testing.T) (*appStoreClient, *[]string) {
	t.Helper()
	var requested []string
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		requested = append(requested, r.URL.String())
		switch {
		case r.URL.Path == "/search":
			fmt.Fprint(w, `{"results":[{"trackId":123456,"trackName":"Todo Pro","sellerName":"Acme","primaryGenreName":"Productivity","averageUserRating":4.6,"userRatingCount":1200,"version":"3.1","trackViewUrl":"https://apps.apple.com/us/app/todo-pro/id123456"}]}`)
		case strings.Contains(r.URL.Path, "/rss/customerreviews/"):
			// Like the real feed: only page 1 has reviews, and the two sorts overlap (30 reviews are in both).
			if !strings.Contains(r.URL.Path, "page=1/") {
				fmt.Fprint(w, `{"feed":{}}`)
				return
			}
			first := 0
			if strings.Contains(r.URL.Path, "mosthelpful") {
				first = 20 // ids 20..69: 30 in common with 0..49
			}
			entries := `{"im:name":{"label":"Todo Pro"},"id":{"label":"app"}},` // describes the app, not a review
			for i := first; i < first+50; i++ {
				entries += fmt.Sprintf(`{"id":{"label":"r%d"},"author":{"name":{"label":"user%d"}},"im:rating":{"label":"%d"},"title":{"label":"Title %d"},"content":{"label":"Body\nof review %d"},"im:version":{"label":"3.1"},"updated":{"label":"2026-10-01T10:00:00-07:00"}},`, i, i, 1+i%5, i, i)
			}
			fmt.Fprintf(w, `{"feed":{"entry":[%s]}}`, strings.TrimSuffix(entries, ","))
		default:
			http.NotFound(w, r)
		}
	})
	c := newAppStoreClient()
	c.searchBase, c.reviewBase = srv.URL, srv.URL
	return c, &requested
}

func TestAppStoreSearchListsAppsWithTheirIDs(t *testing.T) {
	c, requested := appStoreFixture(t)
	tl := byName(t, appStoreTools(c, NewKeys(nil)), "appstore_search")

	out, err := call(t, tl, map[string]any{"query": "todo", "country": "FR", "limit": float64(3)})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Todo Pro (id 123456)", "by Acme", "4.6★", "1200 ratings", "apps.apple.com"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if !strings.Contains((*requested)[0], "country=fr") || !strings.Contains((*requested)[0], "limit=3") {
		t.Errorf("unexpected request %s", (*requested)[0])
	}
}

func TestAppStoreReviewsMergesBothSortsWithoutRepeatingAReview(t *testing.T) {
	c, requested := appStoreFixture(t)
	tl := byName(t, appStoreTools(c, NewKeys(nil)), "appstore_reviews")

	out, err := call(t, tl, map[string]any{"app_id": "123456", "limit": float64(100)})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "70 reviews of app 123456") {
		t.Errorf("50 + 50 with 30 in common is 70 distinct reviews:\n%s", out[:200])
	}
	if strings.Count(out, "Title 25\n") != 1 {
		t.Error("a review in both sorts must appear once")
	}
	if strings.Contains(out, "Todo Pro") {
		t.Error("the entry describing the app must not be a review")
	}
	if !strings.Contains(out, "Body of review 0") {
		t.Errorf("a review's text must be on one line:\n%s", out[:400])
	}
	// Both sorts are read; the empty second page of each ends it.
	if len(*requested) != 4 {
		t.Errorf("expected page 1 and the empty page 2 of each sort: %v", *requested)
	}
}

func TestAppStoreReviewsOfOneSortOnlyAsksForThatSort(t *testing.T) {
	c, requested := appStoreFixture(t)
	tl := byName(t, appStoreTools(c, NewKeys(nil)), "appstore_reviews")

	out, err := call(t, tl, map[string]any{"app_id": "123456", "sort": "helpful", "limit": float64(10)})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "10 reviews") || strings.Contains(out, "Title 0\n") || !strings.Contains(out, "Title 20\n") {
		t.Errorf("unexpected reviews:\n%s", out)
	}
	for _, r := range *requested {
		if strings.Contains(r, "mostrecent") {
			t.Errorf("only the helpful sort was asked for: %v", *requested)
		}
	}
}

func TestMojibakeIsRepairedAndGoodTextIsLeftAlone(t *testing.T) {
	cases := map[string]string{
		"butâ€¦":                   "but…",
		"CafÃ© trÃ¨s":              "Café très",
		"Déjà vu, naïve, “quoted”": "Déjà vu, naïve, “quoted”",
		"plain ascii":              "plain ascii",
		"Ã is a letter, â too":     "Ã is a letter, â too",
	}
	for in, want := range cases {
		if got := repairMojibake(in); got != want {
			t.Errorf("repairMojibake(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAppStoreReviewsRefusesWhatIsNotAnAppID(t *testing.T) {
	c, requested := appStoreFixture(t)
	tl := byName(t, appStoreTools(c, NewKeys(nil)), "appstore_reviews")

	for _, args := range []map[string]any{
		{"app_id": "../../etc"},
		{"app_id": "123456", "country": "france"},
		{"app_id": "123456", "sort": "oldest"},
	} {
		if _, err := call(t, tl, args); err == nil {
			t.Errorf("%v must be refused", args)
		}
	}
	if len(*requested) != 0 {
		t.Errorf("nothing must be requested for a refused input: %v", *requested)
	}
}

func TestAppStoreSaysSoWhenThereIsNothing(t *testing.T) {
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"results":[],"feed":{}}`) })
	c := newAppStoreClient()
	c.searchBase, c.reviewBase = srv.URL, srv.URL
	tools := appStoreTools(c, NewKeys(nil))

	if out, _ := call(t, byName(t, tools, "appstore_search"), map[string]any{"query": "zzz"}); !strings.Contains(out, "No app found") {
		t.Errorf("got %q", out)
	}
	if out, _ := call(t, byName(t, tools, "appstore_reviews"), map[string]any{"app_id": "123456"}); !strings.Contains(out, "has no review") {
		t.Errorf("got %q", out)
	}
}
