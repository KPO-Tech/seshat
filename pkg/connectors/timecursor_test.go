package connectors

import (
	"testing"
	"time"
)

func TestTimeCursorRoundTrips(t *testing.T) {
	now := time.Date(2026, 3, 4, 12, 30, 0, 123456789, time.UTC)
	cursor := TimeCursor{Since: now}
	encoded := cursor.Encode()

	decoded, err := DecodeTimeCursor(encoded)
	if err != nil {
		t.Fatalf("DecodeTimeCursor: %v", err)
	}
	if !decoded.Since.Equal(now) {
		t.Fatalf("expected cursor to round-trip to %v, got %v (encoded as %q)", now, decoded.Since, encoded)
	}
}

func TestDecodeTimeCursorEmptyMeansBootstrap(t *testing.T) {
	decoded, err := DecodeTimeCursor("")
	if err != nil {
		t.Fatalf("DecodeTimeCursor: %v", err)
	}
	if !decoded.Since.IsZero() {
		t.Fatalf("expected an empty cursor to decode to the zero time (bootstrap sync), got %v", decoded.Since)
	}
}

func TestDecodeTimeCursorRejectsInvalidInput(t *testing.T) {
	if _, err := DecodeTimeCursor("not-a-timestamp"); err == nil {
		t.Fatal("expected an error for an unparseable cursor")
	}
	// RFC3339 (whole seconds, no sub-second precision) is a different,
	// stricter-looking but actually acceptable format Parse still handles -
	// only genuinely malformed input should error.
	if _, err := DecodeTimeCursor("2026-03-04T12:30:00Z"); err != nil {
		t.Fatalf("expected a plain RFC3339 timestamp to still parse, got: %v", err)
	}
}

func TestNewTimeCursorNowIsRecentAndEncodable(t *testing.T) {
	before := time.Now().UTC()
	cursor := NewTimeCursorNow()
	after := time.Now().UTC()

	if cursor.Since.Before(before) || cursor.Since.After(after) {
		t.Fatalf("expected NewTimeCursorNow to capture the current time, got %v (window %v..%v)", cursor.Since, before, after)
	}
	decoded, err := DecodeTimeCursor(cursor.Encode())
	if err != nil {
		t.Fatalf("DecodeTimeCursor(NewTimeCursorNow().Encode()): %v", err)
	}
	if !decoded.Since.Equal(cursor.Since) {
		t.Fatalf("expected the encoded/decoded cursor to match, got %v want %v", decoded.Since, cursor.Since)
	}
}
