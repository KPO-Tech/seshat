package shared

import (
	"fmt"
	"strings"
)

// IsUNCPath reports whether path is a UNC/network path (\\server\share or //server/share).
// These can leak NTLM credentials on Windows and are blocked unconditionally.
func IsUNCPath(path string) bool {
	return strings.HasPrefix(path, "\\\\") || strings.HasPrefix(path, "//")
}

// ValidateUNCPathSecurity returns an error when path is a UNC/network path.
func ValidateUNCPathSecurity(path string) error {
	if IsUNCPath(path) {
		return fmt.Errorf("UNC/network paths are not allowed for security reasons (potential NTLM credential leaks): %s", path)
	}
	return nil
}
