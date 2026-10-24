package gallery

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

var ErrAlbumFull = errors.New("a set can contain at most 5 distinct images")
var ErrAlbumIncomplete = errors.New("a set needs at least 2 distinct images; upload more images together")

var ErrSourceChanged = errors.New("example URL changed during extraction; retry with the current example")

type Store struct{ db *sql.DB }

const newRibbonLimit = 10

// expireOldNewRibbons keeps the New flag only on the ten most recently
// inserted sets. It also clears flags left by older versions of the app.
func expireOldNewRibbons(ctx context.Context, tx *sql.Tx) error {
	var cutoff SetID
	err := tx.QueryRowContext(ctx, "SELECT id FROM sets ORDER BY id DESC LIMIT 1 OFFSET ?", newRibbonLimit-1).Scan(&cutoff)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, "SELECT id,style FROM sets WHERE id<?", cutoff)
	if err != nil {
		return err
	}
	type expiredStyle struct {
		id    SetID
		style string
	}
	updates := []expiredStyle{}
	for rows.Next() {
		var id SetID
		var encoded string
		if err = rows.Scan(&id, &encoded); err != nil {
			break
		}
		var style map[string]json.RawMessage
		if err = json.Unmarshal([]byte(encoded), &style); err != nil {
			break
		}
		var isNew bool
		if raw := style["new"]; len(raw) != 0 {
			if err = json.Unmarshal(raw, &isNew); err != nil {
				break
			}
		}
		if !isNew {
			continue
		}
		style["new"] = json.RawMessage("false")
		var updated []byte
		updated, err = json.Marshal(style)
		if err != nil {
			break
		}
		updates = append(updates, expiredStyle{id, string(updated)})
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return err
	}
	for _, update := range updates {
		if _, err := tx.ExecContext(ctx, "UPDATE sets SET style=? WHERE id=?", update.style, update.id); err != nil {
			return err
		}
	}
	return nil
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite3", path+"?_foreign_keys=on&_busy_timeout=5000&_journal_mode=WAL")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS sets (
 id INTEGER PRIMARY KEY, title TEXT NOT NULL, group_name TEXT NOT NULL, date TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT '', example TEXT NOT NULL DEFAULT '', notes TEXT NOT NULL DEFAULT '', raw_date TEXT NOT NULL DEFAULT '', raw_cells TEXT NOT NULL DEFAULT '{}', import_key TEXT UNIQUE, import_warning TEXT NOT NULL DEFAULT '', origin TEXT NOT NULL DEFAULT 'manual', image_state TEXT NOT NULL DEFAULT 'pending', image_error TEXT NOT NULL DEFAULT '', style TEXT NOT NULL DEFAULT '{}');
 CREATE INDEX IF NOT EXISTS sets_group_date ON sets(group_name,date DESC,id);
 CREATE INDEX IF NOT EXISTS sets_date ON sets(date DESC,id);
 CREATE TABLE IF NOT EXISTS images (id INTEGER PRIMARY KEY, set_id INTEGER NOT NULL REFERENCES sets(id) ON DELETE CASCADE, mime TEXT NOT NULL, bytes BLOB NOT NULL, hash TEXT NOT NULL, source_url TEXT NOT NULL DEFAULT '', UNIQUE(set_id,hash));
 CREATE INDEX IF NOT EXISTS images_set ON images(set_id,id);
 CREATE TABLE IF NOT EXISTS remote_limits (host TEXT PRIMARY KEY, next_at INTEGER NOT NULL DEFAULT 0, paused_until INTEGER NOT NULL DEFAULT 0, reason TEXT NOT NULL DEFAULT '');
 CREATE TABLE IF NOT EXISTS remote_leases (host TEXT PRIMARY KEY, until INTEGER NOT NULL);
 CREATE TABLE IF NOT EXISTS page_cache (url TEXT PRIMARY KEY, bytes BLOB NOT NULL, mime TEXT NOT NULL, fetched_at INTEGER NOT NULL);
 CREATE TABLE IF NOT EXISTS sheet_credentials (id INTEGER PRIMARY KEY CHECK(id=1), nonce BLOB NOT NULL, ciphertext BLOB NOT NULL, updated_at TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS usage_visits (id INTEGER PRIMARY KEY, visitor_id TEXT NOT NULL, country TEXT NOT NULL DEFAULT '', started_at INTEGER NOT NULL, last_seen INTEGER NOT NULL);
 CREATE INDEX IF NOT EXISTS usage_visits_visitor ON usage_visits(visitor_id, last_seen DESC);
 CREATE INDEX IF NOT EXISTS usage_visits_seen ON usage_visits(last_seen);
 `)
	if err != nil {
		db.Close()
		return nil, err
	}
	// Existing databases may retain a pause imposed by the former local daily budget.
	if _, err = db.Exec(`UPDATE remote_limits SET paused_until=0,reason='' WHERE host='ganknow.com' AND reason LIKE 'local daily request budget reached (%'`); err != nil {
		db.Close()
		return nil, err
	}
	var styleColumn int
	rows, err := db.Query(`PRAGMA table_info(sets)`)
	if err != nil {
		db.Close()
		return nil, err
	}
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, kind string
		var defaultValue sql.NullString
		if err = rows.Scan(&cid, &name, &kind, &notNull, &defaultValue, &primaryKey); err != nil {
			break
		}
		if name == "style" {
			styleColumn++
		}
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err == nil && styleColumn == 0 {
		_, err = db.Exec(`ALTER TABLE sets ADD COLUMN style TEXT NOT NULL DEFAULT '{}'`)
	}
	if err == nil {
		var version int
		err = db.QueryRow(`PRAGMA user_version`).Scan(&version)
		if err == nil && version < 3 {
			_, err = db.Exec(`PRAGMA user_version=3`)
		}
	}
	if err == nil {
		var tx *sql.Tx
		tx, err = db.Begin()
		if err == nil {
			err = expireOldNewRibbons(context.Background(), tx)
			if err == nil {
				err = tx.Commit()
			} else {
				_ = tx.Rollback()
			}
		}
	}
	if err != nil {
		db.Close()
		return nil, err
	}
	store := &Store{db: db}
	if err = store.initTeraBox(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	if err = store.seedArtistCatalog(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}
func (s *Store) Close() error { return s.db.Close() }

func (s *Store) Import(ctx context.Context, drafts []SetDraft) (ImportReport, error) {
	return s.ImportWithImagePolicy(ctx, drafts, KeepExistingImages)
}

func (s *Store) PreviewImport(ctx context.Context, drafts []SetDraft) (ImportReport, error) {
	return s.PreviewImportWithImagePolicy(ctx, drafts, KeepExistingImages)
}

type ImageImportPolicy string

const (
	KeepExistingImages   ImageImportPolicy = "keep"
	ReplaceChangedImages ImageImportPolicy = "replace"
)

func (p ImageImportPolicy) valid() bool { return p == KeepExistingImages || p == ReplaceChangedImages }

func (s *Store) ImportWithImagePolicy(ctx context.Context, drafts []SetDraft, policy ImageImportPolicy) (ImportReport, error) {
	return s.importSheet(ctx, drafts, false, policy)
}

func (s *Store) PreviewImportWithImagePolicy(ctx context.Context, drafts []SetDraft, policy ImageImportPolicy) (ImportReport, error) {
	return s.importSheet(ctx, drafts, true, policy)
}

func (s *Store) importSheet(ctx context.Context, drafts []SetDraft, preview bool, policy ImageImportPolicy) (ImportReport, error) {
	report := ImportReport{Total: len(drafts), Candidates: []SetID{}, ScanCandidates: []SetID{}, Changes: []ImportChange{}}
	if !policy.valid() {
		return report, fmt.Errorf("image policy must be keep or replace")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return report, err
	}
	defer tx.Rollback()
	for _, d := range drafts {
		if d.ImportKey == "" {
			return report, fmt.Errorf("import row missing identity")
		}
		old, lookupErr := scanSet(tx.QueryRowContext(ctx, "SELECT "+setColumns+" FROM sets WHERE import_key=?", d.ImportKey))
		err = lookupErr
		if err != nil && err != sql.ErrNoRows {
			return report, err
		}
		id := old.ID
		isNew := err == sql.ErrNoRows
		oldImageCount := 0
		if isNew {
			result, e := tx.ExecContext(ctx, `INSERT INTO sets(title,group_name,date,source,example,notes,raw_date,raw_cells,import_key,import_warning,origin,style) VALUES(?,?,?,?,?,?,?,?,?,?,'sheet',?)`, d.Title, d.Group, d.Date, d.Source, d.Example, d.Notes, d.RawDate, d.RawCells, d.ImportKey, d.ImportWarning, `{"new":true}`)
			if e != nil {
				return report, e
			}
			n, e := result.LastInsertId()
			if e != nil {
				return report, e
			}
			id = SetID(n)
			report.Created++
			report.Changes = append(report.Changes, ImportChange{ID: id, Kind: "new", After: d})
		} else {
			old.ImportKey = d.ImportKey
			_, err = tx.ExecContext(ctx, `UPDATE sets SET title=?,group_name=?,date=?,source=?,example=?,notes=?,raw_date=?,raw_cells=?,import_warning=? WHERE id=?`, d.Title, d.Group, d.Date, d.Source, d.Example, d.Notes, d.RawDate, d.RawCells, d.ImportWarning, id)
			if err != nil {
				return report, err
			}
			report.Updated++
			if old.SetDraft != d {
				report.Changed++
				report.Changes = append(report.Changes, ImportChange{ID: id, Kind: "updated", Before: &old.SetDraft, After: d})
			} else {
				report.Unchanged++
			}
			if old.Example != d.Example {
				if err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM images WHERE set_id=?", id).Scan(&oldImageCount); err != nil {
					return report, err
				}
				if policy == ReplaceChangedImages {
					if _, err = tx.ExecContext(ctx, "DELETE FROM images WHERE set_id=?", id); err != nil {
						return report, err
					}
					if _, err = tx.ExecContext(ctx, "UPDATE sets SET image_state='pending',image_error='' WHERE id=?", id); err != nil {
						return report, err
					}
				}
			}
		}
		if (isNew || (old.Example != d.Example && (policy == ReplaceChangedImages || oldImageCount == 0))) && strings.HasPrefix(d.Example, "https://") {
			report.ScanCandidates = append(report.ScanCandidates, id)
		}
		var count int
		if err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM images WHERE set_id=?", id).Scan(&count); err != nil {
			return report, err
		}
		if count < MinSetImages {
			report.Candidates = append(report.Candidates, id)
		}
	}
	if preview {
		return report, nil
	}
	if err = expireOldNewRibbons(ctx, tx); err != nil {
		return report, err
	}
	return report, tx.Commit()
}

const setColumns = `id,title,group_name,date,source,example,notes,raw_date,raw_cells,import_warning,origin,image_state,image_error,style`

func scanSet(row interface{ Scan(...any) error }) (Set, error) {
	var x Set
	var styleJSON string
	err := row.Scan(&x.ID, &x.Title, &x.Group, &x.Date, &x.Source, &x.Example, &x.Notes, &x.RawDate, &x.RawCells, &x.ImportWarning, &x.Origin, &x.ImageState, &x.ImageError, &styleJSON)
	if err == nil {
		err = json.Unmarshal([]byte(styleJSON), &x.Style)
	}
	x.Images = []Image{}
	return x, err
}
func (s *Store) attach(ctx context.Context, x *Set) error {
	rows, err := s.db.QueryContext(ctx, "SELECT id,source_url,hash FROM images WHERE set_id=? ORDER BY id", x.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var img Image
		var hash string
		if err := rows.Scan(&img.ID, &img.SourceURL, &hash); err != nil {
			return err
		}
		img.URL = fmt.Sprintf("/api/images/%d?v=%s", img.ID, hash[:12])
		x.Images = append(x.Images, img)
	}
	return rows.Err()
}
func (s *Store) Get(ctx context.Context, id SetID) (Set, error) {
	x, err := scanSet(s.db.QueryRowContext(ctx, "SELECT "+setColumns+" FROM sets WHERE id=?", id))
	if err != nil {
		return x, err
	}
	err = s.attach(ctx, &x)
	return x, err
}

func (s *Store) Delete(ctx context.Context, id SetID) error {
	result, err := s.db.ExecContext(ctx, "DELETE FROM sets WHERE id=?", id)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) List(ctx context.Context, f Filter) (Page, error) {
	where := []string{"1=1"}
	args := []any{}
	if f.Query != "" {
		where = append(where, `(title LIKE ? ESCAPE '\' OR group_name LIKE ? ESCAPE '\' OR source LIKE ? ESCAPE '\')`)
		q := "%" + strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(f.Query) + "%"
		args = append(args, q, q, q)
	}
	if f.Group != "" {
		where = append(where, "group_name=?")
		args = append(args, f.Group)
	}
	if f.Ribbon != "" {
		where = append(where, "json_extract(style,?)=1")
		args = append(args, "$."+f.Ribbon)
	}
	if f.From != "" {
		where = append(where, "date>=?")
		args = append(args, f.From)
	}
	if f.To != "" {
		where = append(where, "date<>'' AND date<=?")
		args = append(args, f.To)
	}
	clause := strings.Join(where, " AND ")
	p := Page{Items: []Set{}, Page: f.Page, Limit: f.Limit}
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sets WHERE "+clause, args...).Scan(&p.Total); err != nil {
		return p, err
	}
	order := "(date=''),date DESC,id DESC"
	if f.Sort == "oldest" {
		order = "(date=''),date ASC,id ASC"
	}
	if f.Sort == "name" {
		order = "title COLLATE NOCASE,id"
	}
	rows, err := s.db.QueryContext(ctx, "SELECT "+setColumns+" FROM sets WHERE "+clause+" ORDER BY "+order+" LIMIT ? OFFSET ?", append(args, f.Limit, (f.Page-1)*f.Limit)...)
	if err != nil {
		return p, err
	}
	for rows.Next() {
		x, e := scanSet(rows)
		if e != nil {
			rows.Close()
			return p, e
		}
		p.Items = append(p.Items, x)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return p, err
	}
	for i := range p.Items {
		if err := s.attach(ctx, &p.Items[i]); err != nil {
			return p, err
		}
	}
	return p, nil
}
func (s *Store) Facets(ctx context.Context) (Facets, error) {
	f := Facets{Groups: []Facet{}, Sources: []Facet{}}
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*),COALESCE(SUM(EXISTS(SELECT 1 FROM images WHERE set_id=sets.id)),0),COALESCE(SUM((SELECT COUNT(*) FROM images WHERE set_id=sets.id) BETWEEN 2 AND 5),0),COALESCE(MIN(NULLIF(date,'')),''),COALESCE(MAX(date),'') FROM sets").Scan(&f.Total, &f.Ready, &f.Complete, &f.From, &f.To)
	if err != nil {
		return f, err
	}
	rows, err := s.db.QueryContext(ctx, "SELECT group_name,COUNT(*) FROM sets GROUP BY group_name ORDER BY group_name COLLATE NOCASE")
	if err != nil {
		return f, err
	}
	defer rows.Close()
	for rows.Next() {
		var x Facet
		if err := rows.Scan(&x.Name, &x.Count); err != nil {
			return f, err
		}
		f.Groups = append(f.Groups, x)
	}
	if err := rows.Err(); err != nil {
		return f, err
	}
	if err := rows.Close(); err != nil {
		return f, err
	}
	sources, err := s.db.QueryContext(ctx, "SELECT source,COUNT(*) FROM sets WHERE TRIM(source)<>'' GROUP BY source ORDER BY source COLLATE NOCASE")
	if err != nil {
		return f, err
	}
	defer sources.Close()
	for sources.Next() {
		var x Facet
		if err := sources.Scan(&x.Name, &x.Count); err != nil {
			return f, err
		}
		f.Sources = append(f.Sources, x)
	}
	return f, sources.Err()
}
func (s *Store) Save(ctx context.Context, id SetID, d SetDraft) (Set, error) {
	return s.SaveStyled(ctx, id, d, nil)
}

