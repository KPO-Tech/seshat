//go:build !darwin

package notification

import (
	_ "embed"
)

//go:embed seshat-icon-solo.png
var Icon []byte
