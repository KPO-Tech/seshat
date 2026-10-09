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

// Trustpilot, through its Business Units API with an API key: a company's score and its public reviews.

const trustpilotBase = "https://api.trustpilot.com/v1"

var (
	trustpilotDomain = regexp.MustCompile(`^[a-z0-9]([a-z0-9.-]*[a-z0-9])?\.[a-z]{2,}$`)
	trustpilotUnitID = regexp.MustCompile(`^[a-f0-9]{24}$`)
)

type trustpilotClient struct {
	http *httpClient
	keys *Keys
	base string
}

func newTrustpilotClient(keys *Keys) *trustpilotClient {
	return &trustpilotClient{http: newHTTPClient("Trustpilot"), keys: keys, base: trustpilotBase}
}

func (c *trustpilotClient) get(ctx context.Context, path string, q url.Values, out any) error {
	endpoint := c.base + path
	if len(q) > 0 {
		endpoint += "?" + q.Encode()
	}
	return c.http.getJSON(ctx, endpoint, map[string]string{"apikey": c.keys.Get(KeyTrustpilotAPIKey)}, out)
}

func trustpilotTools(c *trustpilotClient, keys *Keys) []tool.Tool {
	needs := []string{KeyTrustpilotAPIKey}
	return []tool.Tool{
		newTool(spec{
			name:        "trustpilot_company",
			displayName: "Trustpilot company",
			description: `Look a company up on Trustpilot by its website's domain: its TrustScore, stars and number of reviews, and the id that trustpilot_reviews accepts.

Parameters:
- domain: the company's website domain, e.g. "example.com" (required)`,
			properties: map[string]any{"domain": map[string]any{"type": "string"}},
			required:   []string{"domain"},
			needs:      needs,
		}, keys, c.company),
		newTool(spec{
			name:        "trustpilot_reviews",
			displayName: "Trustpilot reviews",
			description: `Read the public Trustpilot reviews of a company: stars, title, text, author, country and date, and the company's reply when there is one. Filter by stars to study complaints or praise.

Parameters:
- company:  the company's domain (e.g. "example.com") or its business unit id (required)
- stars:    only reviews with this many stars, 1-5 (optional)
- language: two-letter language of the reviews (optional)
- order:    "recent" | "oldest" | "best" | "worst" (default: recent)
- limit:    number of reviews, 1-200 (default: 40)`,
			properties: map[string]any{
				"company":  map[string]any{"type": "string"},
				"stars":    map[string]any{"type": "integer"},
				"language": map[string]any{"type": "string"},
				"order":    map[string]any{"type": "string", "enum": []string{"recent", "oldest", "best", "worst"}},
				"limit":    map[string]any{"type": "integer"},
			},
			required: []string{"company"},
			needs:    needs,
		}, keys, c.reviews),
	}
}

type trustpilotUnit struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	WebsiteURL  string `json:"websiteUrl"`
	TrustScore  float64
	Stars       float64
	Total       int
}

// resolve turns a domain or a business unit id into the unit's id and name.
func (c *trustpilotClient) resolve(ctx context.Context, company string) (trustpilotUnit, error) {
	company = strings.ToLower(strings.TrimSpace(company))
	company = strings.TrimPrefix(strings.TrimPrefix(company, "https://"), "http://")
	company = strings.TrimPrefix(company, "www.")
	company = strings.SplitN(company, "/", 2)[0]
	if trustpilotUnitID.MatchString(company) {
		return trustpilotUnit{ID: company}, nil
	}
	if !trustpilotDomain.MatchString(company) {
		return trustpilotUnit{}, fmt.Errorf("company must be a website domain such as example.com, or a Trustpilot business unit id")
	}
	var found struct {
		ID          string `json:"id"`
		DisplayName string `json:"displayName"`
		WebsiteURL  string `json:"websiteUrl"`
	}
	if err := c.get(ctx, "/business-units/find", url.Values{"name": {company}}, &found); err != nil {
		var apiErr *apiError
		if asAPIError(err, &apiErr) && apiErr.status == 404 {
			return trustpilotUnit{}, fmt.Errorf("Trustpilot has no company with the domain %s", company)
		}
		return trustpilotUnit{}, err
	}
	return trustpilotUnit{ID: found.ID, DisplayName: found.DisplayName, WebsiteURL: found.WebsiteURL}, nil
}

