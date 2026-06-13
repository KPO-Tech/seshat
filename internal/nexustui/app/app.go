// Package app exposes the types used by the UI for app-level events.
// The actual application wiring is done by the workspace adapter.
package app

// UpdateAvailableMsg is sent when a new version of the application is available.
type UpdateAvailableMsg struct {
	CurrentVersion string
	LatestVersion  string
	IsDevelopment  bool
}
