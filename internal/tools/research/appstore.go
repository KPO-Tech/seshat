package research

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	tool "github.com/KPO-Tech/seshat/internal/tools/registry"
)

// The Apple App Store, through its public search and customer-review feeds. No key is needed.
//
// The review feed is Apple's legacy RSS and is patchy, as checked against the real service: it serves at most
// 50 reviews per country, sort and app, usually only the first page, and for some combinations of app, country
// and sort it serves none while another sort of the same app serves 50. So the tool asks both sorts and merges
// them, and says what it got. Its text is also mis-encoded (UTF-8 read as Windows-1252), which is repaired.
const (
	appStoreSearchBase = "https://itunes.apple.com"
	appStoreReviewBase = "https://itunes.apple.com"
	appStorePageSize   = 50
	appStoreMaxPages   = 10
)

var (
	appStoreCountry = regexp.MustCompile(`^[a-zA-Z]{2}$`)
	appStoreID      = regexp.MustCompile(`^[0-9]{3,20}$`)
)

type appStoreClient struct {
	http       *httpClient
	searchBase string
	reviewBase string
}

func newAppStoreClient() *appStoreClient {
	return &appStoreClient{http: newHTTPClient("The Apple App Store"), searchBase: appStoreSearchBase, reviewBase: appStoreReviewBase}
}

func appStoreTools(c *appStoreClient, keys *Keys) []tool.Tool {
	return []tool.Tool{
		newTool(spec{
			name:        "appstore_search",
			displayName: "Search the App Store",
			description: `Find apps in the Apple App Store by name or topic. Returns each app's id (needed by appstore_reviews), developer, average rating and number of ratings.

Parameters:
- query:   what to look for (required)
- country: two-letter storefront, e.g. "us", "fr" (default: us)
- limit:   number of apps, 1-20 (default: 5)`,
			properties: map[string]any{
				"query":   map[string]any{"type": "string"},
				"country": map[string]any{"type": "string"},
				"limit":   map[string]any{"type": "integer"},
			},
			required: []string{"query"},
		}, keys, c.search),
		newTool(spec{
			name:        "appstore_reviews",
			displayName: "App Store reviews",
			description: `Read customer reviews of an app in the Apple App Store: rating, title, text, version, author and date.

Apple's public feed gives at most 50 reviews per country and per sort (usually the first page only), and sometimes none for one sort while the other has some, so by default both sorts are read and merged (up to about 100 distinct reviews). Reviews are per country storefront: ask for several countries to compare markets and to read more.

Parameters:
- app_id:  the app's numeric id, from appstore_search (required)
- country: two-letter storefront (default: us)
- sort:    "recent" | "helpful" (default: both, merged)
- limit:   number of reviews, 1-100 (default: 50)`,
			properties: map[string]any{
				"app_id":  map[string]any{"type": "string"},
				"country": map[string]any{"type": "string"},
				"sort":    map[string]any{"type": "string", "enum": []string{"recent", "helpful"}},
				"limit":   map[string]any{"type": "integer"},
			},
			required: []string{"app_id"},
		}, keys, c.reviews),
	}
}

func appStoreCountryArg(args map[string]any) (string, error) {
	country := strings.ToLower(stringArg(args, "country"))
	if country == "" {
		return "us", nil
	}
	if !appStoreCountry.MatchString(country) {
		return "", fmt.Errorf("country must be a two-letter storefront such as us or fr")
	}
	return country, nil
}

func (c *appStoreClient) search(ctx context.Context, args map[string]any) (string, error) {
	country, err := appStoreCountryArg(args)
	if err != nil {
		return "", err
	}
	limit := intArg(args, "limit", 5, 1, 20)
	q := url.Values{"term": {stringArg(args, "query")}, "entity": {"software"}, "country": {country}, "limit": {strconv.Itoa(limit)}}
	var out struct {
		Results []struct {
			TrackID           int64   `json:"trackId"`
			TrackName         string  `json:"trackName"`
			SellerName        string  `json:"sellerName"`
			Genre             string  `json:"primaryGenreName"`
			AverageRating     float64 `json:"averageUserRating"`
			RatingCount       int     `json:"userRatingCount"`
			AverageRatingThis float64 `json:"averageUserRatingForCurrentVersion"`
			Version           string  `json:"version"`
			Price             float64 `json:"price"`
			URL               string  `json:"trackViewUrl"`
		} `json:"results"`
	}
	if err := c.http.getJSON(ctx, c.searchBase+"/search?"+q.Encode(), nil, &out); err != nil {
		return "", err
	}
	if len(out.Results) == 0 {
		return fmt.Sprintf("No app found in the %s storefront for %q.", strings.ToUpper(country), stringArg(args, "query")), nil
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "App Store (%s) — %d apps for %q\n\n", strings.ToUpper(country), len(out.Results), stringArg(args, "query"))
	for i, a := range out.Results {
		fmt.Fprintf(&sb, "%d. %s (id %d)\n   %s\n   %s\n   %s\n\n", i+1, a.TrackName, a.TrackID,
			join("by "+a.SellerName, a.Genre, "version "+a.Version),
			join(stars(a.AverageRating), fmt.Sprintf("%d ratings", a.RatingCount)),
			a.URL)
	}
	return strings.TrimRight(sb.String(), "\n"), nil
}

