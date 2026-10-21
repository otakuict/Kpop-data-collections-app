package gallery

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// The SQLite volume persists across Pod restarts. The administrator token is
// supplied separately by the deployment and acts as the encryption secret.
func sheetCredentialKey(adminToken string) [32]byte {
	return sha256.Sum256([]byte("bias-archive/sheet-credentials/v1\x00" + adminToken))
}

func sheetCredentialCipher(adminToken string) (cipher.AEAD, error) {
	key := sheetCredentialKey(adminToken)
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func (s *Store) SaveSheetCredentials(ctx context.Context, adminToken string, credentials []byte) error {
	aead, err := sheetCredentialCipher(adminToken)
	if err != nil {
		return err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return err
	}
	ciphertext := aead.Seal(nil, nonce, credentials, nil)
	_, err = s.db.ExecContext(ctx, `INSERT INTO sheet_credentials (id, nonce, ciphertext, updated_at)
		VALUES (1, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET nonce=excluded.nonce,
		ciphertext=excluded.ciphertext, updated_at=excluded.updated_at`, nonce, ciphertext, time.Now().UTC().Format(time.RFC3339))
	return err
}

func (s *Store) HasSheetCredentials(ctx context.Context) (bool, error) {
	var exists int
	err := s.db.QueryRowContext(ctx, `SELECT 1 FROM sheet_credentials WHERE id=1`).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return exists == 1, err
}

func (s *Store) LoadSheetCredentials(ctx context.Context, adminToken string) ([]byte, error) {
	var nonce, ciphertext []byte
	err := s.db.QueryRowContext(ctx, `SELECT nonce, ciphertext FROM sheet_credentials WHERE id=1`).Scan(&nonce, &ciphertext)
	if err != nil {
		return nil, err
	}
	aead, err := sheetCredentialCipher(adminToken)
	if err != nil {
		return nil, err
	}
	if len(nonce) != aead.NonceSize() {
		return nil, fmt.Errorf("stored Google Sheets credential is invalid")
	}
	data, err := aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("cannot decrypt stored Google Sheets credential; check ADMIN_TOKEN")
	}
	return data, nil
}
