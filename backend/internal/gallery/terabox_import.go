package gallery

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

type teraImportRow struct {
	FileID  int64   `json:"fileId"`
	Title   string  `json:"title"`
	Group   string  `json:"group"`
	SetDate string  `json:"setDate"`
	Source  *string `json:"source,omitempty"`
	Example string  `json:"example"`
	Notes   string  `json:"notes"`
}
type teraImportRequest struct {
	PreviewID string          `json:"previewId"`
	Rows      []teraImportRow `json:"rows"`
}
type teraImportResult struct {
	FileID int64  `json:"fileId"`
	SetID  SetID  `json:"setId"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

func (t *teraService) commit(ctx context.Context, sheet *SheetSync, active bool, request teraImportRequest) ([]teraImportResult, error) {
	if !active || sheet == nil {
		return nil, errors.New("Enable Google Sheet sync before confirming a TeraBox import")
	}
	if len(request.Rows) == 0 || len(request.Rows) > 200 {
		return nil, errors.New("Select between 1 and 200 preview rows")
	}
	var payload, revision, currentRevision, source string
	var expires int64
	var accountID int
	err := t.store.db.QueryRowContext(ctx, `SELECT p.payload,p.expires_at,p.account_id,p.account_revision,a.updated_at,a.source FROM terabox_previews p JOIN terabox_accounts a ON a.id=p.account_id WHERE p.id=?`, request.PreviewID).Scan(&payload, &expires, &accountID, &revision, &currentRevision, &source)
	if err != nil || expires < t.now().Unix() || revision != currentRevision {
		return nil, errors.New("Preview expired or the account settings changed; scan again")
	}
	var preview teraPreview
	if json.Unmarshal([]byte(payload), &preview) != nil {
		return nil, errors.New("Invalid saved preview")
	}
	allowed := map[int64]bool{}
	for _, row := range preview.Rows {
		allowed[row.FileID] = row.Status != "duplicate" && row.Status != "reconcile"
	}
	seen := map[int64]bool{}
	drafts := []SetDraft{}
	dates := []time.Time{}
	for _, row := range request.Rows {
		if !allowed[row.FileID] || seen[row.FileID] {
			return nil, errors.New("Selection contains a duplicate or a folder outside the reviewed preview")
		}
		seen[row.FileID] = true
		date, e := time.Parse("2006-01-02", row.SetDate)
		if e != nil || date.Year() < 2000 || date.Year() > 2099 {
			return nil, errors.New("Each row needs a valid photographed date")
		}
		title, date := buildSetTitle(date, row.Title)
		rowSource := source
		if row.Source != nil {
			rowSource = strings.TrimSpace(*row.Source)
		}
		draft := SetDraft{Title: title, Group: row.Group, Date: t.now().In(time.FixedZone("Asia/Bangkok", 7*60*60)).Format("2006-01-02"), Source: rowSource, Example: row.Example, Notes: row.Notes}
		if e = draft.Validate(); e != nil {
			return nil, e
		}
		var groupExists int
		if e = t.store.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM artist_groups WHERE name=?", draft.Group).Scan(&groupExists); e != nil {
			return nil, e
		}
		if groupExists != 1 {
			return nil, errors.New("Choose a group from the gallery catalog")
		}
		drafts = append(drafts, draft)
		dates = append(dates, date)
	}
	results := []teraImportResult{}
	for i, row := range request.Rows {
		draft := drafts[i]
		result := teraImportResult{FileID: row.FileID}
		var oldState string
		var oldID sql.NullInt64
		e := t.store.db.QueryRowContext(ctx, "SELECT state,set_id FROM terabox_imports WHERE account_id=? AND file_id=?", accountID, row.FileID).Scan(&oldState, &oldID)
		if e == nil {
			result.Status = oldState
			if oldState == "saved" {
				result.Status = "duplicate"
			}
			result.SetID = SetID(oldID.Int64)
			if oldState != "saved" {
				result.Error = "Prior write needs Sheet reconciliation; no retry was sent"
			}
			results = append(results, result)
			continue
		}
		if e != sql.ErrNoRows {
			return results, e
		}
		exists, e := t.store.teraTitleExists(ctx, draft.Title, draft.Group)
		if e != nil {
			return results, e
		}
		if exists {
			result.Status = "duplicate"
			results = append(results, result)
			continue
		}
		// Reserve before the external write. After a timeout/crash we cannot know if
		// Google applied it; a pending receipt blocks duplicate remote submissions.
		if _, e = t.store.db.ExecContext(ctx, `INSERT INTO terabox_imports(account_id,file_id,state,title,group_name) VALUES(?,?,'pending',?,?)`, accountID, row.FileID, draft.Title, draft.Group); e != nil {
			return results, e
		}
		synced, _, e := sheet.insertNew(ctx, t.store, draft, dates[i], true)
		if e == nil {
			var saved Set
			saved, e = t.store.SaveStyled(ctx, 0, synced, nil)
			result.SetID = saved.ID
		}
		if e != nil {
			result.Status = "reconcile"
			result.Error = "Save was interrupted. Import the Google Sheet to reconcile before retrying this folder."
			// Do not store upstream error bodies or credentials in receipts.
			_, _ = t.store.db.ExecContext(context.WithoutCancel(ctx), "UPDATE terabox_imports SET state='reconcile',error=? WHERE account_id=? AND file_id=?", result.Error, accountID, row.FileID)
			results = append(results, result)
			break
		}
		result.Status = "saved"
		if _, e = t.store.db.ExecContext(context.WithoutCancel(ctx), "UPDATE terabox_imports SET state='saved',set_id=?,error='' WHERE account_id=? AND file_id=?", result.SetID, accountID, row.FileID); e != nil {
			return results, errors.New("Set saved; receipt update failed. Scan and reconcile before retrying")
		}
		results = append(results, result)
	}
	return results, nil
}

func (s *Store) reconcileTeraImports(ctx context.Context) error {
	// Only repair receipts when the corresponding title and group really exist in
	// the local DB, e.g. after the normal Sheet import reconciles a partial write.
	_, err := s.db.ExecContext(ctx, `UPDATE terabox_imports SET state='saved',set_id=(SELECT id FROM sets WHERE lower(trim(title))=lower(trim(terabox_imports.title)) AND lower(trim(group_name))=lower(trim(terabox_imports.group_name)) LIMIT 1),error='' WHERE state<>'saved' AND EXISTS(SELECT 1 FROM sets WHERE lower(trim(title))=lower(trim(terabox_imports.title)) AND lower(trim(group_name))=lower(trim(terabox_imports.group_name)))`)
	return err
}

func validateTeraSessionInput(session *teraSession) {
	session.Host = strings.TrimSpace(session.Host)
	session.Cookie = strings.TrimSpace(session.Cookie)
	session.BDSToken = strings.TrimSpace(session.BDSToken)
	session.JSToken = strings.TrimSpace(session.JSToken)
}
