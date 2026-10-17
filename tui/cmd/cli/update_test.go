package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestConfirmDefaultsToYes(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"\n", true},
		{"y\n", true},
		{"YES\n", true},
		{"oui\n", true},
		{"n\n", false},
		{"no\n", false},
		{"later\n", false},
		{"", true}, // end of input on an empty line behaves like Enter
	}
	for _, tt := range tests {
		var out bytes.Buffer
		got, err := confirm(strings.NewReader(tt.input), &out, "Install?")
		if err != nil {
			t.Fatalf("%q: %v", tt.input, err)
		}
		if got != tt.want {
			t.Errorf("confirm(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestUpdateOnADevelopmentBuildExplainsInsteadOfFailing(t *testing.T) {
	previous := version
	version = "dev"
	t.Cleanup(func() { version = previous })

	var out, errOut bytes.Buffer
	if err := runUpdate(context.Background(), nil, strings.NewReader(""), &out, &errOut); err != nil {
		t.Fatalf("runUpdate: %v", err)
	}
	if !strings.Contains(out.String(), "development build") {
		t.Errorf("expected an explanation, got %q", out.String())
	}
}

func TestUpdateRefusesStrayArguments(t *testing.T) {
	var out, errOut bytes.Buffer
	if err := runUpdate(context.Background(), []string{"extra"}, strings.NewReader(""), &out, &errOut); err == nil {
		t.Fatal("expected an error for a stray argument")
	}
}

// The offer talks to the user, so it must stay out of the way of scripts, pipes, CI and development builds.
func TestNoUpdateOfferOutsideAnInteractiveReleaseRun(t *testing.T) {
	previous := version
	t.Cleanup(func() { version = previous })

	// Not a terminal: the reader is not a file.
	version = "v1.9.0"
	if updateChecksAllowed(strings.NewReader("")) {
		t.Error("a piped input must not be offered an update")
	}
	if offerUpdate(context.Background(), strings.NewReader(""), &bytes.Buffer{}) {
		t.Error("offerUpdate must do nothing without a terminal")
	}

	version = "dev"
	if updateChecksAllowed(strings.NewReader("")) {
		t.Error("a development build must not be offered an update")
	}

	t.Setenv(envNoUpdateCheck, "1")
	version = "v1.9.0"
	if updateChecksAllowed(strings.NewReader("")) {
		t.Error("SESHAT_NO_UPDATE_CHECK must turn the check off")
	}
}
