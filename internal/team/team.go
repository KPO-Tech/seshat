package team

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/EngineerProjects/nexus-engine/internal/agent"
	"github.com/EngineerProjects/nexus-engine/internal/db"
	"github.com/google/uuid"
)

// Team is a named, persistent group of agents. Its ID is the stable routing
// key stored in AgentProfile.TeamID — use it when dispatching or broadcasting.
type Team struct {
	// ID is a UUID — globally unique, never changes.
	ID string `json:"id"`

	// Name is a human-readable label, unique across all teams.
	Name string `json:"name"`

	// Description explains the team's purpose.
	Description string `json:"description,omitempty"`

	// Metadata holds arbitrary extension fields.
	Metadata map[string]string `json:"metadata,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// NewTeam creates a Team with a freshly generated UUID.
func NewTeam(name, description string) Team {
	now := time.Now().UTC()
	return Team{
		ID:          uuid.New().String(),
		Name:        name,
		Description: description,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

// ErrTeamNotFound is returned when a team lookup yields no result.
var ErrTeamNotFound = db.ErrTeamNotFound

// TeamRegistry stores and retrieves Team records backed by SQLite.
// Member assignment updates AgentProfile.TeamID directly so the mailbox
// routing keys stay consistent without a separate join table.
type TeamRegistry struct {
	db       *db.DB
	profiles *agent.ProfileRegistry
}

// NewTeamRegistry creates a TeamRegistry wired to the given DB and profile
// registry (used for member queries and assignment).
func NewTeamRegistry(database *db.DB, profiles *agent.ProfileRegistry) *TeamRegistry {
	return &TeamRegistry{db: database, profiles: profiles}
}

// Create persists a new team. Returns an error when the name is already taken.
func (r *TeamRegistry) Create(ctx context.Context, t Team) error {
	if t.ID == "" {
		return errors.New("team ID must not be empty — use NewTeam to generate one")
	}
	if t.Name == "" {
		return errors.New("team name must not be empty")
	}
	t.UpdatedAt = time.Now().UTC()
	if t.CreatedAt.IsZero() {
		t.CreatedAt = t.UpdatedAt
	}
	return r.db.UpsertTeam(ctx, toGTeam(t))
}

// Update replaces the team's mutable fields (Name, Description, Metadata).
func (r *TeamRegistry) Update(ctx context.Context, t Team) error {
	if t.ID == "" {
		return errors.New("team ID must not be empty")
	}
	t.UpdatedAt = time.Now().UTC()
	return r.db.UpsertTeam(ctx, toGTeam(t))
}

// Get returns the team with the given UUID.
// Returns ErrTeamNotFound when absent.
func (r *TeamRegistry) Get(ctx context.Context, id string) (Team, error) {
	row, err := r.db.GetTeam(ctx, id)
	if err != nil {
		return Team{}, err
	}
	return fromGTeam(row)
}

// GetByName returns the team whose name matches exactly (case-sensitive).
// Returns ErrTeamNotFound when absent.
func (r *TeamRegistry) GetByName(ctx context.Context, name string) (Team, error) {
	row, err := r.db.GetTeamByName(ctx, name)
	if err != nil {
		return Team{}, err
	}
	return fromGTeam(row)
}

// List returns all teams ordered by name.
func (r *TeamRegistry) List(ctx context.Context) ([]Team, error) {
	rows, err := r.db.ListTeams(ctx)
	if err != nil {
		return nil, err
	}
	teams := make([]Team, 0, len(rows))
	for _, row := range rows {
		t, err := fromGTeam(row)
		if err != nil {
			return nil, err
		}
		teams = append(teams, t)
	}
	return teams, nil
}

// Delete removes the team record. Agent profiles that reference this team's ID
// keep their TeamID field — callers should RemoveMember each agent first when
// disbanding cleanly.
func (r *TeamRegistry) Delete(ctx context.Context, id string) error {
	return r.db.DeleteTeam(ctx, id)
}

// AddMember assigns an agent to this team by updating AgentProfile.TeamID.
// An agent belongs to at most one team; re-calling with a different teamID
// moves the agent.
func (r *TeamRegistry) AddMember(ctx context.Context, teamID, agentID string) error {
	if teamID == "" {
		return errors.New("teamID must not be empty")
	}
	if agentID == "" {
		return errors.New("agentID must not be empty")
	}
	return r.db.SetProfileTeam(ctx, agentID, teamID)
}

// RemoveMember clears AgentProfile.TeamID for the given agent.
func (r *TeamRegistry) RemoveMember(ctx context.Context, agentID string) error {
	if agentID == "" {
		return errors.New("agentID must not be empty")
	}
	return r.db.SetProfileTeam(ctx, agentID, "")
}

// Members returns all AgentProfiles currently assigned to the given team.
func (r *TeamRegistry) Members(ctx context.Context, teamID string) ([]agent.AgentProfile, error) {
	return r.profiles.FindByTeam(ctx, teamID)
}

// ─── conversion helpers ───────────────────────────────────────────────────────

func toGTeam(t Team) db.GTeam {
	metaJSON := "{}"
	if len(t.Metadata) > 0 {
		if b, err := json.Marshal(t.Metadata); err == nil {
			metaJSON = string(b)
		}
	}
	return db.GTeam{
		ID:            t.ID,
		Name:          t.Name,
		Description:   t.Description,
		MetadataJSON:  metaJSON,
		CreatedAtUnix: t.CreatedAt.Unix(),
		UpdatedAtUnix: t.UpdatedAt.Unix(),
	}
}

func fromGTeam(row db.GTeam) (Team, error) {
	var meta map[string]string
	if row.MetadataJSON != "" && row.MetadataJSON != "{}" {
		if err := json.Unmarshal([]byte(row.MetadataJSON), &meta); err != nil {
			return Team{}, fmt.Errorf("parse metadata for team %q: %w", row.ID, err)
		}
	}
	return Team{
		ID:          row.ID,
		Name:        row.Name,
		Description: row.Description,
		Metadata:    meta,
		CreatedAt:   time.Unix(row.CreatedAtUnix, 0).UTC(),
		UpdatedAt:   time.Unix(row.UpdatedAtUnix, 0).UTC(),
	}, nil
}
