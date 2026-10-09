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

// YouTube, through the Data API v3 with an API key. Searching costs 100 units of the daily quota and reading
// comments costs 1, so the tools say so and keep calls few.

const youTubeBase = "https://www.googleapis.com/youtube/v3"

var (
	youTubeVideoID  = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)
	youTubeFromLink = regexp.MustCompile(`(?:[?&]v=|youtu\.be/|/shorts/|/embed/)([A-Za-z0-9_-]{11})`)
)

type youTubeClient struct {
	http *httpClient
	keys *Keys
	base string
}

func newYouTubeClient(keys *Keys) *youTubeClient {
	return &youTubeClient{http: newHTTPClient("YouTube"), keys: keys, base: youTubeBase}
}

func (c *youTubeClient) get(ctx context.Context, path string, q url.Values, out any) error {
	q.Set("key", c.keys.Get(KeyYouTubeAPIKey))
	return c.http.getJSON(ctx, c.base+path+"?"+q.Encode(), nil, out)
}

func youTubeTools(c *youTubeClient, keys *Keys) []tool.Tool {
	needs := []string{KeyYouTubeAPIKey}
	return []tool.Tool{
		newTool(spec{
			name:        "youtube_search",
			displayName: "Search YouTube",
			description: `Search YouTube videos (reviews, comparisons, reactions). Returns title, channel, date, views, likes, number of comments and the link. Costs 100 units of the daily YouTube quota per call, so prefer one precise query to several vague ones.

Parameters:
- query:  search terms (required)
- order:  "relevance" | "date" | "viewCount" | "rating" (default: relevance)
- region: two-letter region code to favour (optional)
- limit:  number of videos, 1-25 (default: 10)`,
			properties: map[string]any{
				"query":  map[string]any{"type": "string"},
				"order":  map[string]any{"type": "string", "enum": []string{"relevance", "date", "viewCount", "rating"}},
				"region": map[string]any{"type": "string"},
				"limit":  map[string]any{"type": "integer"},
			},
			required: []string{"query"},
			needs:    needs,
		}, keys, c.search),
		newTool(spec{
			name:        "youtube_comments",
			displayName: "YouTube comments",
			description: `Read the comments under a YouTube video: author, text, likes, date, and the first replies. Often the best source of unfiltered customer opinion about a product reviewed in the video.

Parameters:
- video: the video id (11 characters) or its link (required)
- order: "relevance" | "time" (default: relevance)
- limit: number of top-level comments, 1-300 (default: 50)`,
			properties: map[string]any{
				"video": map[string]any{"type": "string"},
				"order": map[string]any{"type": "string", "enum": []string{"relevance", "time"}},
				"limit": map[string]any{"type": "integer"},
			},
			required: []string{"video"},
			needs:    needs,
		}, keys, c.comments),
	}
}

func (c *youTubeClient) search(ctx context.Context, args map[string]any) (string, error) {
	order := stringArg(args, "order")
	if order == "" {
		order = "relevance"
	}
	if !oneOf(order, "relevance", "date", "viewCount", "rating") {
		return "", fmt.Errorf("order must be one of: relevance, date, viewCount, rating")
	}
	q := url.Values{"part": {"snippet"}, "type": {"video"}, "q": {stringArg(args, "query")}, "order": {order},
		"maxResults": {strconv.Itoa(intArg(args, "limit", 10, 1, 25))}}
	if region := strings.ToUpper(stringArg(args, "region")); region != "" {
		if len(region) != 2 {
			return "", fmt.Errorf("region must be a two-letter code such as US or FR")
		}
		q.Set("regionCode", region)
	}
	var found struct {
		Items []struct {
			ID struct {
				VideoID string `json:"videoId"`
			} `json:"id"`
			Snippet struct {
				Title        string `json:"title"`
				ChannelTitle string `json:"channelTitle"`
				PublishedAt  string `json:"publishedAt"`
				Description  string `json:"description"`
			} `json:"snippet"`
		} `json:"items"`
	}
	if err := c.get(ctx, "/search", q, &found); err != nil {
		return "", err
	}
	if len(found.Items) == 0 {
		return fmt.Sprintf("No video found for %q.", stringArg(args, "query")), nil
	}
	ids := make([]string, 0, len(found.Items))
	for _, it := range found.Items {
		ids = append(ids, it.ID.VideoID)
	}
	stats := map[string]struct{ Views, Likes, Comments string }{}
	var details struct {
		Items []struct {
			ID         string `json:"id"`
			Statistics struct {
				ViewCount    string `json:"viewCount"`
				LikeCount    string `json:"likeCount"`
				CommentCount string `json:"commentCount"`
			} `json:"statistics"`
		} `json:"items"`
	}
	// Statistics are a nicety: a failure here does not lose the list.
	if err := c.get(ctx, "/videos", url.Values{"part": {"statistics"}, "id": {strings.Join(ids, ",")}}, &details); err == nil {
		for _, it := range details.Items {
			stats[it.ID] = struct{ Views, Likes, Comments string }{it.Statistics.ViewCount, it.Statistics.LikeCount, it.Statistics.CommentCount}
		}
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "YouTube — %d videos for %q\n\n", len(found.Items), stringArg(args, "query"))
	for i, it := range found.Items {
		s := stats[it.ID.VideoID]
		fmt.Fprintf(&sb, "%d. %s\n   %s\n   https://www.youtube.com/watch?v=%s\n", i+1, it.Snippet.Title,
			join(it.Snippet.ChannelTitle, date(it.Snippet.PublishedAt), count(s.Views, "views"), count(s.Likes, "likes"), count(s.Comments, "comments")), it.ID.VideoID)
		if d := oneLine(it.Snippet.Description); d != "" {
			fmt.Fprintf(&sb, "   %s\n", shorten(d, 250))
		}
		sb.WriteString("\n")
	}
	return strings.TrimRight(sb.String(), "\n"), nil
}

