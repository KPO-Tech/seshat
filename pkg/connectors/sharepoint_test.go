package connectors

import "testing"

func TestSPPermissionsToAccessControl(t *testing.T) {
	cases := []struct {
		name  string
		perms []spGraphPermission
		want  []AccessEntry
	}{
		{
			name: "granted user with siteUser loginName maps to user entry",
			perms: []spGraphPermission{
				{GrantedToV2: &spGraphIdentitySet{SiteUser: &spGraphSiteUser{LoginName: "alice@example.com"}}},
			},
			want: []AccessEntry{"user:alice@example.com"},
		},
		{
			name: "granted user with no loginName is skipped, not guessed",
			perms: []spGraphPermission{
				{GrantedToV2: &spGraphIdentitySet{User: &spGraphIdentity{ID: "guid-only", DisplayName: "Bob"}}},
			},
			want: nil,
		},
		{
			name: "unredeemed invitation email maps to user entry",
			perms: []spGraphPermission{
				{Invitation: &struct {
					Email string `json:"email"`
				}{Email: "invitee@example.com"}},
			},
			want: []AccessEntry{"user:invitee@example.com"},
		},
		{
			name: "anonymous link maps to public",
			perms: []spGraphPermission{
				{Link: &struct {
					Scope string `json:"scope"`
				}{Scope: "anonymous"}},
			},
			want: []AccessEntry{"public"},
		},
		{
			name: "organization link is deliberately NOT mapped to public",
			perms: []spGraphPermission{
				{Link: &struct {
					Scope string `json:"scope"`
				}{Scope: "organization"}},
			},
			want: nil,
		},
		{
			name: "grantedToIdentitiesV2 collection is also mapped",
			perms: []spGraphPermission{
				{GrantedToIdentitiesV2: []spGraphIdentitySet{
					{SiteUser: &spGraphSiteUser{LoginName: "carol@example.com"}},
				}},
			},
			want: []AccessEntry{"user:carol@example.com"},
		},
		{
			name: "duplicate identities across permissions are deduplicated",
			perms: []spGraphPermission{
				{GrantedToV2: &spGraphIdentitySet{SiteUser: &spGraphSiteUser{LoginName: "dave@example.com"}}},
				{GrantedToV2: &spGraphIdentitySet{SiteUser: &spGraphSiteUser{LoginName: "dave@example.com"}}},
			},
			want: []AccessEntry{"user:dave@example.com"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := spPermissionsToAccessControl(tc.perms)
			if len(got) != len(tc.want) {
				t.Fatalf("expected %v, got %v", tc.want, got)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("expected %v, got %v", tc.want, got)
				}
			}
		})
	}
}

func TestSPIsAllowedFile(t *testing.T) {
	allowed := spGraphDriveItem{Name: "report.pdf", File: &struct{}{}}
	if !spIsAllowedFile(allowed) {
		t.Fatal("expected a .pdf file to be allowed")
	}

	notInAllowlist := spGraphDriveItem{Name: "script.py", File: &struct{}{}}
	if spIsAllowedFile(notInAllowlist) {
		t.Fatal("expected a .py file to be excluded (code, not business documents)")
	}

	folder := spGraphDriveItem{Name: "Reports", Folder: &struct{}{}}
	if spIsAllowedFile(folder) {
		t.Fatal("expected a folder to never be allowed")
	}

	pkg := spGraphDriveItem{Name: "Notebook.one", Package: &struct{}{}}
	if spIsAllowedFile(pkg) {
		t.Fatal("expected a package item (e.g. OneNote) to never be allowed")
	}

	deleted := spGraphDriveItem{Name: "report.pdf", File: &struct{}{}, Deleted: &struct{}{}}
	if spIsAllowedFile(deleted) {
		t.Fatal("expected a delta-feed deletion to never be allowed")
	}
}

func TestSPDriveCursorRoundTrips(t *testing.T) {
	cursor := SPDriveCursor{"drive-a": "link-a", "drive-b": "link-b"}
	encoded := cursor.Encode()

	decoded := DecodeSPDriveCursor(encoded)
	if len(decoded) != 2 || decoded["drive-a"] != "link-a" || decoded["drive-b"] != "link-b" {
		t.Fatalf("expected cursor to round-trip, got %v from %q", decoded, encoded)
	}
}

func TestSPDecodeCursorHandlesEmptyAndInvalidInput(t *testing.T) {
	if got := DecodeSPDriveCursor(""); len(got) != 0 {
		t.Fatalf("expected an empty cursor for empty input, got %v", got)
	}
	if got := DecodeSPDriveCursor("not json"); len(got) != 0 {
		t.Fatalf("expected an empty cursor for invalid input, got %v", got)
	}
}

func TestNewSharePointConnector(t *testing.T) {
	c := NewSharePointConnector(nil)
	if c == nil {
		t.Fatal("expected a non-nil connector")
	}
}
