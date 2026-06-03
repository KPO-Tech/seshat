package read

const (
	// DefaultLimit is the default number of lines to read
	DefaultLimit = 100

	// MaxLimit is the maximum number of lines to read
	MaxLimit = 10000

	// MaxFileSize is the maximum file size to read (10MB)
	MaxFileSize = 10 * 1024 * 1024

	// MaxImageSize is the maximum image size to read (5MB)
	MaxImageSize = 5 * 1024 * 1024

	// MinLineLength is the minimum line length for binary detection
	MinLineLength = 1000
)

// Blocked device paths that would hang the process
var BlockedDevicePaths = map[string]bool{
	"/dev/zero":    true,
	"/dev/random":  true,
	"/dev/urandom": true,
	"/dev/full":    true,
	"/dev/stdin":   true,
	"/dev/tty":     true,
	"/dev/console": true,
	"/dev/stdout":  true,
	"/dev/stderr":  true,
	"/dev/fd/0":    true,
	"/dev/fd/1":    true,
	"/dev/fd/2":    true,
}
