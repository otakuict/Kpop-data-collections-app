# Candidate A: synchronous catalog operations, one SQLite owner

## Problem

Build a public Svelte gallery and separate Svelte back-office backed by Gin. One SQLite database stores both metadata and original image bytes. The source has 165 data rows after the fourth-row header; all columns and raw values must survive ingestion, including unknown date `0`. Image failures must never hide metadata or cause fabricated thumbnails. This is greenfield; no existing API needs compatibility.

## Usage (caller's view)

1. `docker compose up --build` starts API, gallery, and back-office. The API atomically seeds the checked-in source snapshot once. Gallery immediately lists all 165 records, showing a neutral “Image unavailable” state where needed.
2. An administrator signs in, pastes a Google Sheet URL, and selects Import. The response contains created, updated, unchanged, and invalid-date diagnostics. All rows remain browsable. The back-office then lets the administrator extract/retry images; each item completes as one bounded HTTP operation with progress shown by the client.
3. Administrator creates an entry with text and optional image upload. The public gallery can display it immediately after a successful transaction.

```js
// Gallery: domain JSON, never direct Sheet or image-provider calls.
const page = await catalog.list({ group: 'aespa', query: 'Karina', offset: 0, limit: 24 });
// page.items carry local /api/images/:id URLs and an explicit image state.

// Back-office: server validates provider URL and owns import semantics.
const report = await admin.importSheet(sheetURL);
for (const id of report.imageCandidates) {
  const result = await admin.extractImages(id); // one entry per bounded request
  showResult(result); // failure is visible and retryable
}

const entry = await admin.create({ name, group, source, date: null, remarks }, uploads);
```

## Shape

### Module map and ownership

- `apps/gallery/`: Svelte/Vite public browser application; filters, detail view, accessible image layout.
- `apps/backoffice/`: Svelte/Vite administration; login, manual creation, Sheet import result table, sequential image extraction with retry. No secrets compiled into JS.
- `apps/api/cmd/server/`: configuration, server startup, shutdown; no catalog SQL.
- `apps/api/internal/catalog/`: sole database owner; domain models, schema/migrations, queries, metadata upsert, image BLOB writes, import receipts. SQL and transaction objects remain private.
- `apps/api/internal/source/`: source-format knowledge, Google URL normalization/CSV parsing, bounded Gank extraction and safe remote-image fetching; never writes SQLite.
- `apps/api/internal/httpapi/`: Gin transport, admin session authentication, CSRF/origin checks, multipart limits, wire/domain mapping. Calls catalog operations; does not coordinate multiple writes.
- `data/source.csv`: exact fetched seed, source identifier and SHA-256 manifest alongside it. No parallel hand-maintained JSON list.
- `.github/workflows/{gallery,backoffice,api}.yml`: independently triggered verification/build/publish units.

Call chains are handler → catalog → SQL or handler → source → catalog, at most three ownership boundaries. Catalog `Import` owns all receipt/upsert transaction decisions; browser never coordinates database phases. Source owns external representations. A single process is the only mounted SQLite writer, per single ownership; no worker service or script opens the production database.

### Go type and signature sketch

