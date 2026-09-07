package rag

import "testing"

func TestDefaultChunkProfileIsMedium(t *testing.T) {
	profile := DefaultChunkProfile()
	if profile.Name != ChunkProfileMedium {
		t.Fatalf("expected default profile %q, got %q", ChunkProfileMedium, profile.Name)
	}
	if profile.MaxTokens != 768 {
		t.Fatalf("expected medium max tokens 768, got %d", profile.MaxTokens)
	}
	if profile.OverlapTokens != 120 {
		t.Fatalf("expected medium overlap 120, got %d", profile.OverlapTokens)
	}
}

func TestRecommendedChunkProfileNormalizesName(t *testing.T) {
	profile, ok := RecommendedChunkProfile("  LARGE  ")
	if !ok {
		t.Fatal("expected large profile to resolve")
	}
	if profile.Name != ChunkProfileLarge || profile.MaxTokens != 1536 {
		t.Fatalf("unexpected large profile: %+v", profile)
	}
}

func TestNewCustomChunkProfileValidatesOverlap(t *testing.T) {
	if _, err := NewCustomChunkProfile(512, 512); err == nil {
		t.Fatal("expected invalid overlap to fail")
	}
	profile, err := NewCustomChunkProfile(640, 96)
	if err != nil {
		t.Fatalf("NewCustomChunkProfile: %v", err)
	}
	if profile.Name != ChunkProfileCustom || profile.MaxTokens != 640 || profile.OverlapTokens != 96 {
		t.Fatalf("unexpected custom profile: %+v", profile)
	}
}
