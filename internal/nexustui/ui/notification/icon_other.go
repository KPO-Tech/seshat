//go:build !darwin

package notification

import (
	_ "embed"
)

//go:embed nexus-icon-solo.png
var Icon []byte
