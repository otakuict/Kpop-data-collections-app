package gallery

import (
	"context"
	"errors"
	"strings"
)

// Explicit operator recovery checks the live Sheet under its write lock. It
// never replays a failed insertion: present rows are reconciled into SQLite;
// absent rows are released for a fresh preview and a separate confirmation.
func (t *teraService) recover(ctx context.Context, sheet *SheetSync, active bool, accountID int) ([]teraImportResult, error) {
	if !active || sheet == nil {
		return nil, errors.New("Enable Google Sheet sync before recovery")
	}
	sheet.writeMu.Lock()
	defer sheet.writeMu.Unlock()
	title, err := sheet.sheetTitle(ctx)
	if err != nil {
		return nil, err
	}
	cells, err := sheet.rows(ctx, title)
	if err != nil {
		return nil, err
	}
	live, err := parseLiveRows(cells, sheet.source)
	if err != nil {
		return nil, errors.New("Cannot parse the live Sheet; pending receipts were retained")
	}
	pending, err := t.store.db.QueryContext(ctx, "SELECT file_id,title,group_name FROM terabox_imports WHERE account_id=? AND state<>'saved'", accountID)
	if err != nil {
		return nil, err
	}
	type receipt struct {
		id           int64
		title, group string
	}
	receipts := []receipt{}
	for pending.Next() {
		var r receipt
		if err = pending.Scan(&r.id, &r.title, &r.group); err != nil {
			break
		}
		receipts = append(receipts, r)
	}
	if err == nil {
		err = pending.Err()
	}
	pending.Close()
	if err != nil {
		return nil, err
	}
	results := []teraImportResult{}
	for _, r := range receipts {
		matches := []SetDraft{}
		for _, row := range live {
			if strings.EqualFold(strings.TrimSpace(row.Draft.Title), strings.TrimSpace(r.title)) && strings.EqualFold(strings.TrimSpace(row.Draft.Group), strings.TrimSpace(r.group)) {
				matches = append(matches, row.Draft)
			}
		}
		result := teraImportResult{FileID: r.id}
		if len(matches) > 1 {
			result.Status = "reconcile"
			result.Error = "Multiple matching Sheet rows; review and import the Sheet manually"
			results = append(results, result)
			continue
		}
		if len(matches) == 0 {
			_, err = t.store.db.ExecContext(ctx, "DELETE FROM terabox_imports WHERE account_id=? AND file_id=? AND state<>'saved'", accountID, r.id)
			result.Status = "retry"
		} else {
			_, err = t.store.Import(ctx, matches)
			if err == nil {
				err = t.store.db.QueryRowContext(ctx, "SELECT id FROM sets WHERE import_key=?", matches[0].ImportKey).Scan(&result.SetID)
			}
			if err == nil {
				_, err = t.store.db.ExecContext(ctx, "UPDATE terabox_imports SET state='saved',set_id=?,error='' WHERE account_id=? AND file_id=?", result.SetID, accountID, r.id)
			}
			result.Status = "saved"
		}
		if err != nil {
			return results, err
		}
		results = append(results, result)
	}
	return results, nil
}
