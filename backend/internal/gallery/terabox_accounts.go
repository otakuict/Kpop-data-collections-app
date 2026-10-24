package gallery

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"
)

type teraAccount struct {
	ID         int    `json:"id"`
	Label      string `json:"label"`
	Root       string `json:"root"`
	Source     string `json:"source"`
	Configured bool   `json:"configured"`
	Host       string `json:"host"`
	// Input only; never populated in account responses.
	Cookie string `json:"-"`
}
type teraSession struct {
	Host     string `json:"host"`
	Cookie   string `json:"cookie"`
	BDSToken string `json:"bdstoken"`
	JSToken  string `json:"jsToken"`
}

func (s *Store) initTeraBox(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
 CREATE TABLE IF NOT EXISTS terabox_accounts(id INTEGER PRIMARY KEY CHECK(id IN (1,2)),label TEXT NOT NULL,root TEXT NOT NULL,source TEXT NOT NULL DEFAULT '',nonce BLOB NOT NULL,ciphertext BLOB NOT NULL,updated_at TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS artist_groups(name TEXT PRIMARY KEY,kind TEXT NOT NULL DEFAULT 'group');
 CREATE TABLE IF NOT EXISTS artist_members(id INTEGER PRIMARY KEY,group_name TEXT NOT NULL REFERENCES artist_groups(name),name TEXT NOT NULL,aliases TEXT NOT NULL DEFAULT '[]',source TEXT NOT NULL DEFAULT '',UNIQUE(group_name,name));
 CREATE TABLE IF NOT EXISTS terabox_previews(id TEXT PRIMARY KEY,account_id INTEGER NOT NULL,account_revision TEXT NOT NULL,expires_at INTEGER NOT NULL,payload TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS terabox_imports(account_id INTEGER NOT NULL,file_id INTEGER NOT NULL,state TEXT NOT NULL,set_id INTEGER,title TEXT NOT NULL,group_name TEXT NOT NULL,error TEXT NOT NULL DEFAULT '',PRIMARY KEY(account_id,file_id));
 `)
	return err
}

func (s *Store) saveTeraAccount(ctx context.Context, key string, a teraAccount, session teraSession) error {
	if _, err := teraBrowserOrigin(session.Host); err != nil {
		return err
	}
	if a.ID < 1 || a.ID > 2 || len(strings.TrimSpace(a.Label)) == 0 || len(a.Label) > 80 || len(a.Source) > 300 || len(a.Root) > 1500 || !strings.HasPrefix(a.Root, "/") || strings.ContainsAny(a.Root, "\x00\r\n") {
		return errors.New("Choose account 1 or 2, a label, and an absolute root folder path")
	}
	if !strings.Contains(session.Cookie, "ndus=") || len(session.Cookie) > 16000 || strings.ContainsAny(session.Cookie, "\r\n\x00") || len(session.BDSToken) > 2000 || len(session.JSToken) > 4000 {
		return errors.New("Provide a valid TeraBox session Cookie containing ndus")
	}
	aead, err := sheetCredentialCipher("terabox/accounts/v1\x00" + key)
	if err != nil {
		return err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return err
	}
	raw, err := json.Marshal(session)
	if err != nil {
		return err
	}
	encrypted := aead.Seal(nil, nonce, raw, []byte(fmt.Sprint(a.ID)))
	_, err = s.db.ExecContext(ctx, `INSERT INTO terabox_accounts(id,label,root,source,nonce,ciphertext,updated_at) VALUES(?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET label=excluded.label,root=excluded.root,source=excluded.source,nonce=excluded.nonce,ciphertext=excluded.ciphertext,updated_at=excluded.updated_at`, a.ID, strings.TrimSpace(a.Label), path.Clean(a.Root), a.Source, nonce, encrypted, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

func (s *Store) teraAccounts(ctx context.Context) ([]teraAccount, error) {
	result := []teraAccount{}
	rows, err := s.db.QueryContext(ctx, "SELECT id,label,root,source FROM terabox_accounts ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var a teraAccount
		if err = rows.Scan(&a.ID, &a.Label, &a.Root, &a.Source); err != nil {
			return nil, err
		}
		a.Configured = true
		result = append(result, a)
	}
	return result, rows.Err()
}

func (s *Store) teraSession(ctx context.Context, key string, id int) (teraSession, error) {
	var nonce, encrypted []byte
	var session teraSession
	if err := s.db.QueryRowContext(ctx, "SELECT nonce,ciphertext FROM terabox_accounts WHERE id=?", id).Scan(&nonce, &encrypted); err != nil {
		return session, err
	}
	aead, err := sheetCredentialCipher("terabox/accounts/v1\x00" + key)
	if err != nil {
		return session, err
	}
	if len(nonce) != aead.NonceSize() {
		return session, errors.New("Invalid stored TeraBox session")
	}
	raw, err := aead.Open(nil, nonce, encrypted, []byte(fmt.Sprint(id)))
	if err != nil {
		return session, errors.New("Cannot decrypt TeraBox session; reconnect this account after changing ADMIN_TOKEN")
	}
	err = json.Unmarshal(raw, &session)
	return session, err
}
