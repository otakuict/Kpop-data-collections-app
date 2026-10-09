package gallery

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

var ErrAlbumFull = errors.New("a set can contain at most 5 distinct images")
var ErrAlbumIncomplete = errors.New("a set needs at least 2 distinct images; upload more images together")

var ErrSourceChanged = errors.New("example URL changed during extraction; retry with the current example")

type Store struct{ db *sql.DB }

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite3", path+"?_foreign_keys=on&_busy_timeout=5000&_journal_mode=WAL")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS sets (
 id INTEGER PRIMARY KEY, title TEXT NOT NULL, group_name TEXT NOT NULL, date TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT '', example TEXT NOT NULL DEFAULT '', notes TEXT NOT NULL DEFAULT '', raw_date TEXT NOT NULL DEFAULT '', raw_cells TEXT NOT NULL DEFAULT '{}', import_key TEXT UNIQUE, import_warning TEXT NOT NULL DEFAULT '', origin TEXT NOT NULL DEFAULT 'manual', image_state TEXT NOT NULL DEFAULT 'pending', image_error TEXT NOT NULL DEFAULT '');
 CREATE INDEX IF NOT EXISTS sets_group_date ON sets(group_name,date DESC,id);
 CREATE INDEX IF NOT EXISTS sets_date ON sets(date DESC,id);
 CREATE TABLE IF NOT EXISTS images (id INTEGER PRIMARY KEY, set_id INTEGER NOT NULL REFERENCES sets(id) ON DELETE CASCADE, mime TEXT NOT NULL, bytes BLOB NOT NULL, hash TEXT NOT NULL, source_url TEXT NOT NULL DEFAULT '', UNIQUE(set_id,hash));
 CREATE INDEX IF NOT EXISTS images_set ON images(set_id,id);
 CREATE TABLE IF NOT EXISTS remote_limits (host TEXT PRIMARY KEY, next_at INTEGER NOT NULL DEFAULT 0, paused_until INTEGER NOT NULL DEFAULT 0, reason TEXT NOT NULL DEFAULT '');
 CREATE TABLE IF NOT EXISTS remote_leases (host TEXT PRIMARY KEY, until INTEGER NOT NULL);
 CREATE TABLE IF NOT EXISTS remote_requests (host TEXT NOT NULL, at INTEGER NOT NULL);
 CREATE TABLE IF NOT EXISTS remote_budgets (host TEXT PRIMARY KEY, max_requests INTEGER NOT NULL, until INTEGER NOT NULL);
 CREATE INDEX IF NOT EXISTS remote_requests_host_time ON remote_requests(host,at);
 CREATE TABLE IF NOT EXISTS page_cache (url TEXT PRIMARY KEY, bytes BLOB NOT NULL, mime TEXT NOT NULL, fetched_at INTEGER NOT NULL);
 PRAGMA user_version=2;`)
	if err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}
func (s *Store) Close() error { return s.db.Close() }

func (s *Store) Import(ctx context.Context, drafts []SetDraft) (ImportReport, error) {
	return s.importSheet(ctx, drafts, false)
}

func (s *Store) PreviewImport(ctx context.Context, drafts []SetDraft) (ImportReport, error) {
	return s.importSheet(ctx, drafts, true)
}

func (s *Store) importSheet(ctx context.Context, drafts []SetDraft, preview bool) (ImportReport, error) {
	report := ImportReport{Total: len(drafts), Candidates: []SetID{}, ScanCandidates: []SetID{}, Changes: []ImportChange{}}
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
		if isNew {
			result, e := tx.ExecContext(ctx, `INSERT INTO sets(title,group_name,date,source,example,notes,raw_date,raw_cells,import_key,import_warning,origin) VALUES(?,?,?,?,?,?,?,?,?,?,'sheet')`, d.Title, d.Group, d.Date, d.Source, d.Example, d.Notes, d.RawDate, d.RawCells, d.ImportKey, d.ImportWarning)
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
				if _, err = tx.ExecContext(ctx, "DELETE FROM images WHERE set_id=?", id); err != nil {
					return report, err
				}
				if _, err = tx.ExecContext(ctx, "UPDATE sets SET image_state='pending',image_error='' WHERE id=?", id); err != nil {
					return report, err
				}
			}
		}
		if (isNew || old.Example != d.Example) && strings.HasPrefix(d.Example, "https://") {
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
	return report, tx.Commit()
}

const setColumns = `id,title,group_name,date,source,example,notes,raw_date,raw_cells,import_warning,origin,image_state,image_error`

func scanSet(row interface{ Scan(...any) error }) (Set, error) {
	var x Set
	err := row.Scan(&x.ID, &x.Title, &x.Group, &x.Date, &x.Source, &x.Example, &x.Notes, &x.RawDate, &x.RawCells, &x.ImportWarning, &x.Origin, &x.ImageState, &x.ImageError)
	x.Images = []Image{}
	return x, err
}
func (s *Store) attach(ctx context.Context, x *Set) error {
	rows, err := s.db.QueryContext(ctx, "SELECT id,source_url FROM images WHERE set_id=? ORDER BY id", x.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var img Image
		if err := rows.Scan(&img.ID, &img.SourceURL); err != nil {
			return err
		}
		img.URL = fmt.Sprintf("/api/images/%d", img.ID)
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
	f := Facets{Groups: []Facet{}}
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
	return f, rows.Err()
}
func (s *Store) Save(ctx context.Context, id SetID, d SetDraft) (Set, error) {
	if err := d.Validate(); err != nil {
		return Set{}, err
	}
	if id == 0 {
		r, err := s.db.ExecContext(ctx, `INSERT INTO sets(title,group_name,date,source,example,notes) VALUES(?,?,?,?,?,?)`, d.Title, d.Group, d.Date, d.Source, d.Example, d.Notes)
		if err != nil {
			return Set{}, err
		}
		n, err := r.LastInsertId()
		if err != nil {
			return Set{}, err
		}
		return s.Get(ctx, SetID(n))
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Set{}, err
	}
	defer tx.Rollback()
	var oldExample string
	if err = tx.QueryRowContext(ctx, "SELECT example FROM sets WHERE id=?", id).Scan(&oldExample); err != nil {
		return Set{}, err
	}
	_, err = tx.ExecContext(ctx, "UPDATE sets SET title=?,group_name=?,date=?,source=?,example=?,notes=? WHERE id=?", d.Title, d.Group, d.Date, d.Source, d.Example, d.Notes, id)
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
