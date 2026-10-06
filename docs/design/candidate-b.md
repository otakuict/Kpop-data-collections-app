# Candidate B: durable SQLite import ledger

## Problem

The greenfield app needs a public Svelte gallery, an authenticated Svelte back office, a Gin API, and SQLite for both metadata and image bytes. The supplied CSV has three decorative lines, a header on line 4, and **165 data rows**. Several data rows lack a usable date, example link, or other metadata. Import must preserve all 165, keep the original cells and row numbers, extract only real image bytes, and survive a restart between catalog import and remote media retrieval. The backend is the sole SQLite writer; browsers never write the database or fetch arbitrary example URLs on behalf of the server.

## Usage (caller's view)

The back office asks the API to import a known Google Sheet or uploads its CSV export. It receives a durable job immediately, polls one status URL, and sees a diagnostic beside each sheet row. It can retry failed image extraction without importing metadata again. The public gallery reads entries from one catalog endpoint and image bytes from one media endpoint.

```ts
// Back office: the UI never sends a fetch-anywhere URL.
const job = await api.importSheet({ sheetId: configuredSheetId, gid: configuredGid });
const result = await api.getImportJob(job.id);
// result.rows includes sourceRowNumber, entryId, state, and diagnostic.

// Back office: replay only recoverable extraction failures.
await api.retryImportMedia(job.id);

// Public gallery: missing artwork is an explicit null, not a synthetic image.
const page = await api.listEntries({ group: "aespa", cursor: null, limit: 40 });
renderCard(page.items[0].imageUrl); // null means show the app's neutral placeholder.
```

The Gin handlers have one application dependency, `Catalog`. Three call sites illustrate its API:

```go
// POST /admin/imports/sheet, after session and CSRF middleware.
job, err := catalog.EnqueueSheetImport(ctx, SheetRef{ID: req.SheetID, GID: req.GID})

// GET /admin/imports/:id, after session middleware.
view, err := catalog.ImportStatus(ctx, JobID(routeID))

// GET /api/entries, public.
page, err := catalog.ListEntries(ctx, EntryFilter{Group: group, Cursor: cursor, Limit: limit})
```

## Shape

### Data and outcomes

```go
type JobID string
type EntryID int64
type MediaID int64
type SourceKey string // canonical sheet ID + gid; local seed has its own fixed key

type SheetRef struct { ID SheetID; GID SheetGID } // parsed and bounded at HTTP boundary
type ImportInput struct { Source SourceKey; CSV []byte; SHA256 [32]byte }
type ImportJob struct { ID JobID; Source SourceKey; State JobState; Total, Imported, MediaReady, Failed int }
type ImportRow struct {
    Job JobID
    SourceRowNumber int // physical CSV row; first data row is 5
    RawCells []string   // every cell, including spare columns, stored losslessly as JSON
    Entry EntryID
    State RowState // imported | image_ready | missing_example | extraction_error
    Diagnostic string
}
type CatalogEntry struct {
    ID EntryID
    Source SourceKey
    SourceRowNumber int
    RawCells []string
    Date *CivilDate // nil for blank, literal "0", or invalid YYMMDD
    Name, Group, SourceLabel, Remark string
    ExampleURL *SafePostURL
    Image *MediaID
}
type Page[T any] struct { Items []T; NextCursor *string }
```

`CatalogEntry` has a non-null raw source row and an optional parsed date, URL, and image. The source row ordinal is the stable import key within a Sheet. A later import updates that ordinal; when its example URL changes, its old image link is cleared before extraction. This sacrifices stable entry IDs under row reordering, but never displays another row's old image. The raw cells plus immutable job snapshot preserve evidence of what changed. No catalog row is deleted just because a subsequent partial import fails. Once an entire snapshot is parsed and committed, entries absent from that source's latest snapshot are marked inactive in the same transaction.

Dates parse `YYMMDD` as 20YY-MM-DD only when the calendar date is valid. `0`, blanks, and invalid strings become SQL `NULL` while the raw value remains in `raw_cells_json`. All 165 data rows are staged, including rows with empty metadata; a row cannot disappear due to validation. The CSV parser recognizes the `Date,Name,GROUP,Source,anything,Example,REMARK` header and starts at the following physical row, keeping trailing columns.

### Public Go surface

