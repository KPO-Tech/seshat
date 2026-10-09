package research

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	tool "github.com/KPO-Tech/seshat/internal/tools/registry"
)

// Reddit, through its official API with an application-only token (client credentials): public posts and comments
// only, read only. The organization registers a Reddit app and gives its client id and secret.

const (
	redditAuthBase = "https://www.reddit.com"
	redditAPIBase  = "https://oauth.reddit.com"
)

var (
	redditSubreddit = regexp.MustCompile(`^[A-Za-z0-9_]{2,21}$`)
	redditPostID    = regexp.MustCompile(`(?:comments/|t3_|^)([a-z0-9]{5,10})(?:/|$)`)
)

type redditClient struct {
	http     *httpClient
	keys     *Keys
	authBase string
	apiBase  string

	mu      sync.Mutex
	token   string
	expires time.Time
}

func newRedditClient(keys *Keys) *redditClient {
	return &redditClient{http: newHTTPClient("Reddit"), keys: keys, authBase: redditAuthBase, apiBase: redditAPIBase}
}

// bearer returns a valid application-only token, asking Reddit for a new one when the last one is about to expire.
func (c *redditClient) bearer(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && time.Now().Before(c.expires.Add(-30*time.Second)) {
		return c.token, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.authBase+"/api/v1/access_token", strings.NewReader("grant_type=client_credentials"))
	if err != nil {
		return "", err
	}
	req.SetBasicAuth(c.keys.Get(KeyRedditClientID), c.keys.Get(KeyRedditClientSecret))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	var out struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		Error       string `json:"error"`
	}
	if err := c.http.do(req, &out); err != nil {
		return "", err
	}
	if out.AccessToken == "" {
		return "", fmt.Errorf("Reddit did not give a token (%s): check the client id and secret", out.Error)
	}
	c.token = out.AccessToken
	c.expires = time.Now().Add(time.Duration(out.ExpiresIn) * time.Second)
	return c.token, nil
}

func (c *redditClient) get(ctx context.Context, path string, q url.Values, out any) error {
	token, err := c.bearer(ctx)
	if err != nil {
		return err
	}
	endpoint := c.apiBase + path
	if len(q) > 0 {
		endpoint += "?" + q.Encode()
	}
	err = c.http.getJSON(ctx, endpoint, map[string]string{"Authorization": "Bearer " + token}, out)
	var apiErr *apiError
	if asAPIError(err, &apiErr) && apiErr.status == http.StatusUnauthorized {
		c.mu.Lock()
		c.token = "" // the token was revoked or expired early: the next call asks for a new one
		c.mu.Unlock()
	}
	return err
}

func redditTools(c *redditClient, keys *Keys) []tool.Tool {
	needs := []string{KeyRedditClientID, KeyRedditClientSecret}
	return []tool.Tool{
		newTool(spec{
			name:        "reddit_search",
			displayName: "Search Reddit",
			description: `Search public posts on Reddit, across Reddit or inside one subreddit. Returns title, subreddit, score, number of comments, date, link and the start of the text.

Parameters:
- query:     search terms (required)
- subreddit: limit the search to this subreddit, without r/ (optional)
- sort:      "relevance" | "hot" | "top" | "new" | "comments" (default: relevance)
- time:      "all" | "year" | "month" | "week" | "day" | "hour" (default: all)
- limit:     number of posts, 1-100 (default: 15)`,
			properties: map[string]any{
				"query":     map[string]any{"type": "string"},
				"subreddit": map[string]any{"type": "string"},
				"sort":      map[string]any{"type": "string", "enum": []string{"relevance", "hot", "top", "new", "comments"}},
				"time":      map[string]any{"type": "string", "enum": []string{"all", "year", "month", "week", "day", "hour"}},
				"limit":     map[string]any{"type": "integer"},
			},
			required: []string{"query"},
			needs:    needs,
		}, keys, c.search),
		newTool(spec{
			name:        "reddit_posts",
			displayName: "Reddit subreddit posts",
			description: `List posts of a subreddit.

Parameters:
- subreddit: subreddit name without r/ (required)
- sort:      "hot" | "new" | "top" | "rising" (default: hot)
- time:      "all" | "year" | "month" | "week" | "day" | "hour" (for top only, default: day)
- limit:     number of posts, 1-100 (default: 15)`,
			properties: map[string]any{
				"subreddit": map[string]any{"type": "string"},
				"sort":      map[string]any{"type": "string", "enum": []string{"hot", "new", "top", "rising"}},
				"time":      map[string]any{"type": "string", "enum": []string{"all", "year", "month", "week", "day", "hour"}},
				"limit":     map[string]any{"type": "integer"},
			},
			required: []string{"subreddit"},
			needs:    needs,
		}, keys, c.posts),
		newTool(spec{
			name:        "reddit_post",
			displayName: "Reddit post and comments",
			description: `Read one Reddit post with its comments (the discussion under it), indented by reply depth.

Parameters:
- post:     the post id (e.g. "1abc2de") or its link (required)
- sort:     "top" | "best" | "new" | "controversial" (default: top)
- limit:    number of comments, 1-200 (default: 50)
- depth:    how deep replies go, 1-8 (default: 3)`,
			properties: map[string]any{
				"post":  map[string]any{"type": "string"},
				"sort":  map[string]any{"type": "string", "enum": []string{"top", "best", "new", "controversial"}},
				"limit": map[string]any{"type": "integer"},
				"depth": map[string]any{"type": "integer"},
			},
			required: []string{"post"},
			needs:    needs,
		}, keys, c.post),
	}
}

