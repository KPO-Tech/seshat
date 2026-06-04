package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/EngineerProjects/nexus-engine/internal/db"
)

// ProfileRegistry stores and retrieves AgentProfile records backed by SQLite.
// It seeds built-in profiles on first use and never overwrites user-customised
// versions of those profiles.
type ProfileRegistry struct {
	db     *db.DB
	mu     sync.Mutex
	seeded bool
}

// NewProfileRegistry creates a ProfileRegistry backed by the given DB.
func NewProfileRegistry(database *db.DB) *ProfileRegistry {
	return &ProfileRegistry{db: database}
}

// Seed inserts built-in profiles that do not already exist. Idempotent.
func (r *ProfileRegistry) Seed(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.seeded {
		return nil
	}
	for _, p := range BuiltInProfiles() {
		row := toGProfile(p)
		if err := r.db.UpsertProfileIfAbsent(ctx, row); err != nil {
			return fmt.Errorf("seed profile %q: %w", p.ID, err)
		}
	}
	r.seeded = true
	return nil
}

// Register inserts or fully replaces the profile.
func (r *ProfileRegistry) Register(ctx context.Context, p AgentProfile) error {
	if p.ID == "" {
		return errors.New("profile ID must not be empty")
	}
	p.UpdatedAt = time.Now().UTC()
	if p.CreatedAt.IsZero() {
		p.CreatedAt = p.UpdatedAt
	}
	return r.db.UpsertProfile(ctx, toGProfile(p))
}

// Get returns the profile with the given ID.
// Returns ErrProfileNotFound when no record matches.
func (r *ProfileRegistry) Get(ctx context.Context, id string) (AgentProfile, error) {
	row, err := r.db.GetProfile(ctx, id)
	if err != nil {
		return AgentProfile{}, err
	}
	return fromGProfile(row)
}

// List returns all profiles ordered by ID.
func (r *ProfileRegistry) List(ctx context.Context) ([]AgentProfile, error) {
	rows, err := r.db.ListProfiles(ctx)
	if err != nil {
		return nil, err
	}
	profiles := make([]AgentProfile, 0, len(rows))
	for _, row := range rows {
		p, err := fromGProfile(row)
		if err != nil {
			return nil, err
		}
		profiles = append(profiles, p)
	}
	return profiles, nil
}

// FindByRole returns all profiles whose Role matches the given tag
// (case-insensitive).
func (r *ProfileRegistry) FindByRole(ctx context.Context, role string) ([]AgentProfile, error) {
	rows, err := r.db.FindProfilesByRole(ctx, strings.ToLower(role))
	if err != nil {
		return nil, err
	}
	profiles := make([]AgentProfile, 0, len(rows))
	for _, row := range rows {
		p, err := fromGProfile(row)
		if err != nil {
			return nil, err
		}
		profiles = append(profiles, p)
	}
	return profiles, nil
}

// Delete removes the profile with the given ID. No-op if absent.
func (r *ProfileRegistry) Delete(ctx context.Context, id string) error {
	return r.db.DeleteProfile(ctx, id)
}

// ─── conversion helpers ───────────────────────────────────────────────────────

func toGProfile(p AgentProfile) db.GAgentProfile {
	skillsJSON := "[]"
	if len(p.Skills) > 0 {
		if b, err := json.Marshal(p.Skills); err == nil {
			skillsJSON = string(b)
		}
	}
	metaJSON := "{}"
	if len(p.Metadata) > 0 {
		if b, err := json.Marshal(p.Metadata); err == nil {
			metaJSON = string(b)
		}
	}
	return db.GAgentProfile{
		ID:            p.ID,
		Name:          p.Name,
		Role:          strings.ToLower(p.Role),
		SystemPrompt:  p.SystemPrompt,
		Model:         p.Model,
		SkillsJSON:    skillsJSON,
		MetadataJSON:  metaJSON,
		CreatedAtUnix: p.CreatedAt.Unix(),
		UpdatedAtUnix: p.UpdatedAt.Unix(),
	}
}

func fromGProfile(row db.GAgentProfile) (AgentProfile, error) {
	var skills []string
	if row.SkillsJSON != "" && row.SkillsJSON != "[]" {
		if err := json.Unmarshal([]byte(row.SkillsJSON), &skills); err != nil {
			return AgentProfile{}, fmt.Errorf("parse skills for profile %q: %w", row.ID, err)
		}
	}
	var meta map[string]string
	if row.MetadataJSON != "" && row.MetadataJSON != "{}" {
		if err := json.Unmarshal([]byte(row.MetadataJSON), &meta); err != nil {
			return AgentProfile{}, fmt.Errorf("parse metadata for profile %q: %w", row.ID, err)
		}
	}
	return AgentProfile{
		ID:           row.ID,
		Name:         row.Name,
		Role:         row.Role,
		SystemPrompt: row.SystemPrompt,
		Model:        row.Model,
		Skills:       skills,
		Metadata:     meta,
		CreatedAt:    time.Unix(row.CreatedAtUnix, 0).UTC(),
		UpdatedAt:    time.Unix(row.UpdatedAtUnix, 0).UTC(),
	}, nil
}
