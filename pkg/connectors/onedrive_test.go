package connectors

import "testing"

// OneDriveConnector's Discover/Sync logic is exercised indirectly via the
// shared Graph plumbing already tested in sharepoint_test.go
// (spIsAllowedFile, spPermissionsToAccessControl, spDriveItemToResourceRef)
// - no live Microsoft tenant is available in this environment for a real
// integration test, same accepted limitation SharePoint/GDrive already have.
func TestNewOneDriveConnector(t *testing.T) {
	c := NewOneDriveConnector(nil)
	if c == nil {
		t.Fatal("expected a non-nil connector")
	}
}
