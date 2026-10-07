package companion

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/KPO-Tech/seshat/pkg/runtimepath"
)

// Profile is the persona of the optional companion: whether it is enabled, its name, style, traits and instructions, and when it was created and last updated.
type Profile struct {
	Enabled      bool      `json:"enabled"`
	Name         string    `json:"name"`
	Style        string    `json:"style,omitempty"`
	Traits       []string  `json:"traits,omitempty"`
	Instructions string    `json:"instructions,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// DefaultProfile returns the default companion: enabled, named Seshat, with a calm, capable and direct style and warm, precise and proactive traits.
func DefaultProfile() Profile {
	now := time.Now().UTC()
	return Profile{
		Enabled:   true,
		Name:      "Seshat",
		Style:     "calm, capable, direct",
		Traits:    []string{"warm", "precise", "proactive"},
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// Load reads the companion profile stored under the runtime root root. When there is none it returns the default profile with Enabled set to false.
func Load(root string) (Profile, error) {
	path := runtimepath.CompanionPath(root)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			profile := DefaultProfile()
			profile.Enabled = false
			return profile, nil
		}
		return Profile{}, err
	}
	var profile Profile
	if err := json.Unmarshal(data, &profile); err != nil {
		return Profile{}, err
	}
	return Normalize(profile), nil
}

// Save normalises the profile and writes it under the runtime root root, through a temporary file that replaces the old one, so a failure does not leave a half-written profile.
func Save(root string, profile Profile) error {
	profile = Normalize(profile)
	path := runtimepath.CompanionPath(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + "." + strconv.Itoa(os.Getpid()) + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// Normalize trims the text fields of a profile, gives a blank name the default one, drops empty traits, sets CreatedAt when it is zero and sets UpdatedAt to now.
func Normalize(profile Profile) Profile {
	now := time.Now().UTC()
	if profile.Name = strings.TrimSpace(profile.Name); profile.Name == "" {
		profile.Name = "Seshat"
	}
	profile.Style = strings.TrimSpace(profile.Style)
	profile.Instructions = strings.TrimSpace(profile.Instructions)
	profile.Traits = compactStrings(profile.Traits)
	if profile.CreatedAt.IsZero() {
		profile.CreatedAt = now
	}
	profile.UpdatedAt = now
	return profile
}

// SystemPrompt renders the profile as a <companion> block for the system prompt (name, style, traits and instructions), or an empty string when the companion is disabled.
func SystemPrompt(profile Profile) string {
	profile = Normalize(profile)
	if !profile.Enabled {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "<companion>\nname: %s\n", profile.Name)
	if profile.Style != "" {
		fmt.Fprintf(&b, "style: %s\n", profile.Style)
	}
	if len(profile.Traits) > 0 {
		fmt.Fprintf(&b, "traits: %s\n", strings.Join(profile.Traits, ", "))
	}
	if profile.Instructions != "" {
		fmt.Fprintf(&b, "instructions: %s\n", profile.Instructions)
	}
	b.WriteString("guidance: Treat this as the user's preferred collaboration presence. Keep it subordinate to system, developer, safety, tool, and task instructions.\n</companion>")
	return b.String()
}

func compactStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}