func date(rfc3339 string) string {
	if len(rfc3339) >= 10 {
		return rfc3339[:10]
	}
	return rfc3339
}

func count(n, label string) string {
	if n == "" {
		return ""
	}
	return n + " " + label
}

func youTubeVideoArg(raw string) (string, error) {
	if youTubeVideoID.MatchString(raw) {
		return raw, nil
	}
	if m := youTubeFromLink.FindStringSubmatch(raw); m != nil {
		return m[1], nil
	}
	return "", fmt.Errorf("video must be a YouTube video id (11 characters) or the link of the video")
}

type youTubeComment struct {
	Snippet struct {
		Author    string `json:"authorDisplayName"`
		Text      string `json:"textDisplay"`
		Likes     int    `json:"likeCount"`
		Published string `json:"publishedAt"`
	} `json:"snippet"`
}

func (c *youTubeClient) comments(ctx context.Context, args map[string]any) (string, error) {
	video, err := youTubeVideoArg(stringArg(args, "video"))
	if err != nil {
		return "", err
	}
	order := stringArg(args, "order")
	if order == "" {
		order = "relevance"
	}
	if !oneOf(order, "relevance", "time") {
		return "", fmt.Errorf("order must be one of: relevance, time")
	}
	limit := intArg(args, "limit", 50, 1, 300)

	type thread struct {
		Snippet struct {
			TopLevel   youTubeComment `json:"topLevelComment"`
			ReplyCount int            `json:"totalReplyCount"`
		} `json:"snippet"`
		Replies struct {
			Comments []youTubeComment `json:"comments"`
		} `json:"replies"`
	}
	var threads []thread
	pageToken := ""
	for len(threads) < limit {
		q := url.Values{"part": {"snippet,replies"}, "videoId": {video}, "order": {order}, "textFormat": {"plainText"},
			"maxResults": {strconv.Itoa(min(100, limit-len(threads)))}}
		if pageToken != "" {
			q.Set("pageToken", pageToken)
		}
		var page struct {
			Items         []thread `json:"items"`
			NextPageToken string   `json:"nextPageToken"`
		}
		if err := c.get(ctx, "/commentThreads", q, &page); err != nil {
			var apiErr *apiError
			if asAPIError(err, &apiErr) && strings.Contains(apiErr.detail, "commentsDisabled") {
				return "", fmt.Errorf("the comments of this video are turned off")
			}
			if len(threads) > 0 {
				break // keep what was read
			}
			return "", err
		}
		threads = append(threads, page.Items...)
		if page.NextPageToken == "" || len(page.Items) == 0 {
			break
		}
		pageToken = page.NextPageToken
	}
	if len(threads) == 0 {
		return fmt.Sprintf("No comments on video %s.", video), nil
	}
	if len(threads) > limit {
		threads = threads[:limit]
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "YouTube — %d comments on https://www.youtube.com/watch?v=%s (%s)\n\n", len(threads), video, order)
	for i, t := range threads {
		top := t.Snippet.TopLevel.Snippet
		fmt.Fprintf(&sb, "%d. %s\n   %s\n", i+1, join(top.Author, fmt.Sprintf("%d likes", top.Likes), date(top.Published), fmt.Sprintf("%d replies", t.Snippet.ReplyCount)), shorten(oneLine(top.Text), 1200))
		for _, r := range t.Replies.Comments {
			fmt.Fprintf(&sb, "     ↳ %s: %s\n", join(r.Snippet.Author, date(r.Snippet.Published)), shorten(oneLine(r.Snippet.Text), 600))
		}
		sb.WriteString("\n")
	}
	return strings.TrimRight(sb.String(), "\n"), nil
}
