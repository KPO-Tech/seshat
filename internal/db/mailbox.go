package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ErrMessageNotFound is returned when a requested mailbox message does not exist.
var ErrMessageNotFound = errors.New("mailbox message not found")

// GMailboxMessage is the GORM model for the mailbox_messages table.
type GMailboxMessage struct {
	ID        string `gorm:"primaryKey;size:36"`
	Kind      string `gorm:"not null;index"`
	FromAgent string `gorm:"column:from_agent;not null;index"`
	ToAgent   string `gorm:"column:to_agent;not null;index"`
	Subject   string `gorm:"not null"`
	Body      string `gorm:"type:text;not null"`
	ReplyTo   string `gorm:"column:reply_to;index"`
	TeamID    string `gorm:"column:team_id;index"`
	ReadAt    *int64 `gorm:"column:read_at"`
	CreatedAt int64  `gorm:"column:created_at;not null;index"`
}

func (GMailboxMessage) TableName() string { return "mailbox_messages" }

// InsertMessage writes a new message record.
func (db *DB) InsertMessage(ctx context.Context, row GMailboxMessage) error {
	return db.gormDB.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&row).Error
}

// GetUnreadMessages returns all unread messages for toAgent, oldest first.
func (db *DB) GetUnreadMessages(ctx context.Context, toAgent string) ([]GMailboxMessage, error) {
	var rows []GMailboxMessage
	err := db.gormDB.WithContext(ctx).
		Where("to_agent = ? AND read_at IS NULL", toAgent).
		Order("created_at ASC").
		Find(&rows).Error
	return rows, err
}

// MarkMessageRead sets read_at to now for the given message ID.
func (db *DB) MarkMessageRead(ctx context.Context, msgID string) error {
	now := time.Now().UTC().Unix()
	result := db.gormDB.WithContext(ctx).
		Model(&GMailboxMessage{}).
		Where("id = ?", msgID).
		Update("read_at", now)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrMessageNotFound
	}
	return nil
}

// MarkAllMessagesRead sets read_at to now for all unread messages of toAgent.
func (db *DB) MarkAllMessagesRead(ctx context.Context, toAgent string) error {
	now := time.Now().UTC().Unix()
	return db.gormDB.WithContext(ctx).
		Model(&GMailboxMessage{}).
		Where("to_agent = ? AND read_at IS NULL", toAgent).
		Update("read_at", now).Error
}

// GetMessageHistory returns up to limit messages for toAgent, newest first.
func (db *DB) GetMessageHistory(ctx context.Context, toAgent string, limit int) ([]GMailboxMessage, error) {
	var rows []GMailboxMessage
	q := db.gormDB.WithContext(ctx).
		Where("to_agent = ?", toAgent).
		Order("created_at DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	err := q.Find(&rows).Error
	return rows, err
}

// GetMessageThread returns all messages in a thread rooted at rootID,
// including the root itself, ordered oldest first.
func (db *DB) GetMessageThread(ctx context.Context, rootID string) ([]GMailboxMessage, error) {
	var rows []GMailboxMessage
	err := db.gormDB.WithContext(ctx).
		Where("id = ? OR reply_to = ?", rootID, rootID).
		Order("created_at ASC").
		Find(&rows).Error
	return rows, err
}

// GetTeamAgents returns the distinct to_agent values that have received
// messages tagged with teamID. Used for broadcast expansion.
func (db *DB) GetTeamAgents(ctx context.Context, teamID string) ([]string, error) {
	rows, err := db.SQL().QueryContext(ctx,
		`SELECT DISTINCT to_agent FROM mailbox_messages WHERE team_id = ? ORDER BY to_agent`,
		teamID,
	)
	if err != nil {
		return nil, fmt.Errorf("get team agents: %w", err)
	}
	defer rows.Close()
	var agents []string
	for rows.Next() {
		var agent string
		if err := rows.Scan(&agent); err != nil {
			return nil, fmt.Errorf("scan team agent: %w", err)
		}
		agents = append(agents, agent)
	}
	return agents, rows.Err()
}

// DeleteMessage removes a message record permanently.
func (db *DB) DeleteMessage(ctx context.Context, msgID string) error {
	result := db.gormDB.WithContext(ctx).Delete(&GMailboxMessage{}, "id = ?", msgID)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrMessageNotFound
	}
	return nil
}

// GetMessage returns a single message by ID.
func (db *DB) GetMessage(ctx context.Context, msgID string) (GMailboxMessage, error) {
	var row GMailboxMessage
	err := db.gormDB.WithContext(ctx).Where("id = ?", msgID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return GMailboxMessage{}, ErrMessageNotFound
	}
	return row, err
}