```go
type Catalog interface {
    EnqueueSheetImport(context.Context, SheetRef) (ImportJob, error)
    EnqueueCSVImport(context.Context, SourceKey, []byte) (ImportJob, error)
    ImportStatus(context.Context, JobID) (ImportJobView, error)
    RetryImportMedia(context.Context, JobID) (ImportJob, error)
    ListEntries(context.Context, EntryFilter) (Page[EntryView], error)
    GetMedia(context.Context, MediaID) (MediaBlob, error)
    CreateManualEntry(context.Context, ManualEntryInput) (EntryView, error)
}

type Runner interface { Run(context.Context) error } // started by backend main, stopped on shutdown

func NewCatalog(db *sql.DB, fetcher SafeFetcher, clock Clock) (Catalog, Runner, error)
func NewRouter(c Catalog, sessions SessionStore) *gin.Engine
```

Only `Catalog` executes SQL mutations. `NewRouter` owns HTTP parsing, response mapping, authentication, and CSRF. The worker consumes jobs through the same catalog implementation, not through a second database owner. API handlers never perform media extraction directly. `SafeFetcher` is private to the backend package; it accepts a parsed `SheetRef` or a parsed `SafePostURL`, not an arbitrary string or URL from a row.

### Durable state and worker contract

```sql
CREATE TABLE import_jobs (
  id TEXT PRIMARY KEY, source_key TEXT NOT NULL, snapshot_sha256 BLOB NOT NULL,
  csv_bytes BLOB NOT NULL, state TEXT NOT NULL,
  created_at TEXT NOT NULL, updated_at TEXT NOT NULL, lease_until TEXT,
  total_rows INTEGER NOT NULL DEFAULT 0, error TEXT
);
CREATE UNIQUE INDEX import_jobs_same_snapshot ON import_jobs(source_key, snapshot_sha256);
CREATE INDEX import_jobs_claim ON import_jobs(state, lease_until, created_at);

CREATE TABLE entries (
  id INTEGER PRIMARY KEY, source_key TEXT, source_row_number INTEGER,
  raw_cells_json TEXT NOT NULL, date_iso TEXT, name TEXT NOT NULL,
  group_name TEXT NOT NULL, source_label TEXT NOT NULL, remark TEXT NOT NULL,
  example_url TEXT, media_id INTEGER REFERENCES media(id), active INTEGER NOT NULL DEFAULT 1,
  UNIQUE(source_key, source_row_number)
);
CREATE INDEX entries_gallery ON entries(active, date_iso DESC, id DESC);
CREATE INDEX entries_group_gallery ON entries(active, group_name, date_iso DESC, id DESC);

CREATE TABLE import_rows (
  job_id TEXT NOT NULL REFERENCES import_jobs(id), source_row_number INTEGER NOT NULL,
  raw_cells_json TEXT NOT NULL, entry_id INTEGER REFERENCES entries(id),
  state TEXT NOT NULL, diagnostic TEXT NOT NULL DEFAULT '',
  PRIMARY KEY(job_id, source_row_number)
);
CREATE INDEX import_rows_work ON import_rows(job_id, state, source_row_number);

CREATE TABLE media (
  id INTEGER PRIMARY KEY, sha256 BLOB NOT NULL UNIQUE, content_type TEXT NOT NULL,
  bytes BLOB NOT NULL, byte_length INTEGER NOT NULL, created_at TEXT NOT NULL
);
```

`EnqueueSheetImport` fetches a bounded Google export from an exact configured Sheets endpoint, then stores the immutable CSV bytes and job in one transaction. `EnqueueCSVImport` stores a bounded uploaded byte snapshot the same way. The `(source_key, snapshot_sha256)` constraint returns the existing job for repeated submissions of identical content. Seed uses the checked-in CSV and a fixed `SourceKey`; startup enqueue is idempotent.

One worker loop in the Gin process claims the oldest queued job or an expired lease on a `running`/`media_pending` job using a short `BEGIN IMMEDIATE` transaction, then works without holding a DB write transaction across the network. It parses all source rows before catalog mutation. One transaction upserts all 165 catalog rows and their `import_rows`, updates active flags for that source, and marks the job `media_pending`; a parse failure records a job error without changing the catalog. The worker then fetches each allowable example page and image, one row at a time, with a network deadline and byte limits. For each row it uses one short transaction to insert/deduplicate media by SHA-256, link the entry only if the example URL still matches, and update that row's outcome. Completion is derived from durable row states; status counts are SQL aggregates, not synchronized counters.

Crash/restart behavior: expired leases are reclaimable; already staged rows are re-read; `UNIQUE(source_key, source_row_number)` and media hashes make repeats idempotent. A failed row remains visible with a diagnostic. `RetryImportMedia` changes only recoverable row states back to pending, preserving the raw snapshot and metadata. A missing example is terminal until a newer import supplies one. A media fetch error does not fail catalog import or discard that row. The worker has one in-process instance; an SQLite lease also prevents duplicate claims if operators briefly overlap processes. Set WAL, foreign keys, busy timeout, one DB connection for writes, and bounded read connections.

