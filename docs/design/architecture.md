# K-pop archive architecture

## Problem and usage

Preserve all 165 source rows while making their real preview images browsable. `docker compose up --build` serves gallery on 5173 and back-office on 5174; both read Gin at `/api` through their own proxy. Only Gin opens the persisted SQLite database. A checked-in seed DB provides initial metadata and locally stored image BLOBs without depending on live Sheets at startup.

Gallery calls `GET /api/sets?q=&group=&from=&to=&sort=&page=&limit=` and `/api/facets`, then `/api/sets/:id` and `/api/images/:id`. Back-office authenticates with an administrator bearer token kept only in browser memory; calls `POST /api/admin/import` with a Google Sheet URL or CSV multipart upload, `POST /api/admin/sets` for manual metadata, `PUT /api/admin/sets/:id` for corrections, `POST /api/admin/sets/:id/images` for uploads, and `POST /api/admin/sets/:id/extract` for retryable per-set preview ingestion.

## Shape and signature sketch

`SetID` and `ImageID` are integer domain IDs. `Set` contains title, group, ISO date (empty means unknown), raw date/example/row cells, source, notes, origin, image state/error and an image list. `SetDraft` is parsed and validated at the HTTP/CSV boundary. `Image` exposes only local URL and provenance. Images are validated raster bytes, resized to a maximum 1200-pixel edge before SQLite BLOB storage; hashes deduplicate bytes within each set.

- `backend/internal/gallery/model.go`: domain structures and metadata validation.
- `store.go`: sole SQL owner, schema, atomic metadata upsert, queries, image BLOB writes.
- `import.go`: Sheet format knowledge and deterministic identity; all raw cells retained.
- `media.go`: HTTPS fetch/DNS/redirect policy, OG preview extraction, byte/pixel limits and raster normalization.
- `http.go`: Gin routes, bounded request parsing, auth, error/status translation.
- `backend/cmd/server/main.go`: configuration, seed copying, CLI seed refresh and graceful lifecycle.
- `frontend/`, `backoffice/`: independently built Svelte/Vite apps; no direct provider access.

```go
func ParseSheet(io.Reader, string) ([]SetDraft, error) // entire source parsed before writes
func (s *Store) Import(context.Context, []SetDraft) (ImportReport, error) // one metadata transaction
func (s *Store) List(context.Context, Filter) (Page, error)
func (s *Store) Save(context.Context, SetID, SetDraft) (Set, error)
func (s *Store) AddImage(context.Context, SetID, Raster) error
func Extract(ctx context.Context, store *Store, id SetID) (Set, error) // bounded, no transaction across HTTP
func Router(store *Store, adminToken string) *gin.Engine
```

Indexes `(group_name,date DESC,id)`, `(date DESC,id)`, unique `import_key`, and `(set_id,hash)` for images. WAL, foreign keys, busy timeout, one database connection. Query values bound as SQL parameters. Network work precedes short image write transactions. Manual records have no import key.

## Behavior contracts

CSV header is discovered after decorative rows. Known Date/Name/GROUP/Source/Example/REMARK map to metadata; every column is retained. `0` is unknown date. Invalid dates remain unknown with a visible warning. Identity is source + group + raw date + title, with occurrence suffix for duplicate identical rows. Reimport updates source metadata without duplicate records, preserves images only when example is unchanged, never deletes absent rows or manual entries. Renaming the identifying tuple creates a new record; an optional `ID` column in future templates provides stable editable identity. This additive policy is explicit in the back-office.

Extraction stores up to 5 distinct public images per source post; imported or existing sets with fewer than 2 remain incomplete. No URL/non-URL examples are retained with `missing` state. Unsupported, deleted, or inaccessible pages report `failed` with the reason. OG-image and twitter-image accepted; Gank resized social-card suffix removed using observed original URL behavior. Gank default assets excluded. No fabricated thumbnails. Retry retains existing usable images if provider fails. An extraction commits only if the example URL still equals the one it fetched; a changed URL refuses stale results. Upload supports multiple raster images; bytes/MIME/pixel validation applies before storage.