func (c *trustpilotClient) company(ctx context.Context, args map[string]any) (string, error) {
	unit, err := c.resolve(ctx, stringArg(args, "domain"))
	if err != nil {
		return "", err
	}
	var info struct {
		DisplayName     string  `json:"displayName"`
		WebsiteURL      string  `json:"websiteUrl"`
		TrustScore      float64 `json:"trustScore"`
		Stars           float64 `json:"stars"`
		NumberOfReviews struct {
			Total int `json:"total"`
		} `json:"numberOfReviews"`
	}
	if err := c.get(ctx, "/business-units/"+unit.ID+"/profileinfo", nil, &info); err != nil {
		return "", err
	}
	name := firstNonEmpty(info.DisplayName, unit.DisplayName, stringArg(args, "domain"))
	return fmt.Sprintf("%s (business unit %s)\n%s\nTrustScore %.1f · %s · %d reviews\nhttps://www.trustpilot.com/review/%s",
		name, unit.ID, firstNonEmpty(info.WebsiteURL, unit.WebsiteURL), info.TrustScore, stars(info.Stars), info.NumberOfReviews.Total,
		strings.TrimPrefix(strings.TrimPrefix(firstNonEmpty(info.WebsiteURL, unit.WebsiteURL), "https://"), "http://")), nil
}

func (c *trustpilotClient) reviews(ctx context.Context, args map[string]any) (string, error) {
	unit, err := c.resolve(ctx, stringArg(args, "company"))
	if err != nil {
		return "", err
	}
	orderBy := map[string]string{"": "createdat.desc", "recent": "createdat.desc", "oldest": "createdat.asc", "best": "stars.desc", "worst": "stars.asc"}
	order, ok := orderBy[stringArg(args, "order")]
	if !ok {
		return "", fmt.Errorf("order must be one of: recent, oldest, best, worst")
	}
	lang, err := languageArg(args)
	if err != nil {
		return "", err
	}
	limit := intArg(args, "limit", 40, 1, 200)
	perPage := min(100, limit)

	type review struct {
		Title     string `json:"title"`
		Text      string `json:"text"`
		Stars     int    `json:"stars"`
		CreatedAt string `json:"createdAt"`
		Language  string `json:"language"`
		Consumer  struct {
			DisplayName     string `json:"displayName"`
			DisplayLocation string `json:"displayLocation"`
		} `json:"consumer"`
		Reply *struct {
			Message string `json:"message"`
		} `json:"companyReply"`
	}
	var reviews []review
	for page := 1; len(reviews) < limit; page++ {
		q := url.Values{"perPage": {strconv.Itoa(perPage)}, "page": {strconv.Itoa(page)}, "orderBy": {order}}
		if s := intArg(args, "stars", 0, 0, 5); s > 0 {
			q.Set("stars", strconv.Itoa(s))
		}
		if lang != "" {
			q.Set("language", lang)
		}
		var out struct {
			Reviews []review `json:"reviews"`
		}
		if err := c.get(ctx, "/business-units/"+unit.ID+"/reviews", q, &out); err != nil {
			if len(reviews) > 0 {
				break // keep what was read
			}
			return "", err
		}
		reviews = append(reviews, out.Reviews...)
		if len(out.Reviews) < perPage {
			break
		}
	}
	if len(reviews) == 0 {
		return "No Trustpilot review matches.", nil
	}
	if len(reviews) > limit {
		reviews = reviews[:limit]
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "Trustpilot — %d reviews of %s (%s)\n\n", len(reviews), firstNonEmpty(unit.DisplayName, unit.ID), order)
	for i, r := range reviews {
		fmt.Fprintf(&sb, "%d. %s %s\n   %s\n   %s\n", i+1, strings.Repeat("★", r.Stars), r.Title,
			join("by "+r.Consumer.DisplayName, r.Consumer.DisplayLocation, date(r.CreatedAt), r.Language), shorten(oneLine(r.Text), 1500))
		if r.Reply != nil && strings.TrimSpace(r.Reply.Message) != "" {
			fmt.Fprintf(&sb, "   ↳ company reply: %s\n", shorten(oneLine(r.Reply.Message), 500))
		}
		sb.WriteString("\n")
	}
	return strings.TrimRight(sb.String(), "\n"), nil
}
