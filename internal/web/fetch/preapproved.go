package fetch

import webcore "github.com/EngineerProjects/nexus-engine/internal/web"

// PreapprovedHosts re-exports the shared documentation host allowlist for backward compatibility.
var PreapprovedHosts = webcore.PreapprovedHosts

// PathPrefixes keeps the old fetch-local name as an alias to the shared path-scoped allowlist.
var PathPrefixes = webcore.PreapprovedPathPrefixes

// IsPreapproved reports whether the hostname is covered by the shared preapproval policy.
func IsPreapproved(hostname string) bool {
	return webcore.IsPreapproved(hostname)
}

// IsPreapprovedPath reports whether the host/path pair is covered by the shared preapproval policy.
func IsPreapprovedPath(hostname, pathname string) bool {
	return webcore.IsPreapprovedPath(hostname, pathname)
}
