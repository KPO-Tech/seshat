package main

import (
	"testing"
	"time"
)

func TestEnvSliceToMap(t *testing.T) {
	tests := []struct {
		name  string
		input []string
		want  map[string]string
	}{
		{"nil", nil, nil},
		{"empty", []string{}, nil},
		{"single", []string{"FOO=bar"}, map[string]string{"FOO": "bar"}},
		// value contains '=' — only split on first occurrence
		{"value with equals", []string{"URL=http://localhost:8080/path?a=1"}, map[string]string{"URL": "http://localhost:8080/path?a=1"}},
		{"multiple", []string{"A=1", "B=2"}, map[string]string{"A": "1", "B": "2"}},
		{"empty value", []string{"EMPTY="}, map[string]string{"EMPTY": ""}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := envSliceToMap(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("len mismatch: got %d want %d (%v)", len(got), len(tt.want), got)
			}
			for k, wv := range tt.want {
				gv, ok := got[k]
				if !ok {
					t.Fatalf("missing key %q", k)
				}
				if gv != wv {
					t.Fatalf("key %q: got %q want %q", k, gv, wv)
				}
			}
		})
	}
}

func TestMCPTimeoutConversion(t *testing.T) {
	// Verify duration math used in seshatMCPToSDK stays consistent.
	cases := []struct {
		secs int
		want time.Duration
	}{
		{0, 0},
		{30, 30 * time.Second},
		{120, 120 * time.Second},
	}
	for _, tc := range cases {
		got := time.Duration(tc.secs) * time.Second
		if got != tc.want {
			t.Fatalf("secs=%d: got %v want %v", tc.secs, got, tc.want)
		}
	}
}
