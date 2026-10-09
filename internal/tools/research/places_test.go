package research

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

func placesFixture(t *testing.T) (*placesClient, *Keys, *[]string, *[]map[string]any) {
	t.Helper()
	var seen []string
	var bodies []map[string]any
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Method+" "+r.URL.String()+" mask="+r.Header.Get("X-Goog-FieldMask"))
		if r.Header.Get("X-Goog-Api-Key") != "the-key" {
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `{"error":{"status":"PERMISSION_DENIED"}}`)
			return
		}
		switch {
		case r.URL.Path == "/places:searchText":
			raw, _ := io.ReadAll(r.Body)
			var body map[string]any
			_ = json.Unmarshal(raw, &body)
			bodies = append(bodies, body)
			fmt.Fprint(w, `{"places":[{"id":"ChIJplace12345","displayName":{"text":"Plomberie Martin"},"formattedAddress":"1 rue X, Lyon","rating":4.4,"userRatingCount":210,"googleMapsUri":"https://maps.google.com/?cid=1","primaryTypeDisplayName":{"text":"Plumber"}}]}`)
		case strings.HasPrefix(r.URL.Path, "/places/ChIJplace12345"):
			fmt.Fprint(w, `{"displayName":{"text":"Plomberie Martin"},"formattedAddress":"1 rue X, Lyon","rating":4.4,"userRatingCount":210,"googleMapsUri":"https://maps.google.com/?cid=1","reviews":[
 {"rating":5,"publishTime":"2026-09-10T10:00:00Z","relativePublishTimeDescription":"a month ago","text":{"text":"Rapide et propre.\nMerci."},"authorAttribution":{"displayName":"Luc"}},
 {"rating":2,"relativePublishTimeDescription":"3 months ago","text":{"text":"Retard de deux heures."},"authorAttribution":{"displayName":"Eve"}}]}`)
		case strings.HasPrefix(r.URL.Path, "/places/ChIJnoreviews0"):
			fmt.Fprint(w, `{"displayName":{"text":"Empty"},"rating":0,"userRatingCount":0}`)
		default:
			http.NotFound(w, r)
		}
	})
	keys := NewKeys(map[string]string{KeyGooglePlacesAPIKey: "the-key"})
	c := newPlacesClient(keys)
	c.base = srv.URL
	return c, keys, &seen, &bodies
}

func TestPlacesSearchSendsTheQueryWithAFieldMaskAndListsPlaces(t *testing.T) {
	c, keys, seen, bodies := placesFixture(t)

	out, err := call(t, byName(t, placesTools(c, keys), "places_search"), map[string]any{"query": "plumber Lyon", "language": "fr", "limit": float64(3)})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Plomberie Martin (id ChIJplace12345)", "Plumber · 1 rue X, Lyon", "4.4★ · 210 ratings", "maps.google.com"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if !strings.Contains((*seen)[0], "places.rating") {
		t.Errorf("the field mask must name the fields asked for (it is what Google bills by): %s", (*seen)[0])
	}
	body := (*bodies)[0]
	if body["textQuery"] != "plumber Lyon" || body["languageCode"] != "fr" || body["pageSize"] != float64(3) {
		t.Errorf("unexpected body %v", body)
	}
}

func TestPlacesReviewsSaysThereAreAtMostFive(t *testing.T) {
	c, keys, seen, _ := placesFixture(t)

	out, err := call(t, byName(t, placesTools(c, keys), "places_reviews"), map[string]any{"place": "places/ChIJplace12345"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Plomberie Martin", "4.4★ · 210 ratings in all", "at most the 5 most relevant", "5.0★ · by Luc · 2026-09-10", "Rapide et propre. Merci.", "2.0★ · by Eve · 3 months ago", "Retard de deux heures."} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if !strings.Contains((*seen)[0], "reviews") {
		t.Errorf("the field mask must ask for the reviews: %s", (*seen)[0])
	}
}

func TestPlacesReviewsOfAPlaceWithNoneSaysSo(t *testing.T) {
	c, keys, _, _ := placesFixture(t)

	out, err := call(t, byName(t, placesTools(c, keys), "places_reviews"), map[string]any{"place": "ChIJnoreviews0"})
	if err != nil || !strings.Contains(out, "no written review") {
		t.Fatalf("got %q, %v", out, err)
	}
}

func TestPlacesRefusesBadInputsAndSaysWhenTheKeyIsWrong(t *testing.T) {
	c, keys, seen, _ := placesFixture(t)
	tools := placesTools(c, keys)

	if _, err := call(t, byName(t, tools, "places_reviews"), map[string]any{"place": "../x"}); err == nil {
		t.Error("a bad place id must be refused")
	}
	if _, err := call(t, byName(t, tools, "places_search"), map[string]any{"query": "x", "language": "not-a-language-code"}); err == nil {
		t.Error("a bad language must be refused")
	}
	if len(*seen) != 0 {
		t.Errorf("nothing must be requested for a refused input: %v", *seen)
	}

	c.keys = NewKeys(map[string]string{KeyGooglePlacesAPIKey: "wrong"})
	if _, err := call(t, byName(t, tools, "places_search"), map[string]any{"query": "x"}); err == nil || !strings.Contains(err.Error(), "no access") {
		t.Errorf("expected a clear refusal, got %v", err)
	}
}
