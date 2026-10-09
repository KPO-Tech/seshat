package research

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	tool "github.com/KPO-Tech/seshat/internal/tools/registry"
)

// Google Places reviews, through the Places API (New) with an API key. The API bills each call and returns at most
// the five most relevant reviews of a place, which the tool says, because a study must not mistake five for all.

const placesBase = "https://places.googleapis.com/v1"

var placesID = regexp.MustCompile(`^[A-Za-z0-9_-]{10,200}$`)

type placesClient struct {
	http *httpClient
	keys *Keys
	base string
}

func newPlacesClient(keys *Keys) *placesClient {
	return &placesClient{http: newHTTPClient("Google Places"), keys: keys, base: placesBase}
}

func (c *placesClient) headers(fieldMask string) map[string]string {
	return map[string]string{"X-Goog-Api-Key": c.keys.Get(KeyGooglePlacesAPIKey), "X-Goog-FieldMask": fieldMask}
}

func placesTools(c *placesClient, keys *Keys) []tool.Tool {
	needs := []string{KeyGooglePlacesAPIKey}
	return []tool.Tool{
		newTool(spec{
			name:        "places_search",
			displayName: "Search Google Places",
			description: `Find businesses and places on Google Maps by name, type and area (a restaurant, a shop, a service company). Returns each place's id (needed by places_reviews), address, average rating and number of ratings. Each call is billed by Google.

Parameters:
- query:    what and where, e.g. "plumber Lyon" (required)
- language: two-letter language of the results (optional)
- limit:    number of places, 1-20 (default: 5)`,
			properties: map[string]any{
				"query":    map[string]any{"type": "string"},
				"language": map[string]any{"type": "string"},
				"limit":    map[string]any{"type": "integer"},
			},
			required: []string{"query"},
			needs:    needs,
		}, keys, c.search),
		newTool(spec{
			name:        "places_reviews",
			displayName: "Google Places reviews",
			description: `Read the reviews of a place on Google Maps, with its average rating and number of ratings. Google returns at most the 5 most relevant reviews of a place, so this shows the tone of a business, not all its reviews. Each call is billed by Google.

Parameters:
- place:    the place id from places_search (required)
- language: two-letter language of the reviews (optional)`,
			properties: map[string]any{
				"place":    map[string]any{"type": "string"},
				"language": map[string]any{"type": "string"},
			},
			required: []string{"place"},
			needs:    needs,
		}, keys, c.reviews),
	}
}

func (c *placesClient) postJSON(ctx context.Context, path string, headers map[string]string, body, out any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+path, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return c.http.do(req, out)
}

func languageArg(args map[string]any) (string, error) {
	lang := strings.ToLower(stringArg(args, "language"))
	if lang != "" && (len(lang) < 2 || len(lang) > 8) {
		return "", fmt.Errorf("language must be a code such as en or fr")
	}
	return lang, nil
}

func (c *placesClient) search(ctx context.Context, args map[string]any) (string, error) {
	lang, err := languageArg(args)
	if err != nil {
		return "", err
	}
	body := map[string]any{"textQuery": stringArg(args, "query"), "pageSize": intArg(args, "limit", 5, 1, 20)}
	if lang != "" {
		body["languageCode"] = lang
	}
	var out struct {
		Places []struct {
			ID          string `json:"id"`
			DisplayName struct {
				Text string `json:"text"`
			} `json:"displayName"`
			Address     string  `json:"formattedAddress"`
			Rating      float64 `json:"rating"`
			RatingCount int     `json:"userRatingCount"`
			MapsURI     string  `json:"googleMapsUri"`
			Type        struct {
				Text string `json:"text"`
			} `json:"primaryTypeDisplayName"`
		} `json:"places"`
	}
	mask := "places.id,places.displayName,places.formattedAddress,places.rating,places.userRatingCount,places.googleMapsUri,places.primaryTypeDisplayName"
	if err := c.postJSON(ctx, "/places:searchText", c.headers(mask), body, &out); err != nil {
		return "", err
	}
	if len(out.Places) == 0 {
		return fmt.Sprintf("No place found for %q.", stringArg(args, "query")), nil
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "Google Places — %d places for %q\n\n", len(out.Places), stringArg(args, "query"))
	for i, p := range out.Places {
		fmt.Fprintf(&sb, "%d. %s (id %s)\n   %s\n   %s\n   %s\n\n", i+1, p.DisplayName.Text, p.ID,
			join(p.Type.Text, p.Address), join(stars(p.Rating), fmt.Sprintf("%d ratings", p.RatingCount)), p.MapsURI)
	}
	return strings.TrimRight(sb.String(), "\n"), nil
}

func (c *placesClient) reviews(ctx context.Context, args map[string]any) (string, error) {
	id := strings.TrimPrefix(stringArg(args, "place"), "places/")
	if !placesID.MatchString(id) {
		return "", fmt.Errorf("place must be a place id such as the ones places_search returns")
	}
	lang, err := languageArg(args)
	if err != nil {
		return "", err
	}
	q := url.Values{}
	if lang != "" {
		q.Set("languageCode", lang)
	}
	endpoint := c.base + "/places/" + id
	if len(q) > 0 {
		endpoint += "?" + q.Encode()
	}
	var out struct {
		DisplayName struct {
			Text string `json:"text"`
		} `json:"displayName"`
		Address     string  `json:"formattedAddress"`
		Rating      float64 `json:"rating"`
		RatingCount int     `json:"userRatingCount"`
		MapsURI     string  `json:"googleMapsUri"`
		Reviews     []struct {
			Rating      float64 `json:"rating"`
			PublishTime string  `json:"publishTime"`
			Relative    string  `json:"relativePublishTimeDescription"`
			Text        struct {
				Text string `json:"text"`
			} `json:"text"`
			Author struct {
				Name string `json:"displayName"`
			} `json:"authorAttribution"`
		} `json:"reviews"`
	}
	mask := "id,displayName,formattedAddress,rating,userRatingCount,googleMapsUri,reviews"
	if err := c.http.getJSON(ctx, endpoint, c.headers(mask), &out); err != nil {
		return "", err
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s\n%s\n%s\n%s\n\n", out.DisplayName.Text, out.Address, join(stars(out.Rating), fmt.Sprintf("%d ratings in all", out.RatingCount)), out.MapsURI)
	if len(out.Reviews) == 0 {
		sb.WriteString("Google returned no written review for this place.")
		return sb.String(), nil
	}
	fmt.Fprintf(&sb, "%d written reviews (Google returns at most the 5 most relevant):\n\n", len(out.Reviews))
	for i, r := range out.Reviews {
		fmt.Fprintf(&sb, "%d. %s\n   %s\n\n", i+1, join(stars(r.Rating), "by "+r.Author.Name, firstNonEmpty(date(r.PublishTime), r.Relative)), shorten(oneLine(r.Text.Text), 1500))
	}
	return strings.TrimRight(sb.String(), "\n"), nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
