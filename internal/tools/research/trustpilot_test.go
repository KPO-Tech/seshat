package research

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

const trustpilotUnit1 = "46d6a8b50000640005011111"

func trustpilotFixture(t *testing.T) (*trustpilotClient, *Keys, *[]string) {
	t.Helper()
	var requested []string
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		requested = append(requested, r.URL.String())
		if r.Header.Get("apikey") != "the-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch {
		case r.URL.Path == "/business-units/find":
			if r.URL.Query().Get("name") != "example.com" {
				http.NotFound(w, r)
				return
			}
			fmt.Fprintf(w, `{"id":"%s","displayName":"Example Ltd","websiteUrl":"https://www.example.com"}`, trustpilotUnit1)
		case r.URL.Path == "/business-units/"+trustpilotUnit1+"/profileinfo":
			fmt.Fprint(w, `{"displayName":"Example Ltd","websiteUrl":"https://www.example.com","trustScore":4.2,"stars":4.0,"numberOfReviews":{"total":3150}}`)
		case r.URL.Path == "/business-units/"+trustpilotUnit1+"/reviews":
			page, _ := strconv.Atoi(r.URL.Query().Get("page"))
			perPage, _ := strconv.Atoi(r.URL.Query().Get("perPage"))
			total := 130 // three pages of 50 would be 150: the last holds 30
			start := (page - 1) * perPage
			var items []string
			for i := start; i < start+perPage && i < total; i++ {
				reply := ""
				if i == 0 {
					reply = `,"companyReply":{"message":"We are sorry\nabout this."}`
				}
				items = append(items, fmt.Sprintf(`{"title":"Title %d","text":"Text\nof %d","stars":%d,"createdAt":"2026-09-%02dT10:00:00Z","language":"en","consumer":{"displayName":"User %d","displayLocation":"GB"}%s}`, i, i, 1+i%5, 1+i%28, i, reply))
			}
			fmt.Fprintf(w, `{"reviews":[%s]}`, strings.Join(items, ","))
		default:
			http.NotFound(w, r)
		}
	})
	keys := NewKeys(map[string]string{KeyTrustpilotAPIKey: "the-key"})
	c := newTrustpilotClient(keys)
	c.base = srv.URL
	return c, keys, &requested
}

func TestTrustpilotCompanyFindsTheUnitByDomainAndReportsItsScore(t *testing.T) {
	c, keys, requested := trustpilotFixture(t)

	out, err := call(t, byName(t, trustpilotTools(c, keys), "trustpilot_company"), map[string]any{"domain": "https://www.Example.com/about"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Example Ltd (business unit " + trustpilotUnit1 + ")", "TrustScore 4.2", "4.0★", "3150 reviews", "https://www.trustpilot.com/review/www.example.com"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if !strings.Contains((*requested)[0], "name=example.com") {
		t.Errorf("the domain must be normalized before the lookup: %s", (*requested)[0])
	}
}

func TestTrustpilotReviewsReadsSeveralPagesAndStopsAtTheLimit(t *testing.T) {
	c, keys, requested := trustpilotFixture(t)

	out, err := call(t, byName(t, trustpilotTools(c, keys), "trustpilot_reviews"), map[string]any{"company": "example.com", "limit": float64(120), "stars": float64(1), "language": "en", "order": "worst"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "120 reviews of Example Ltd (stars.asc)") {
		t.Errorf("unexpected header:\n%s", out[:200])
	}
	for _, want := range []string{"1. ★ Title 0", "by User 0 · GB · 2026-09-01 · en", "Text of 0", "↳ company reply: We are sorry about this."} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out[:600])
		}
	}
	// 120 wanted, 100 a page: a second page, and the lookup first.
	if len(*requested) != 3 {
		t.Fatalf("expected the lookup and two pages: %v", *requested)
	}
	for _, want := range []string{"orderBy=stars.asc", "stars=1", "language=en", "perPage=100"} {
		if !strings.Contains((*requested)[1], want) {
			t.Errorf("first page request lacks %s: %s", want, (*requested)[1])
		}
	}
}

func TestTrustpilotAcceptsABusinessUnitIDDirectly(t *testing.T) {
	c, keys, requested := trustpilotFixture(t)

	if _, err := call(t, byName(t, trustpilotTools(c, keys), "trustpilot_reviews"), map[string]any{"company": trustpilotUnit1, "limit": float64(5)}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains((*requested)[0], "find") {
		t.Errorf("an id needs no lookup: %v", *requested)
	}
}

func TestTrustpilotSaysWhenTheCompanyIsUnknownRefusesBadInputAndNamesAWrongKey(t *testing.T) {
	c, keys, requested := trustpilotFixture(t)
	tools := trustpilotTools(c, keys)

	if _, err := call(t, byName(t, tools, "trustpilot_company"), map[string]any{"domain": "unknown.org"}); err == nil || !strings.Contains(err.Error(), "no company with the domain unknown.org") {
		t.Errorf("got %v", err)
	}
	before := len(*requested)
	if _, err := call(t, byName(t, tools, "trustpilot_reviews"), map[string]any{"company": "not a domain"}); err == nil {
		t.Error("a bad company must be refused")
	}
	if _, err := call(t, byName(t, tools, "trustpilot_reviews"), map[string]any{"company": "example.com", "order": "random"}); err == nil {
		t.Error("a bad order must be refused")
	}
	if len(*requested) != before+1 { // only the lookup of the order test, which resolves the company first
		t.Errorf("a bad company must not be requested: %v", (*requested)[before:])
	}

	c.keys = NewKeys(map[string]string{KeyTrustpilotAPIKey: "wrong"})
	if _, err := call(t, byName(t, tools, "trustpilot_company"), map[string]any{"domain": "example.com"}); err == nil || !strings.Contains(err.Error(), "no access") {
		t.Errorf("expected a clear refusal, got %v", err)
	}
}
