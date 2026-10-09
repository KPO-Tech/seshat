package research

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

func redditFixture(t *testing.T) (*redditClient, *Keys, *atomic.Int32, *[]string) {
	t.Helper()
	var tokens atomic.Int32
	var requested []string
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/access_token":
			want := "Basic " + base64.StdEncoding.EncodeToString([]byte("the-id:the-secret"))
			if r.Header.Get("Authorization") != want || r.FormValue("grant_type") != "client_credentials" {
				w.WriteHeader(http.StatusUnauthorized)
				fmt.Fprint(w, `{"error":"invalid_grant"}`)
				return
			}
			n := tokens.Add(1)
			fmt.Fprintf(w, `{"access_token":"token-%d","expires_in":3600}`, n)
		case strings.HasSuffix(r.URL.Path, "/search"):
			requested = append(requested, r.URL.String())
			fmt.Fprint(w, `{"data":{"children":[{"kind":"t3","data":{"id":"abc123","title":"Is Todo Pro worth it?","subreddit":"productivity","author":"alice","score":120,"num_comments":45,"created_utc":1790000000,"permalink":"/r/productivity/comments/abc123/is_todo_pro_worth_it/","selftext":"I have used it\nfor a month."}},{"kind":"t5","data":{}}]}}`)
		case strings.HasPrefix(r.URL.Path, "/r/golang/"):
			requested = append(requested, r.URL.String())
			fmt.Fprint(w, `{"data":{"children":[{"kind":"t3","data":{"id":"zzz999","title":"Go 2","subreddit":"golang","author":"bob","score":5,"num_comments":1,"created_utc":1790000000,"permalink":"/r/golang/comments/zzz999/go_2/","selftext":""}}]}}`)
		case strings.HasPrefix(r.URL.Path, "/comments/"):
			requested = append(requested, r.URL.String())
			fmt.Fprint(w, `[{"data":{"children":[{"kind":"t3","data":{"id":"abc123","title":"Is Todo Pro worth it?","subreddit":"productivity","author":"alice","score":120,"num_comments":3,"created_utc":1790000000,"permalink":"/r/productivity/comments/abc123/x/","selftext":"Long body."}}]}},
{"data":{"children":[
 {"kind":"t1","data":{"author":"carol","body":"Yes, the sync is great.","score":30,"created_utc":1790000100,"replies":{"data":{"children":[
   {"kind":"t1","data":{"author":"dave","body":"Not on Android.","score":4,"created_utc":1790000200,"replies":""}},
   {"kind":"more","data":{"count":3}}]}}}},
 {"kind":"t1","data":{"author":"[deleted]","body":"","score":0,"created_utc":1790000300,"replies":""}},
 {"kind":"t1","data":{"author":"erin","body":"Too expensive.","score":9,"created_utc":1790000400,"replies":""}}]}}]`)
		default:
			http.NotFound(w, r)
		}
	})
	keys := NewKeys(map[string]string{KeyRedditClientID: "the-id", KeyRedditClientSecret: "the-secret"})
	c := newRedditClient(keys)
	c.authBase, c.apiBase = srv.URL, srv.URL
	return c, keys, &tokens, &requested
}

func TestRedditSearchGetsATokenOnceAndKeepsIt(t *testing.T) {
	c, keys, tokens, requested := redditFixture(t)
	tl := byName(t, redditTools(c, keys), "reddit_search")

	for i := 0; i < 2; i++ {
		out, err := call(t, tl, map[string]any{"query": "todo pro", "sort": "top", "time": "year", "limit": float64(5)})
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"Is Todo Pro worth it?", "r/productivity", "u/alice", "120 points", "45 comments", "https://www.reddit.com/r/productivity/comments/abc123/", "I have used it for a month."} {
			if !strings.Contains(out, want) {
				t.Errorf("missing %q in:\n%s", want, out)
			}
		}
		if strings.Contains(out, "t5") {
			t.Error("only posts are listed")
		}
	}
	if tokens.Load() != 1 {
		t.Errorf("the token must be asked for once, got %d", tokens.Load())
	}
	if !strings.Contains((*requested)[0], "sort=top") || !strings.Contains((*requested)[0], "t=year") || !strings.Contains((*requested)[0], "limit=5") {
		t.Errorf("unexpected request %s", (*requested)[0])
	}
}

func TestRedditSearchInsideASubredditIsRestrictedToIt(t *testing.T) {
	c, keys, _, requested := redditFixture(t)
	tl := byName(t, redditTools(c, keys), "reddit_search")

	if _, err := call(t, tl, map[string]any{"query": "x", "subreddit": "r/golang"}); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix((*requested)[0], "/r/golang/search") || !strings.Contains((*requested)[0], "restrict_sr=1") {
		t.Errorf("unexpected request %s", (*requested)[0])
	}
}

func TestRedditPostsListsASubreddit(t *testing.T) {
	c, keys, _, requested := redditFixture(t)
	tl := byName(t, redditTools(c, keys), "reddit_posts")

	out, err := call(t, tl, map[string]any{"subreddit": "golang", "sort": "top"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "r/golang — top") || !strings.Contains(out, "Go 2") {
		t.Errorf("unexpected output:\n%s", out)
	}
	if !strings.Contains((*requested)[0], "/r/golang/top") || !strings.Contains((*requested)[0], "t=day") {
		t.Errorf("unexpected request %s", (*requested)[0])
	}
}

func TestRedditPostReadsTheDiscussionIndentedByDepthAndSkipsWhatIsGone(t *testing.T) {
	c, keys, _, requested := redditFixture(t)
	tl := byName(t, redditTools(c, keys), "reddit_post")

	out, err := call(t, tl, map[string]any{"post": "https://www.reddit.com/r/productivity/comments/abc123/is_todo_pro_worth_it/", "limit": float64(10)})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Is Todo Pro worth it?", "Long body.", "- u/carol", "  - u/dave", "Not on Android.", "- u/erin"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "[deleted]") {
		t.Error("a removed comment must be skipped")
	}
	if !strings.HasPrefix((*requested)[0], "/comments/abc123") {
		t.Errorf("unexpected request %s", (*requested)[0])
	}

	limited, _ := call(t, tl, map[string]any{"post": "abc123", "limit": float64(2)})
	if strings.Contains(limited, "u/erin") {
		t.Errorf("the comment limit counts replies too:\n%s", limited)
	}
}

func TestRedditRefusesBadInputsWithoutCallingReddit(t *testing.T) {
	c, keys, tokens, _ := redditFixture(t)
	tools := redditTools(c, keys)

	for name, args := range map[string]map[string]any{
		"reddit_search": {"query": "x", "subreddit": "not a sub!"},
		"reddit_posts":  {"subreddit": "a"},
		"reddit_post":   {"post": "??"},
	} {
		if _, err := call(t, byName(t, tools, name), args); err == nil {
			t.Errorf("%s %v must be refused", name, args)
		}
	}
	if tokens.Load() != 0 {
		t.Error("a refused input must not even ask for a token")
	}
}

func TestRedditSaysWhenTheCredentialsAreWrong(t *testing.T) {
	c, _, _, _ := redditFixture(t)
	wrong := NewKeys(map[string]string{KeyRedditClientID: "the-id", KeyRedditClientSecret: "not-the-secret"})
	c.keys = wrong

	_, err := call(t, byName(t, redditTools(c, wrong), "reddit_search"), map[string]any{"query": "x"})
	if err == nil || !strings.Contains(err.Error(), "no access") {
		t.Fatalf("expected a clear refusal, got %v", err)
	}
}