type redditPost struct {
	ID          string  `json:"id"`
	Title       string  `json:"title"`
	Subreddit   string  `json:"subreddit"`
	Author      string  `json:"author"`
	Score       int     `json:"score"`
	NumComments int     `json:"num_comments"`
	Created     float64 `json:"created_utc"`
	Permalink   string  `json:"permalink"`
	Selftext    string  `json:"selftext"`
	URL         string  `json:"url"`
	Over18      bool    `json:"over_18"`
}

type redditListing struct {
	Data struct {
		Children []struct {
			Kind string          `json:"kind"`
			Data json.RawMessage `json:"data"`
		} `json:"children"`
	} `json:"data"`
}

func (l redditListing) posts() []redditPost {
	var posts []redditPost
	for _, child := range l.Data.Children {
		var p redditPost
		if child.Kind == "t3" && json.Unmarshal(child.Data, &p) == nil {
			posts = append(posts, p)
		}
	}
	return posts
}

func redditTime(unix float64) string {
	return time.Unix(int64(unix), 0).UTC().Format("2006-01-02")
}

func formatRedditPosts(title string, posts []redditPost) string {
	if len(posts) == 0 {
		return title + "\n\nNo posts found."
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s — %d posts\n\n", title, len(posts))
	for i, p := range posts {
		fmt.Fprintf(&sb, "%d. %s\n   %s\n   https://www.reddit.com%s\n", i+1, p.Title,
			join("r/"+p.Subreddit, "u/"+p.Author, fmt.Sprintf("%d points", p.Score), fmt.Sprintf("%d comments", p.NumComments), redditTime(p.Created)), p.Permalink)
		if text := oneLine(p.Selftext); text != "" {
			fmt.Fprintf(&sb, "   %s\n", shorten(text, 400))
		}
		sb.WriteString("\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}

func redditTimeArg(args map[string]any, def string) (string, error) {
	t := stringArg(args, "time")
	if t == "" {
		return def, nil
	}
	if !oneOf(t, "all", "year", "month", "week", "day", "hour") {
		return "", fmt.Errorf("time must be one of: all, year, month, week, day, hour")
	}
	return t, nil
}

func (c *redditClient) search(ctx context.Context, args map[string]any) (string, error) {
	sortBy := stringArg(args, "sort")
	if sortBy == "" {
		sortBy = "relevance"
	}
	if !oneOf(sortBy, "relevance", "hot", "top", "new", "comments") {
		return "", fmt.Errorf("sort must be one of: relevance, hot, top, new, comments")
	}
	period, err := redditTimeArg(args, "all")
	if err != nil {
		return "", err
	}
	q := url.Values{"q": {stringArg(args, "query")}, "sort": {sortBy}, "t": {period}, "type": {"link"},
		"limit": {strconv.Itoa(intArg(args, "limit", 15, 1, 100))}, "raw_json": {"1"}}
	path := "/search"
	if sub := strings.TrimPrefix(stringArg(args, "subreddit"), "r/"); sub != "" {
		if !redditSubreddit.MatchString(sub) {
			return "", fmt.Errorf("subreddit must be a name such as golang, without r/")
		}
		path = "/r/" + sub + "/search"
		q.Set("restrict_sr", "1")
	}
	var out redditListing
	if err := c.get(ctx, path, q, &out); err != nil {
		return "", err
	}
	return formatRedditPosts(fmt.Sprintf("Reddit search %q", stringArg(args, "query")), out.posts()), nil
}

func (c *redditClient) posts(ctx context.Context, args map[string]any) (string, error) {
	sub := strings.TrimPrefix(stringArg(args, "subreddit"), "r/")
	if !redditSubreddit.MatchString(sub) {
		return "", fmt.Errorf("subreddit must be a name such as golang, without r/")
	}
	sortBy := stringArg(args, "sort")
	if sortBy == "" {
		sortBy = "hot"
	}
	if !oneOf(sortBy, "hot", "new", "top", "rising") {
		return "", fmt.Errorf("sort must be one of: hot, new, top, rising")
	}
	q := url.Values{"limit": {strconv.Itoa(intArg(args, "limit", 15, 1, 100))}, "raw_json": {"1"}}
	if sortBy == "top" {
		period, err := redditTimeArg(args, "day")
		if err != nil {
			return "", err
		}
		q.Set("t", period)
	}
	var out redditListing
	if err := c.get(ctx, "/r/"+sub+"/"+sortBy, q, &out); err != nil {
		return "", err
	}
	return formatRedditPosts(fmt.Sprintf("r/%s — %s", sub, sortBy), out.posts()), nil
}

type redditComment struct {
	Author  string          `json:"author"`
	Body    string          `json:"body"`
	Score   int             `json:"score"`
	Created float64         `json:"created_utc"`
	Replies json.RawMessage `json:"replies"` // a listing, or "" when there are none
}

func (c *redditClient) post(ctx context.Context, args map[string]any) (string, error) {
	match := redditPostID.FindStringSubmatch(stringArg(args, "post"))
	if match == nil {
		return "", fmt.Errorf("post must be a Reddit post id such as 1abc2de, or the link of the post")
	}
	id := match[1]
	sortBy := stringArg(args, "sort")
	if sortBy == "" {
		sortBy = "top"
	}
	if !oneOf(sortBy, "top", "best", "new", "controversial") {
		return "", fmt.Errorf("sort must be one of: top, best, new, controversial")
	}
	limit := intArg(args, "limit", 50, 1, 200)
	depth := intArg(args, "depth", 3, 1, 8)
	q := url.Values{"sort": {sortBy}, "limit": {strconv.Itoa(limit)}, "depth": {strconv.Itoa(depth)}, "raw_json": {"1"}}
	var out []redditListing
	if err := c.get(ctx, "/comments/"+id, q, &out); err != nil {
		return "", err
	}
	if len(out) < 2 {
		return "", fmt.Errorf("Reddit answered without the post or its comments")
	}
	posts := out[0].posts()
	if len(posts) == 0 {
		return "", fmt.Errorf("post %s was not found", id)
	}
	p := posts[0]
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s\n%s\nhttps://www.reddit.com%s\n", p.Title,
		join("r/"+p.Subreddit, "u/"+p.Author, fmt.Sprintf("%d points", p.Score), fmt.Sprintf("%d comments", p.NumComments), redditTime(p.Created)), p.Permalink)
	if text := strings.TrimSpace(p.Selftext); text != "" {
		fmt.Fprintf(&sb, "\n%s\n", shorten(text, 6000))
	}
	sb.WriteString("\nComments\n")
	written := 0
	writeRedditComments(&sb, out[1], 0, limit, &written)
	if written == 0 {
		sb.WriteString("(no comments)\n")
	}
	return strings.TrimRight(sb.String(), "\n"), nil
}

func writeRedditComments(sb *strings.Builder, listing redditListing, depth, limit int, written *int) {
	for _, child := range listing.Data.Children {
		if *written >= limit {
			return
		}
		var c redditComment
		if child.Kind != "t1" || json.Unmarshal(child.Data, &c) != nil { // "more" placeholders and anything else
			continue
		}
		body := strings.TrimSpace(c.Body)
		gone := body == "" || body == "[deleted]" || body == "[removed]"
		next := depth
		if !gone {
			*written++
			next = depth + 1
			fmt.Fprintf(sb, "%s- %s: %s\n", strings.Repeat("  ", depth), join("u/"+c.Author, fmt.Sprintf("%d points", c.Score), redditTime(c.Created)), shorten(oneLine(body), 1200))
		}
		// The replies of a removed comment are still worth reading, one level up.
		var replies redditListing
		if len(c.Replies) > 0 && c.Replies[0] == '{' && json.Unmarshal(c.Replies, &replies) == nil {
			writeRedditComments(sb, replies, next, limit, written)
		}
	}
}