func (s *Store) ImportKey(ctx context.Context, id SetID) (string, error) {
	var key sql.NullString
	err := s.db.QueryRowContext(ctx, "SELECT import_key FROM sets WHERE id=?", id).Scan(&key)
	return key.String, err
}

func (s *Store) CheckImportKey(ctx context.Context, id SetID, key string) error {
	var other SetID
	err := s.db.QueryRowContext(ctx, "SELECT id FROM sets WHERE import_key=? AND id<>?", key, id).Scan(&other)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	return fmt.Errorf("sheet row identity would overlap set #%d", other)
}

func nullableImportKey(key string) any {
	if key == "" {
		return nil
	}
	return key
}

func (s *Store) SaveStyled(ctx context.Context, id SetID, d SetDraft, style *SetStyle) (Set, error) {
	if err := d.Validate(); err != nil {
		return Set{}, err
	}
	var styleJSON any
	if style != nil {
		if err := style.Validate(); err != nil {
			return Set{}, err
		}
		encoded, err := json.Marshal(style)
		if err != nil {
			return Set{}, err
		}
		styleJSON = string(encoded)
	} else if id == 0 {
		styleJSON = `{"new":true}`
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Set{}, err
	}
	defer tx.Rollback()
	if id == 0 {
		origin := "manual"
		if d.ImportKey != "" {
			origin = "sheet"
		}
		r, err := tx.ExecContext(ctx, `INSERT INTO sets(title,group_name,date,source,example,notes,raw_date,raw_cells,import_key,import_warning,origin,style) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, d.Title, d.Group, d.Date, d.Source, d.Example, d.Notes, d.RawDate, d.RawCells, nullableImportKey(d.ImportKey), d.ImportWarning, origin, styleJSON)
		if err != nil {
			return Set{}, err
		}
		n, err := r.LastInsertId()
		if err != nil {
			return Set{}, err
		}
		id = SetID(n)
	} else {
		var oldExample string
		if err = tx.QueryRowContext(ctx, "SELECT example FROM sets WHERE id=?", id).Scan(&oldExample); err != nil {
			return Set{}, err
		}
		synced := d.ImportKey != ""
		_, err = tx.ExecContext(ctx, `UPDATE sets SET title=?,group_name=?,date=?,source=?,example=?,notes=?,style=COALESCE(?,style),
			raw_date=CASE WHEN ? THEN ? ELSE raw_date END,
			raw_cells=CASE WHEN ? THEN ? ELSE raw_cells END,
			import_key=CASE WHEN ? THEN ? ELSE import_key END,
			import_warning=CASE WHEN ? THEN ? ELSE import_warning END WHERE id=?`,
			d.Title, d.Group, d.Date, d.Source, d.Example, d.Notes, styleJSON,
			synced, d.RawDate, synced, d.RawCells, synced, d.ImportKey, synced, d.ImportWarning, id)
		if err != nil {
			return Set{}, err
		}
		if oldExample != d.Example {
			if _, err = tx.ExecContext(ctx, "DELETE FROM images WHERE set_id=?", id); err != nil {
				return Set{}, err
			}
			if _, err = tx.ExecContext(ctx, "UPDATE sets SET image_state='pending',image_error='' WHERE id=?", id); err != nil {
				return Set{}, err
			}
		}
	}
	if err = expireOldNewRibbons(ctx, tx); err != nil {
		return Set{}, err
	}
	if err = tx.Commit(); err != nil {
		return Set{}, err
	}
	return s.Get(ctx, id)
}
func (s *Store) AddImage(ctx context.Context, id SetID, r Raster) error {
	return s.addImages(ctx, id, []Raster{r}, false)
}
func (s *Store) AddImages(ctx context.Context, id SetID, rasters []Raster) error {
	return s.addImages(ctx, id, rasters, true)
}
func (s *Store) addImages(ctx context.Context, id SetID, rasters []Raster, requireComplete bool) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var example string
	if err = tx.QueryRowContext(ctx, "SELECT example FROM sets WHERE id=?", id).Scan(&example); err != nil {
		return err
	}
	hashes := map[string]bool{}
	rows, err := tx.QueryContext(ctx, "SELECT hash FROM images WHERE set_id=?", id)
	if err != nil {
		return err
	}
	for rows.Next() {
		var hash string
		if err = rows.Scan(&hash); err != nil {
			rows.Close()
			return err
		}
		hashes[hash] = true
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, r := range rasters {
		if r.expectedExample != nil && example != *r.expectedExample {
			return ErrSourceChanged
		}
		sum := sha256.Sum256(r.Bytes)
		hash := hex.EncodeToString(sum[:])
		if hashes[hash] {
			continue
		}
		if len(hashes) >= MaxSetImages {
			return ErrAlbumFull
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO images(set_id,mime,bytes,hash,source_url) VALUES(?,?,?,?,?)", id, r.MIME, r.Bytes, hash, r.SourceURL); err != nil {
			return err
		}
		hashes[hash] = true
	}
	if requireComplete && len(hashes) < MinSetImages {
		return ErrAlbumIncomplete
	}
	if _, err = tx.ExecContext(ctx, "UPDATE sets SET image_state='ready',image_error='' WHERE id=?", id); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) Outcome(ctx context.Context, id SetID, expectedExample, state, message string) error {
	_, err := s.db.ExecContext(ctx, "UPDATE sets SET image_state=CASE WHEN EXISTS(SELECT 1 FROM images WHERE set_id=sets.id) THEN 'ready' ELSE ? END,image_error=? WHERE id=? AND example=?", state, message, id, expectedExample)
	return err
}
func (s *Store) ImageBytes(ctx context.Context, id ImageID) ([]byte, string, error) {
	var b []byte
	var mime string
	err := s.db.QueryRowContext(ctx, "SELECT bytes,mime FROM images WHERE id=?", id).Scan(&b, &mime)
	return b, mime, err
}

func (s *Store) Backup(ctx context.Context, path string) error {
	_, err := s.db.ExecContext(ctx, "VACUUM INTO ?", path)
	return err
}
