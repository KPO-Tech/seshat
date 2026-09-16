package connectors

import (
	"testing"
	"time"
)

// Confluence's own live API shape (search pagination, cloud ID resolution,
// token rotation against a real Atlassian tenant) has no integration test
// here - no real Atlassian tenant/app credentials are available in this
// environment, same accepted limitation already true for SharePoint/
// OneDrive/GDrive's own test files. These are the pure-logic pieces.

func TestNewConfluenceConnector(t *testing.T) {
	c := NewConfluenceConnector(nil)
	if c == nil {
		t.Fatal("expected a non-nil connector")
	}
}

func TestConfluenceRefreshedTokenReportsNoneByDefault(t *testing.T) {
	c := NewConfluenceConnector(nil)
	if _, _, _, ok := c.RefreshedToken(); ok {
		t.Fatal("expected no refreshed token before any call")
	}
}

func TestParseConfluenceTime(t *testing.T) {
	cases := []struct {
		raw     string
		wantErr bool
	}{
		{"2026-08-20T12:00:00.000Z", false},
		{"2026-08-20T12:00:00Z", false},
		{"not a timestamp", true},
	}
	for _, tc := range cases {
		_, ok := parseConfluenceTime(tc.raw)
		if ok == tc.wantErr {
			t.Fatalf("parseConfluenceTime(%q): ok=%v, wantErr=%v", tc.raw, ok, tc.wantErr)
		}
	}
}

func TestParseConfluenceTimeRoundTrips(t *testing.T) {
	want := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	got, ok := parseConfluenceTime("2026-08-20T12:00:00.000Z")
	if !ok {
		t.Fatal("expected a successful parse")
	}
	if !got.Equal(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
