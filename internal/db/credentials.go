package db

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ─── GORM private model ───────────────────────────────────────────────────────

type gCredential struct {
	Key           string `gorm:"primaryKey;size:191"`
	CipherText    string `gorm:"column:cipher_text;not null"`
	CreatedAtUnix int64  `gorm:"column:created_at_unix;autoCreateTime:unix"`
	UpdatedAtUnix int64  `gorm:"column:updated_at_unix;autoUpdateTime:unix"`
}

func (gCredential) TableName() string { return "credentials" }

// ─── Public API (methods on *DB) ─────────────────────────────────────────────

// UpsertCredential encrypts plaintext and stores it under key.
func (db *DB) UpsertCredential(ctx context.Context, key, plaintext string) error {
	aesKey, err := loadOrCreateEncryptionKey()
	if err != nil {
		return err
	}
	cipherText, err := encryptAESGCM(aesKey, []byte(plaintext))
	if err != nil {
		return err
	}
	row := gCredential{Key: key, CipherText: cipherText}
	return db.gormDB.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "key"}},
			DoUpdates: clause.AssignmentColumns([]string{"cipher_text", "updated_at_unix"}),
		}).Create(&row).Error
}

// GetCredential retrieves and decrypts the value stored under key.
// Returns ("", false, nil) when the key does not exist.
func (db *DB) GetCredential(ctx context.Context, key string) (string, bool, error) {
	var row gCredential
	err := db.gormDB.WithContext(ctx).Where("key = ?", key).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", false, nil
		}
		return "", false, err
	}
	aesKey, err := loadOrCreateEncryptionKey()
	if err != nil {
		return "", false, err
	}
	plain, err := decryptAESGCM(aesKey, row.CipherText)
	if err != nil {
		return "", false, err
	}
	return string(plain), true, nil
}

// DeleteCredential removes the credential for key. No-op if it does not exist.
func (db *DB) DeleteCredential(ctx context.Context, key string) error {
	return db.gormDB.WithContext(ctx).Delete(&gCredential{}, "key = ?", key).Error
}

// ListCredentialKeys returns the stored keys without their values.
func (db *DB) ListCredentialKeys(ctx context.Context) ([]string, error) {
	var rows []gCredential
	if err := db.gormDB.WithContext(ctx).Order("key").Find(&rows).Error; err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(rows))
	for _, r := range rows {
		keys = append(keys, r.Key)
	}
	return keys, nil
}

// ─── Encryption helpers ────────────────────────────────────────────────────────

const keyFile = ".nexus_secret"

// loadOrCreateEncryptionKey returns the 32-byte AES key from ~/.nexus_secret,
// creating and writing it with mode 0600 on first use.
func loadOrCreateEncryptionKey() ([]byte, error) {
	path, err := encryptionKeyPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err == nil && len(data) == 32 {
		return data, nil
	}
	// Generate a new key.
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("generate encryption key: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create key directory: %w", err)
	}
	if err := os.WriteFile(path, key, 0o600); err != nil {
		return nil, fmt.Errorf("write encryption key: %w", err)
	}
	return key, nil
}

func encryptionKeyPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, keyFile), nil
}

// encryptAESGCM encrypts plaintext with AES-256-GCM and returns a base64 string
// of the form: base64(nonce || ciphertext).
func encryptAESGCM(key, plaintext []byte) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("create AES cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create GCM: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}
	sealed := gcm.Seal(nonce, nonce, plaintext, nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// decryptAESGCM reverses encryptAESGCM.
func decryptAESGCM(key []byte, encoded string) ([]byte, error) {
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("decode credential: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create AES cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create GCM: %w", err)
	}
	ns := gcm.NonceSize()
	if len(data) < ns {
		return nil, fmt.Errorf("credential data too short")
	}
	plain, err := gcm.Open(nil, data[:ns], data[ns:], nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt credential: %w", err)
	}
	return plain, nil
}
