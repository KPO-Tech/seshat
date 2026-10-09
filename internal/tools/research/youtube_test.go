package research

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func youTubeFixture(t *testing.T) (*youTubeClient, *Keys, *[]string) {
	t.Helper()
	var requested []string
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		requested = append(requested, r.URL.String())
		if r.URL.Query().Get("key") != "the-key" {
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `{"error":{"errors":[{"reason":"keyInvalid"}]}}`)
			return
		}
		switch r.URL.Path {
		case "/search":
			fmt.Fprint(w, `{"items":[
 {"id":{"videoId":"aaaaaaaaaaa"},"snippet":{"title":"Todo Pro review","channelTitle":"Tech Reviews","publishedAt":"2026-09-01T12:00:00Z","description":"We test it\nfor a week."}},
 {"id":{"videoId":"bbbbbbbbbbb"},"snippet":{"title":"Todo Pro vs Things","channelTitle":"Apps Daily","publishedAt":"2026-08-01T12:00:00Z","description":""}}]}`)
		case "/videos":
			fmt.Fprint(w, `{"items":[{"id":"aaaaaaaaaaa","statistics":{"viewCount":"120000","likeCount":"4500","commentCount":"310"}}]}`)
		case "/commentThreads":
			if r.URL.Query().Get("videoId") == "disabledvid" {
				w.WriteHeader(http.StatusForbidden)
				fmt.Fprint(w, `{"error":{"errors":[{"reason":"commentsDisabled"}]}}`)
				return
			}
			if r.URL.Query().Get("pageToken") == "" {
				fmt.Fprint(w, `{"nextPageToken":"p2","items":[
 {"snippet":{"totalReplyCount":1,"topLevelComment":{"snippet":{"authorDisplayName":"Ana","textDisplay":"Great app,\nbut pricey.","likeCount":12,"publishedAt":"2026-09-02T08:00:00Z"}}},
  "replies":{"comments":[{"snippet":{"authorDisplayName":"Ben","textDisplay":"Agreed.","publishedAt":"2026-09-03T08:00:00Z"}}]}}]}`)
				return
			}
			fmt.Fprint(w, `{"items":[{"snippet":{"totalReplyCount":0,"topLevelComment":{"snippet":{"authorDisplayName":"Cy","textDisplay":"Switched to Things.","likeCount":3,"publishedAt":"2026-09-04T08:00:00Z"}}}}]}`)
		default:
			http.NotFound(w, r)
		}
	})
	keys := NewKeys(map[string]string{KeyYouTubeAPIKey: "the-key"})
	c := newYouTubeClient(keys)
	c.base = srv.URL
	return c, keys, &requested
}

func TestYouTubeSearchAddsTheStatisticsOfEachVideo(t *testing.T) {
	c, keys, requested := youTubeFixture(t)

	out, err := call(t, byName(t, youTubeTools(c, keys), "youtube_search"), map[string]any{"query": "todo pro review", "order": "viewCount", "region": "fr", "limit": float64(2)})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Todo Pro review", "Tech Reviews", "2026-09-01", "120000 views", "4500 likes", "310 comments", "https://www.youtube.com/watch?v=aaaaaaaaaaa", "We test it for a week."} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "Apps Daily · 2026-08-01\n") {
		t.Errorf("a video without statistics is still listed:\n%s", out)
	}
	first := (*requested)[0]
	if !strings.Contains(first, "order=viewCount") || !strings.Contains(first, "regionCode=FR") || !strings.Contains(first, "type=video") {
		t.Errorf("unexpected request %s", first)
	}
}

func TestYouTubeCommentsFollowThePagesAndShowReplies(t *testing.T) {
	c, keys, requested := youTubeFixture(t)

	out, err := call(t, byName(t, youTubeTools(c, keys), "youtube_comments"), map[string]any{"video": "https://youtu.be/aaaaaaaaaaa?t=3", "limit": float64(5)})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"2 comments on https://www.youtube.com/watch?v=aaaaaaaaaaa", "Ana · 12 likes", "Great app, but pricey.", "↳ Ben", "Agreed.", "Cy", "Switched to Things."} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if len(*requested) != 2 || !strings.Contains((*requested)[1], "pageToken=p2") {
		t.Errorf("expected two pages: %v", *requested)
	}
}

func TestYouTubeCommentsSaysWhenTheyAreTurnedOff(t *testing.T) {
	c, keys, _ := youTubeFixture(t)

	_, err := call(t, byName(t, youTubeTools(c, keys), "youtube_comments"), map[string]any{"video": "disabledvid"})
	if err == nil || !strings.Contains(err.Error(), "turned off") {
		t.Fatalf("got %v", err)
	}
}

func TestYouTubeRefusesWhatIsNotAVideoAndSaysWhenTheKeyIsWrong(t *testing.T) {
	c, keys, requested := youTubeFixture(t)
	tools := youTubeTools(c, keys)

	if _, err := call(t, byName(t, tools, "youtube_comments"), map[string]any{"video": "not a video"}); err == nil {
		t.Error("a bad video must be refused")
	}
	if _, err := call(t, byName(t, tools, "youtube_search"), map[string]any{"query": "x", "order": "popular"}); err == nil {
		t.Error("a bad order must be refused")
	}
	if len(*requested) != 0 {
		t.Errorf("nothing must be requested for a refused input: %v", *requested)
	}

	c.keys = NewKeys(map[string]string{KeyYouTubeAPIKey: "wrong"})
	if _, err := call(t, byName(t, tools, "youtube_search"), map[string]any{"query": "x"}); err == nil || !strings.Contains(err.Error(), "no access") {
		t.Errorf("expected a clear refusal, got %v", err)
	}
}