```go
// Domain types; constructors validate external values.
type EntryID string
type ImageID string
type CalendarDate struct { value string } // validated YYYY-MM-DD, no zero value admitted by parser
// nil date means unknown. Raw date is independently retained for fidelity.
type EntryDraft struct {
    Name, Group, Source, Remarks string
    Date *CalendarDate
    ExampleURLs []string
}
type SourceRow struct {
    Key string // stable identity defined below
    SheetRow int
    Cells []string // all CSV cells in original order, including blanks
    Draft EntryDraft
    Diagnostics []Diagnostic
}
type Snapshot struct { SourceID, Digest string; Header []string; Rows []SourceRow }
type ImageState string // pending | ready | unavailable
// Catalog-derived DTOs cannot claim ready unless linked images exist.
type Entry struct { ID EntryID; EntryDraft; Images []Image; ImageState ImageState }
type ImportReport struct {
    Created, Updated, Unchanged int
    Diagnostics []Diagnostic
    ImageCandidates []EntryID
}
type ExtractionResult struct { EntryID EntryID; State ImageState; Added int; Diagnostic string }
type Store struct { /* private database handle */ }
func Open(path string) (*Store, error) { panic("not implemented") }
func (s *Store) Import(ctx context.Context, snapshot Snapshot) (ImportReport, error) { panic("not implemented") }
func (s *Store) List(ctx context.Context, filter Filter) (Page, error) { panic("not implemented") }
func (s *Store) Create(ctx context.Context, draft EntryDraft, images []ValidatedImage) (Entry, error) { panic("not implemented") }
func (s *Store) RecordExtraction(ctx context.Context, id EntryID, result ExtractedImages) (ExtractionResult, error) { panic("not implemented") }
func (s *Store) ReadImage(ctx context.Context, id ImageID) (ImageBytes, error) { panic("not implemented") }

// source package boundary hides provider transport and parsing policy.
func (f *Fetcher) Sheet(ctx context.Context, rawURL string) (Snapshot, error) { panic("not implemented") }
func ParseSeed(csv []byte, sourceID string) (Snapshot, error) { panic("not implemented") }
func (f *Fetcher) Images(ctx context.Context, urls []string) (ExtractedImages, error) { panic("not implemented") }
```

`ValidatedImage` and `ExtractedImages` have private fields plus safe constructors; handlers cannot label unchecked bytes as safe. Real code may place shared domain types within catalog and have source depend on it; catalog must not import source. State transitions and imported source provenance are catalog-owned.

### Schema and dominant reads

- `entries(id PRIMARY KEY, name, group_name, source_name, date_iso NULL, remarks, origin, created_at, updated_at)`.
- `source_rows(source_id, row_key, entry_id REFERENCES entries, sheet_row, header_json, cells_json, content_hash, PRIMARY KEY(source_id,row_key))`.
- `entry_examples(entry_id, ordinal, url, PRIMARY KEY(entry_id,ordinal))`.
- `images(id PRIMARY KEY, sha256 UNIQUE, mime CHECK allowed raster types, bytes BLOB NOT NULL, width, height)`.
- `entry_images(entry_id, image_id, ordinal, PRIMARY KEY(entry_id,image_id))`.
- `extraction_attempts(entry_id PRIMARY KEY, attempted_at, outcome, diagnostic)`; successful previous images survive a failed retry.
- `imports(source_id, snapshot_hash, imported_at, report_json, PRIMARY KEY(source_id,snapshot_hash))`.

Indexes: entries `(group_name,date_iso DESC,id)`, `(date_iso DESC,id)`; entry_images `(entry_id,ordinal)`; source_rows `(entry_id)`. Gallery query orders dated records descending and unknown dates last, then ID as stable tie-breaker. Search is parameterized `LIKE` across name/group/source for this small catalog; no separate search index. Image bytes are fetched only by their dedicated endpoint, never in listing SQL. Image SHA deduplication avoids storing repeated assets while multiple source records remain distinct.

SQLite uses foreign keys, WAL, busy timeout, short write transactions, and one database connection initially. No network requests occur while holding a transaction. An import serializes its metadata transaction; manual entry creation remains an independent atomic operation. One API replica with persistent volume is the supported deployment shape.

### Fidelity and idempotency

Locate the actual Date/Name/GROUP/Source header, not a fixed skip count alone. Preserve every subsequent data row and all 28 cells where present; never use date validity as a row filter. Date `0` and blank map to NULL with raw values retained. Valid six-digit dates map to 20YY-MM-DD after calendar validation; malformed dates yield visible diagnostics plus NULL, not dropped records.

Without a source-owned ID, perfect reconciliation of arbitrary edits/reorders is impossible. Explicit contract: identity is SHA-256 of the canonical example URL set when available, otherwise exact normalized name/group/source/raw-date tuple, with occurrence suffix for duplicate identities in source order. All duplicates remain separate records. Metadata changes with stable example URLs update; altered identity is a new record. Import is additive, never implicitly deletes rows removed from the Sheet. This limitation is visible in the import UI. Exact repeated snapshots return the saved report and independently query remaining missing-image candidates, so retries can continue extraction. Seed uses the same canonical Sheet source identifier as subsequent imports to avoid double-importing the snapshot.

