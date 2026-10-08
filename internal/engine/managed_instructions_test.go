package engine

import "testing"

func TestWithManagedInstructionsComesLastAndSurvivesASessionOverride(t *testing.T) {
	str := func(s string) *string { return &s }
	cases := []struct {
		name    string
		append  *string
		managed string
		want    string
		same    bool
	}{
		{"nothing managed keeps the session text", str("session text"), "", "session text", true},
		{"blank managed keeps the session text", str("session text"), "  \n ", "session text", true},
		{"managed alone", nil, "rules", "rules", false},
		{"managed after the session text", str("session text"), "rules", "session text\n\nrules", false},
		{"managed after an empty session override", str("   "), "rules", "rules", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := withManagedInstructions(tc.append, tc.managed)
			if got == nil || *got != tc.want {
				t.Fatalf("got %v, want %q", got, tc.want)
			}
			if tc.same && got != tc.append {
				t.Fatal("with nothing to add, the session pointer must be returned unchanged")
			}
		})
	}
	if got := withManagedInstructions(nil, ""); got != nil {
		t.Fatalf("nothing at all must stay nil, got %q", *got)
	}
}
