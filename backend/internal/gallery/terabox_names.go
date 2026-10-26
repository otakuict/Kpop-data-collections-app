package gallery

import (
	"context"
	"errors"
	"time"
)

type teraNameEntry struct {
	ModifiedAt string `json:"modifiedAt"`
	FileID     int64  `json:"fileId"`
	Name       string `json:"name"`
	Path       string `json:"path"`
	IsDir      bool   `json:"isDir"`
}

type teraNames struct {
	AccountID int             `json:"accountId"`
	Root      string          `json:"root"`
	Days      int             `json:"days"`
	Entries   []teraNameEntry `json:"entries"`
	Scanned   int             `json:"scanned"`
	Complete  bool            `json:"complete"`
	Warnings  []string        `json:"warnings"`
}

type teraRootListing struct {
	root     string
	revision string
	files    []teraFile
	scanned  int
	complete bool
	warnings []string
}

func (t *teraService) rootNames(ctx context.Context, accountID, days int) (teraRootListing, error) {
	result := teraRootListing{files: []teraFile{}, warnings: []string{}}
	if accountID < 1 || accountID > 2 {
		return result, errors.New("Choose account 1 or 2")
	}
	if days < 0 || days > 10 {
		return result, errors.New("Choose all dates or a lookback period from 1 to 10 days")
	}
	if err := t.store.db.QueryRowContext(ctx, "SELECT root,updated_at FROM terabox_accounts WHERE id=?", accountID).Scan(&result.root, &result.revision); err != nil {
		return result, errors.New("Connect the selected TeraBox account first")
	}
	session, err := t.store.teraSession(ctx, t.key, accountID)
	if err != nil {
		return result, err
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	files, complete, err := t.provider.listRoot(ctx, session, result.root, 100)
	if err != nil {
		return result, err
	}
	result.complete = complete
	result.scanned = len(files)
	if !complete {
		result.warnings = append(result.warnings, "Request limit reached. Results are partial; narrow the account root folder for full coverage.")
	}
	recent, warnings := recentTeraEntries(files, t.now(), days)
	result.files = recent
	result.warnings = append(result.warnings, warnings...)
	return result, nil
}

func (t *teraService) names(ctx context.Context, accountID, days int) (teraNames, error) {
	result := teraNames{AccountID: accountID, Days: days, Entries: []teraNameEntry{}, Warnings: []string{}}
	listing, err := t.rootNames(ctx, accountID, days)
	if err != nil {
		return result, err
	}
	result.Root = listing.root
	result.Scanned = listing.scanned
	result.Complete = listing.complete
	result.Warnings = listing.warnings
	for _, file := range listing.files {
		entry := teraNameEntry{FileID: file.ID, Name: file.Name, Path: file.Path, IsDir: file.IsDir == 1}
		if file.Modified > 0 {
			entry.ModifiedAt = time.Unix(file.Modified, 0).UTC().Format(time.RFC3339)
		}
		result.Entries = append(result.Entries, entry)
	}
	return result, nil
}