type appStoreLabel struct {
	Label string `json:"label"`
}

type appStoreEntry struct {
	Author  struct{ Name appStoreLabel } `json:"author"`
	Rating  *appStoreLabel               `json:"im:rating"`
	Title   appStoreLabel                `json:"title"`
	Content appStoreLabel                `json:"content"`
	Version appStoreLabel                `json:"im:version"`
	Updated appStoreLabel                `json:"updated"`
	ID      appStoreLabel                `json:"id"`
}

func (c *appStoreClient) reviews(ctx context.Context, args map[string]any) (string, error) {
	id := stringArg(args, "app_id")
	if !appStoreID.MatchString(id) {
		return "", fmt.Errorf("app_id must be the numeric id of the app (appstore_search gives it)")
	}
	country, err := appStoreCountryArg(args)
	if err != nil {
		return "", err
	}
	sorts := []string{"mostrecent", "mosthelpful"}
	switch stringArg(args, "sort") {
	case "recent":
		sorts = []string{"mostrecent"}
	case "helpful":
		sorts = []string{"mosthelpful"}
	case "":
	default:
		return "", fmt.Errorf("sort must be recent or helpful")
	}
	limit := intArg(args, "limit", 50, 1, 100)

	seen := map[string]bool{}
	var reviews []appStoreEntry
	var failure error
	for _, sortBy := range sorts {
		got, err := c.readFeed(ctx, country, id, sortBy)
		if err != nil {
			failure = err
			continue
		}
		for _, e := range got {
			if key := e.ID.Label; key == "" || !seen[key] {
				seen[key] = true
				reviews = append(reviews, e)
			}
		}
	}
	if len(reviews) == 0 {
		if failure != nil {
			return "", failure
		}
		return fmt.Sprintf("Apple's public feed has no review for app %s in the %s storefront (it serves none for some apps and countries: try another country).", id, strings.ToUpper(country)), nil
	}
	if len(reviews) > limit {
		reviews = reviews[:limit]
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "App Store (%s) — %d reviews of app %s (Apple's public feed serves at most 50 per sort)\n\n", strings.ToUpper(country), len(reviews), id)
	for i, r := range reviews {
		rating, _ := strconv.Atoi(r.Rating.Label)
		fmt.Fprintf(&sb, "%d. %s %s\n   %s\n   %s\n\n", i+1, strings.Repeat("★", rating), repairMojibake(r.Title.Label),
			join("by "+repairMojibake(r.Author.Name.Label), "version "+r.Version.Label, r.Updated.Label),
			shorten(oneLine(repairMojibake(r.Content.Label)), 1500))
	}
	return strings.TrimRight(sb.String(), "\n"), nil
}

// readFeed reads the pages of one sort until Apple serves an empty one.
func (c *appStoreClient) readFeed(ctx context.Context, country, id, sortBy string) ([]appStoreEntry, error) {
	var reviews []appStoreEntry
	for page := 1; page <= appStoreMaxPages; page++ {
		var out struct {
			Feed struct {
				Entry []appStoreEntry `json:"entry"`
			} `json:"feed"`
		}
		endpoint := fmt.Sprintf("%s/%s/rss/customerreviews/page=%d/id=%s/sortby=%s/json", c.reviewBase, country, page, id, sortBy)
		if err := c.http.getJSON(ctx, endpoint, nil, &out); err != nil {
			if len(reviews) > 0 {
				break // keep what was read; a later page failing does not lose it
			}
			return nil, err
		}
		added := 0
		for _, e := range out.Feed.Entry {
			if e.Rating == nil { // the first entry of a page can describe the app itself
				continue
			}
			reviews = append(reviews, e)
			added++
		}
		if added == 0 {
			break
		}
	}
	return reviews, nil
}
