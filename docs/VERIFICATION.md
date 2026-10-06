# Verification — 6 October 2026

- `npm run check`: gallery and back-office, zero errors/warnings.
- `npm test`: 3 behavior tests pass.
- `npm run build`: both independent Svelte applications build.
- `go test -race ./...`: parser, transaction/import, query, media safety, auth/upload, retry and stale-extraction regression tests pass.
- `go vet ./...`: passes.
- `python3 scripts/verify-seed.py`: every one of 165 source data rows and all raw cells match the actual SQLite seed; database integrity, foreign keys, JPEG MIME/magic and stored hashes pass.
- Docker Compose builds all three images and starts the main stack. Backend health checked through frontend and back-office same-origin proxies.
- Playwright: 6 browser acceptance tests pass across desktop Chrome and mobile viewport. Real seed gallery filters/grouping/date/detail/empty state; authenticated manual addition with actual uploaded JPEG visible from gallery; repeated CSV import returns 0 created / 165 updated and retains manual records.
- Isolated `bias-archive-test` containers and volume removed after verification. Main volume preserved with 165 original records; no test records were added to it.
- Four GitHub workflow files parse as YAML. Actual GitHub execution and registry publishing await a user-authorized push; user explicitly requested local files only.

Initial preview outcome is partial: 29 ready, 37 lack a public URL, 99 failed (missing public image, rate limited HTTP 429, or forbidden HTTP 403). Metadata is complete. No substitute images are used. Failed records remain retryable from the back-office.

## Docker development mounts

- Default Compose uses source bind mounts, Vite HMR for both Svelte applications and Air for Go. Production builds remain in `compose.build.yaml`, which the integration workflow uses explicitly.
- Edited each application's source on the host and observed the changed text in an already-open browser page through HMR, without a page refresh. Restored both files and observed the original text return.
- Edited the Go health response on the host and observed Air rebuild/restart the API automatically. Restored the file and verified the original healthy response returned.
- No image rebuild or container restart commands were issued during these source-edit checks. SQLite facets matched before and after, retaining all 165 sets.
- Development Compose validation, both Svelte checks and the 3 Node behavior tests passed.


## Multi-image sets, carousel and provider pacing

- `npm run check`: both Svelte applications pass with zero errors/warnings. Both production builds succeeded.
- `go test -race ./...` and `go vet ./...`: pass, including atomic 2–5-image uploads, deduplication at capacity, multiple-image extraction, post/product gallery parsing, preserved provider cooldowns, alias/subdomain sharing, daily budget and single in-flight request protection.
- Playwright: 8 acceptance tests pass on an isolated production-style stack across desktop and mobile. Includes explicit set_id/editor identity, real two-image uploads, idempotent import, wheel and arrow controls, keyboard focus/Enter, native mobile touch swipe, and opening the selected image. Test containers and volume removed afterwards.
- Actual development browser: Karina's 5 stored images were displayed; next-image selection decoded the second real JPEG. Back-office shows set_id 136 readonly, ARIN's `1/5 needs 2+` status and disabled Gank retry during the persisted pause.
- Actual enrichment added 34 real JPEGs: final seed has 165 sets and 63 images. Counts: 136 sets with 0 images, 1 with 1, 26 with 2, 2 with 5. Thus 28 complete sets; the remaining 137 remain visibly incomplete.
- Gank batch stopped at the configured 60 requests per rolling day, before making another request. No remote Gank 429/403 was encountered in this enrichment batch. The saved local budget pause expires 7 October 2026 at 08:59:58 UTC (15:59:58 Bangkok). Gumroad enrichment continued independently and set_id 93 reached 5 images.
- The final seed was exported through SQLite VACUUM INTO and verified against every original CSV row, all stored JPEG hashes, database integrity/foreign keys and the 5-image maximum. See IMAGE_IMPORT_REPORT.json for per-set outcomes.


## Bottom carousel drag bar

- Replaced card arrows and wheel/native image scrolling with a centered bottom bar. Mouse/touch pointer capture moves images continuously; release snaps to the nearest image. The bar supports ArrowLeft/ArrowRight and Home/End.
- Updated the carousel acceptance test first: both desktop and mobile failed because the bar was absent, then passed after implementation. All 4 read-only gallery/carousel browser cases pass against the development stack, covering touch drag, continuous mouse movement, dragging outside the bar, backward movement, page scrolling, keyboard selection and opening the selected image.
- Frontend Svelte check passes with zero errors/warnings; production build succeeds. Real local Karina cards show the bar with all 5 stored images on desktop and mobile screenshots. Source changes appeared through the existing development mounts/HMR.
- No source-provider requests, database changes or image enrichment were needed for this UI change.
