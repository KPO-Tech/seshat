package db

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ErrProfileNotFound is returned when a requested agent profile does not exist.
var ErrProfileNotFound = errors.New("agent profile not found")

// GAgentProfile is the GORM model for the agent_profiles table.
type GAgentProfile struct {
	// ID is a UUID string — globally unique, generated once at creation.
	ID       string `gorm:"primaryKey;size:36"`
	Nickname string `gorm:"not null"`
	Role     string `gorm:"not null;index"`
	TeamID   string `gorm:"column:team_id;index"`
	// SystemPromptTemplate is the raw template; {{.Nickname}} is resolved at runtime.
	SystemPromptTemplate string `gorm:"column:system_prompt_template;type:text;not null"`
	Model                string `gorm:"column:model"`
	SkillsJSON           string `gorm:"column:skills_json;type:text;not null;default:'[]'"`
	MetadataJSON         string `gorm:"column:metadata_json;type:text;not null;default:'{}'"`
	CreatedAtUnix        int64  `gorm:"column:created_at_unix;autoCreateTime:unix"`
	UpdatedAtUnix        int64  `gorm:"column:updated_at_unix;autoUpdateTime:unix"`
}

func (GAgentProfile) TableName() string { return "agent_profiles" }

// UpsertProfile inserts or fully replaces a profile record.
func (db *DB) UpsertProfile(ctx context.Context, row GAgentProfile) error {
	return db.gormDB.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"nickname", "role", "team_id", "system_prompt_template",
				"model", "skills_json", "metadata_json", "updated_at_unix",
			}),
		}).Create(&row).Error
}

// UpsertProfileIfAbsent inserts the profile only when no record with the same
// ID exists. Used for seeding built-in profiles without overwriting user edits.
func (db *DB) UpsertProfileIfAbsent(ctx context.Context, row GAgentProfile) error {
	return db.gormDB.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&row).Error
}

// GetProfile returns the profile with the given ID.
// Returns ErrProfileNotFound when no record matches.
func (db *DB) GetProfile(ctx context.Context, id string) (GAgentProfile, error) {
	var row GAgentProfile
	err := db.gormDB.WithContext(ctx).Where("id = ?", id).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return GAgentProfile{}, ErrProfileNotFound
	}
	return row, err
}

// ListProfiles returns all profiles ordered by nickname.
func (db *DB) ListProfiles(ctx context.Context) ([]GAgentProfile, error) {
	var rows []GAgentProfile
	err := db.gormDB.WithContext(ctx).Order("nickname").Find(&rows).Error
	return rows, err
}

// FindProfilesByRole returns all profiles whose role equals the given value.
func (db *DB) FindProfilesByRole(ctx context.Context, role string) ([]GAgentProfile, error) {
	var rows []GAgentProfile
	err := db.gormDB.WithContext(ctx).Where("role = ?", role).Order("nickname").Find(&rows).Error
	return rows, err
}

// FindProfilesByTeam returns all profiles belonging to the given team.
func (db *DB) FindProfilesByTeam(ctx context.Context, teamID string) ([]GAgentProfile, error) {
	var rows []GAgentProfile
	err := db.gormDB.WithContext(ctx).Where("team_id = ?", teamID).Order("nickname").Find(&rows).Error
	return rows, err
}

// DeleteProfile removes the profile with the given ID. No-op if absent.
func (db *DB) DeleteProfile(ctx context.Context, id string) error {
	return db.gormDB.WithContext(ctx).Delete(&GAgentProfile{}, "id = ?", id).Error
}
