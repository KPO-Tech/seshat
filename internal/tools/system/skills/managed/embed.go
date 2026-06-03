package managed

import "embed"

// FS holds embedded builtin skill files under builtin/.
// The builtin/ directory is intentionally empty — skills are now distributed
// via the nexus-skills git repository and auto-cloned at first boot.
//
//go:embed builtin
var FS embed.FS

// Version must be bumped whenever skill files change so EnsureExtracted
// re-extracts on the next boot. Bumped to 2.0.0 when builtin skills were
// moved to github.com/EngineerProjects/nexus-skills.
const Version = "2.0.0"