### Trust boundaries

Admin routes require a server-verified session cookie (`HttpOnly`, `Secure` in deployed HTTPS, `SameSite=Lax`) and CSRF token for every mutation; the initial admin credential is configured at deployment and stored as a password hash, never exposed to Svelte. Public routes are read-only. Upload and Sheet import size limits are enforced before allocation and again before transaction.

Sheet imports accept only validated sheet ID and numeric gid, constructing the export URL server-side. Example extraction accepts HTTPS `ganknow.com/post/<UUID>` links only. `SafeFetcher` resolves each allowed host, rejects loopback/private/link-local/reserved addresses, dials the checked public IP, verifies TLS hostname, revalidates every redirect against a small explicit host allowlist, and refuses redirects to arbitrary origins. The image host allowlist is configured from actual observed source pages; an unknown CDN yields an `extraction_error`, never a permissive fallback. Fetch page and media with time, redirect, compressed/uncompressed byte, content-type, and pixel limits; decode only known raster formats and store confirmed bytes in SQLite. Strip active content such as SVG. Keep only the canonical source URL in catalog; never render fetched HTML in Svelte.

### Module map

```text
backend/cmd/server/main.go             configuration, SQLite open, Gin + worker lifecycle
backend/internal/catalog/catalog.go    sole public catalog interface + implementation
backend/internal/catalog/import.go     CSV parser, upsert transaction, job lease/row state
backend/internal/catalog/media.go      extraction policy, checked media storage/linking
backend/internal/http/router.go        Gin wire types, sessions, CSRF, response mapping
backend/internal/fetch/safe.go         bounded Google/post/media fetch with SSRF policy
backend/internal/sqlite/schema.sql     one schema and indexes
frontend/src/lib/api.ts                typed HTTP client, gallery and admin calls
frontend/src/routes/+page.svelte       public gallery
frontend/src/routes/admin/+page.svelte admin entry and import job views
seed/source.csv                        immutable supplied CSV, all 165 rows
compose.yaml                           backend + frontend, persisted SQLite volume
.github/workflows/frontend.yml         Svelte install/check/build on frontend paths
.github/workflows/backend.yml          Go test/build on backend/seed paths
.github/workflows/backoffice.yml       admin UI checks on admin/shared-client paths
```

The public interface is deliberately deep: callers ask for an import and a status; the catalog hides snapshot identity, CSV structure, transaction ordering, leases, media deduplication, and retries. Gin wire types and SQLite rows stay private. One backend package owns the core state transitions so a contributor cannot add a second writer by copying a nearby handler.

## Synthesis decision

Candidate B for the parallel design exercise. The parent synthesis should compare its durable import ledger and single SQLite owner against the other candidate's whole shape. No synthesis decision is claimed here.

## Tradeoffs accepted

- We accept a BLOB-backed job snapshot in SQLite in exchange for deterministic retries against identical bytes after an external Sheet changes.
- We accept sequential network extraction in the first release in exchange for bounded requests, simple leases, and predictable SQLite writes.
- We accept source row ordinal as the import identity in exchange for full coverage of rows without a stable source key; reordering can change entry IDs, and URL changes clear image links to prevent mismatch.
- We accept visible media failures in exchange for refusing unapproved image hosts and synthetic images.
- We accept SQLite storing image bytes in exchange for the requested single portable data store; upload limits and image size caps bound database growth.

## Alternatives considered

- A synchronous `POST /import` that fetches all images before responding exposes network lifetime and partial-progress policy to the caller, and a restart loses the in-flight operation. Its small function signature hides too little useful complexity.
- A separate queue service plus object storage could scale extraction, but exposes deployment and consistency rules across two stores. It is disproportionate to a 165-row catalog with one backend owner.
- Browser-side extraction would expose remote HTML to users and cannot enforce one image policy or reliably persist results.

## Implementation reconciliation

No implementation has started for this candidate. Reconcile any accepted changes to usage, outcomes, ownership, and signatures before an implementation unit closes.

## Open questions and risks

- Which image CDN hosts do real Ganknow post pages use, and can we approve exact hosts based on a bounded sample before media extraction ships?
- Should rows removed from a later complete Sheet snapshot become hidden (`active=0`) or remain visible with an archive marker in the gallery?
- Does the deployment environment terminate HTTPS before Gin, and how will the initial admin credential be provisioned securely?

## Next implementation step

Implement schema, CSV parser, and one atomic metadata staging transaction, then prove the checked-in seed creates exactly 165 catalog and import-row records including the blank and unknown-date cases.
