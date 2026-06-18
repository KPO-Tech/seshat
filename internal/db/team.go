package db

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ErrTeamNotFound is returned when a requested team does not exist.
var ErrTeamNotFound = errors.New("team not found")

// GTeam is the GORM model for the teams table.
type GTeam struct {
	ID            string `gorm:"primaryKey;size:36"`
	Name          string `gorm:"not null;uniqueIndex"`
	Description   string `gorm:"column:description;type:text;not null;default:''"`
	MetadataJSON  string `gorm:"column:metadata_json;type:text;not null;default:'{}'"`
	CreatedAtUnix int64  `gorm:"column:created_at_unix;autoCreateTime:unix"`
	UpdatedAtUnix int64  `gorm:"column:updated_at_unix;autoUpdateTime:unix"`
}

func (GTeam) TableName() string { return "teams" }

// UpsertTeam inserts or fully replaces a team record.
func (db *DB) UpsertTeam(ctx context.Context, row GTeam) error {
	return db.gormDB.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"name", "description", "metadata_json", "updated_at_unix",
			}),
		}).Create(&row).Error
}

// GetTeam returns the team with the given ID.
// Returns ErrTeamNotFound when no record matches.
func (db *DB) GetTeam(ctx context.Context, id string) (GTeam, error) {
	var row GTeam
	err := db.gormDB.WithContext(ctx).Where("id = ?", id).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return GTeam{}, ErrTeamNotFound
	}
	return row, err
}

// GetTeamByName returns the team with the given name.
// Returns ErrTeamNotFound when no record matches.
func (db *DB) GetTeamByName(ctx context.Context, name string) (GTeam, error) {
	var row GTeam
	err := db.gormDB.WithContext(ctx).Where("name = ?", name).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return GTeam{}, ErrTeamNotFound
	}
	return row, err
}

// ListTeams returns all teams ordered by name.
func (db *DB) ListTeams(ctx context.Context) ([]GTeam, error) {
	var rows []GTeam
	err := db.gormDB.WithContext(ctx).Order("name").Find(&rows).Error
	return rows, err
}

// DeleteTeam removes the team with the given ID. No-op if absent.
func (db *DB) DeleteTeam(ctx context.Context, id string) error {
	return db.gormDB.WithContext(ctx).Delete(&GTeam{}, "id = ?", id).Error
}

// SetProfileTeam updates team_id on a single agent profile.
// Pass an empty teamID to remove the agent from its team.
func (db *DB) SetProfileTeam(ctx context.Context, agentID, teamID string) error {
	return db.gormDB.WithContext(ctx).
		Model(&GAgentProfile{}).
		Where("id = ?", agentID).
		Update("team_id", teamID).Error
}