Metadata snapshot validates completely before its single transaction. Empty/fetch-invalid CSV fails without writes. Row-level metadata anomalies remain rows with diagnostics. Each extraction does bounded external work then commits images and status atomically. On interruption, committed rows/images survive; rerunning extraction deduplicates by SHA. Failed retries retain previous images, update diagnostics, and report usable-image count truthfully.

### Authentication and fetching

Public GET endpoints: entries, individual entry, and validated image bytes. Admin login checks a server-side password hash from environment; rate limit login attempts. Issue a signed short-lived HttpOnly SameSite=Strict session cookie; Secure required in production. Random signing key comes from server environment. Mutating routes require session plus exact allowed Origin and CSRF token; authenticated requests never rely on a secret embedded in Svelte. Local Compose credentials are explicitly development-only; production fails startup without secrets.

Accept only Google Sheet URLs normalized into an HTTPS export URL with validated sheet ID/gid, not arbitrary CSV URLs. Fetch export with bounded bytes/time and provider redirect allowlist. Example extraction initially supports only `ganknow.com/post/<UUID>`; parse structured post image data with OG-image fallback, recording unsupported/non-image failures. External responses are untrusted. Every request and redirect enforces HTTPS, no userinfo, allowed hosts/ports, resolved public addresses, and a dialer pinned to a validated resolution to prevent rebinding. Block loopback, private, link-local, multicast, IPv6 mapped private addresses and metadata endpoints. Image CDN hosts need explicit configuration from observed provider evidence; do not guess or permit all domains. Download limits per image and per extraction, raster magic-byte validation, dimension/pixel limits, and SVG/HTML rejection apply equally to uploads. Return image bytes with correct MIME and nosniff.

### HTTP and operations contract

`GET /api/entries`, `GET /api/entries/:id`, `GET /api/images/:id`; admin session endpoints; `POST /api/admin/entries` multipart; `POST /api/admin/imports` Sheet URL; `POST /api/admin/entries/:id/extract-images`. Error envelope exposes safe codes and messages. An image miss returns 404; extraction refusal is a normal explicit result, not a fake success. Client batches extraction sequentially; a refresh loses progress display but retry reconstructs missing-image work from database. No persistent job queue.

Three Docker builds and path-scoped GitHub workflows include relevant lockfiles/shared Compose changes. API workflow runs Go tests/build and image build; frontend workflows run npm ci/check/build and image builds independently. Publish immutable GHCR images only when repository and registry access exist; deployment jobs target configured environments and secrets, with backend migrations occurring under the sole API owner. Do not invent a live deployment target. Compose mounts SQLite volume only to API and reverse-proxies browser API requests to avoid broad CORS.

Verification: parse seed and assert 165 persisted rows, raw-cell roundtrip including duplicate rows and date 0; repeat import stable IDs/count; failed import rollback; missing-image retries preserve prior bytes; SSRF redirect/rebinding/private-IP cases; auth/CSRF refusal; manual upload and image byte roundtrip; both Svelte builds; Compose gallery/admin smoke flow. Go absent locally means run backend checks in a available container runtime or explicitly report the unavailable check rather than claiming completion.

## Synthesis decision

Independent candidate; orchestrator selects and records synthesis.

## Tradeoffs accepted

- We accept bounded synchronous extraction per entry and client-driven progress in exchange for no queue, leases, workers, or job recovery protocol.
- We accept one API writer replica and modest throughput in exchange for SQLite correctness and simple deployment.
- We accept additive import identity limits in exchange for fidelity without modifying the user's Sheet.

## Alternatives considered

A persisted import-job queue hides long-running work from callers, but exposes job lifecycle/status/cancellation and introduces restart leases and worker ownership for only 165 rows. An external object store hides byte-serving scale concerns but violates SQLite image storage and adds credentials and consistency boundaries.

## Implementation reconciliation

No implementation yet; candidate contracts remain unverified.

## Open questions and risks

Can the source later provide a permanent row ID to reconcile edits that change every identifying field? Which deployment host and domain will supply production environment configuration? Does provider extraction expose full image assets without browser authentication? None blocks preserving metadata and honest extraction diagnostics.

## Next implementation step

Implement the catalog transaction and seed fidelity test first, proving 165 rows and retained raw data before connecting the network source.