Remote URLs require HTTPS, no credentials, port 443, public DNS answers pinned for dialing, and every redirect is revalidated. Request byte/time/redirect caps, MIME and raster dimension limits. Google Sheet export URL is constructed from a validated sheets URL and numeric gid. Bearer admin routes use constant-time token comparison and reject missing/short startup token. Browser-held token stays in memory and cannot be attached cross-origin by forms. Same-origin proxies avoid CORS.

## Synthesis decision

Base A: simpler bounded per-entry operations and additive stable import identity. B contributes explicit persisted per-entry outcome and preservation of raw source data. Reject B's source row identity (sorting the Sheet changes record ownership), hidden deleted rows, persistent queue/lease protocol, and combined frontend/admin build because they conflict with source fidelity or independent CI requirements. Score A: coverage 5, ownership 5, interfaces 4, fidelity 4, operations 5; B: 4,5,3,4,3. Independent judge result is saved beside this file.

## Tradeoffs accepted

Single API replica and bounded synchronous per-entry extraction fit SQLite and 165 sets. Client controls import progress, persisted states allow retry after refresh. Media normalization bounds seed and DB size while preserving source URLs. Import key edits create additional records unless a template provides ID.

## Implementation reconciliation

Accepted by Codex as design owner before implementation: bearer authentication replaces cookie sessions for an operator tool, reducing CSRF/session state while keeping credentials server-side and out of compiled JS. Independent gallery/backoffice directories replace candidate B's shared app. Checked-in seed SQLite contains actual preview BLOBs and raw metadata; only the offline seed command creates it, never while the server owns that DB. Reusable media normalizer stores web-sized JPEG previews rather than large originals. CLI uses the same parsing/storage functions as API.

Codex review accepted a concurrency guard before closing the media unit: example reads for edits occur inside the write transaction, extracted image commits compare the observed example, and stale outcome writes cannot affect a newer example. This strengthens the saved media ownership contract without adding a worker or queue.

Local development update requested by the user: default `compose.yaml` mounts source directories and runs Vite HMR / Air. `compose.build.yaml` retains nginx production builds and is explicitly used by browser CI. Dependency and compiler caches are separate container volumes; the SQLite volume and sole writer contract stay unchanged.

## Open questions and risks

Deployment host/domain and GitHub repository have not been supplied. Workflows verify/build and publish independent GHCR images when pushed to GitHub; live rollout requires a configured target. Some original examples are intentionally blank/deleted and cannot yield a preview. Google import requires readable/public export or uploaded CSV.

## Next implementation step

Run failing source fidelity and URL-safety tests, implement the metadata/store/media boundaries, then verify the real seed and both browser flows.


## Multi-image sets and provider protection — 6 October 2026

Each set is complete at 2–5 distinct normalized raster images. Upload writes all supplied images in one transaction and checks the final hash count before commit. Metadata-only imported or existing records remain visible as incomplete until they can be filled. Gank extraction now reads public post-media URL literals from the observed Nuxt HTML in addition to social-image metadata, without executing remote scripts. Gumroad product-cover images are read from the observed Inertia JSON, excluding recommendations and video entries. It skips stored source URLs and deduplicates image hashes.

The API and CLI share SQLite ingestion state: Gank root and all subdomains use one request lease, a minimum 20-second completion gap, a 60-request rolling-day local budget, and a persisted cooldown. Response 429 pauses at least one hour, 403/challenge at least 24 hours; a longer Retry-After wins. The transport applies this policy to redirected requests too. A blocked fetch returns a typed pause error so browser and CLI batches stop immediately. HTML caches for 7 days; normal gallery browsing reads local image BLOBs.

Public cards use a per-card Svelte carousel with a centered bottom drag bar and a current-image indicator. Pointer capture lets mouse and touch drag the image track continuously, even outside the bar; releasing snaps to the nearest image. The bar supports ArrowLeft/ArrowRight and Home/End. Wheel/trackpad gestures scroll the page without changing images. Opening a card preserves the selected image in its detail dialog. Back-office exposes numeric set_id in its table/editor and uses actual image counts to distinguish complete and incomplete sets.
