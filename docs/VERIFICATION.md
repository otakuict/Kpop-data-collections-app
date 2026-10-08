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

## Additional 60 Gank requests and set 165

- `go test -race ./...`, `go vet ./...` and formatting pass. Tests cover persisted temporary grants, exactly 60 extra requests, preserved source cooldowns, grant expiry and the correct rolling-window wait after expiry.
- The Nuxt parser reads JSON literals and resolves image references only for the current public post, without executing JavaScript. Tests exclude other posts, private galleries, avatars, blur images and video media.
- Actual request history increased from 60 to 120 with one grant. No Gank HTTP 429/403 occurred. Google image CDN HTTP 429 stopped downloads to that host until 6 October 23:06:16 Bangkok; its pause was preserved during the remaining Gank page requests.
- Added 27 stored image records: final runtime and seed contain 165 sets and 93 images. Counts: 121 sets with 0 images, 2 with 1, 39 with 2, 1 with 3 and 2 with 5; 42 sets meet the minimum. The 3 JPEGs for set 165 were read back through the API and visually inspected.
- The exported seed passed source-row fidelity, JPEG MIME/magic/hash, integrity and foreign-key verification. It contains all 120 request timestamps, the temporary grant and 89 cached pages. All batch processes finished. See IMAGE_FETCH_EXTRA_60_SUMMARY.json for per-set outcomes.

## Gallery lazy loading — 7 October 2026

- A browser test first failed on the existing Previous/Next page buttons. After implementation, 8 desktop/mobile tests pass for automatic appending, retained carousel state, final-batch stop, retrying the same failed batch, ignoring an in-flight batch after filter changes, and returning to the collection start when sort changes at the end. The sort regression also failed before its fix.
- Real API on the development stack: both desktop and mobile loaded 165 sets through [24,48,72,96,120,144,165], preserved grouping, reset on sort changes and displayed empty-search state. No browser JavaScript errors; URL stores filters without page numbers. Mobile screenshot of appended cards visually inspected.
- Both existing desktop/mobile carousel drag tests pass. Frontend Svelte check reports zero errors/warnings; production build succeeds; all 3 catalog tests pass.
- No source-provider requests or database mutations were required for this gallery change.

## Two-image enrichment — 7 October 2026

- Real API extraction attempted all 113 original two-image sets, adding 310 stored records; 109 sets now have 3–5 images. Four unchanged two-image examples are recorded explicitly.
- Snapshot comparison confirms all 240 old image IDs/hashes remain and every new record belongs to the original cohort; other 52 sets unchanged. Runtime and seed contain 165 sets / 550 image records.
- `python3 scripts/verify-seed.py data/source.csv data/seed.sqlite.next` passes: all CSV cells, JPEG MIME/magic/hash, SQLite integrity, foreign keys and maximum 5 images per set.
- Existing provider pacing and cooldowns retained; no new provider 429/403. No ingestion batch remains running.
- Read back new image IDs 241,400,550 through `/api/images/:id`: JPEG MIME/magic and SHA-256 match the seed. Live catalogue API confirms 165 sets / 550 images.

## Back-office import preview and scan — 7 October 2026

- Regression tests confirm dry-run imports roll back new rows, metadata and image invalidation; HTTP preview is authenticated, returns the exact CSV snapshot and writes no rows. Repeat scans queue only new or changed public examples and count unchanged rows correctly.
- `go test -race ./...` and `go vet ./...` pass. Back-office Svelte check/build pass; all 9 Node tests pass, including import-before-extraction, reviewed snapshot submission, empty scan, pause retention, stop and import failure.
- Browser driving the actual UI against an isolated HTTP fixture: preview and Cancel make zero commit/extract calls; OK Update makes exactly one commit using the reviewed CSV, then two image-extract calls with target=5. Status reports 2 ready / 0 unavailable.
- Real Google Sheet preview: 8 new,2 changed,163 unchanged,8 image candidates. No real commit clicked; live gallery retains 165 sets / 550 images. Desktop screenshot visually inspected. Browser viewport override had no effect on this IAB session, so no mobile result is claimed; override reset and fixture tabs/processes cleaned up.
- Existing CSV browser acceptance test updated to confirm the new modal before expecting committed import results.

## Shop badges and floating gallery cards — 7 October 2026

- Five behavior tests cover distinct stable shop colors/aliases, hover lift/tilt and return to rest, no frames for touch/coarse pointers/reduced motion, immediate carousel pointer-down reset, cleanup and live reduced-motion changes. All 14 Node tests pass. Frontend Svelte check/build pass.
- Real IAB desktop: shop labels are 12px with different green/blue/pink/purple backgrounds. Settled mouse interaction produces lift -6px and nonzero rotateX/rotateY; leaving removes active hover. Keyboard End/Home, pointer slider click and native drag select images correctly (last-image 4/4), with no accidental detail dialog. Details display the matching source badge. No console errors or horizontal overflow.
- A browser-specific rendering issue was reproduced with preserve-3d and resting perspective, then fixed by flattening card contents and using transform:none at rest. Source labels and slider hit testing now display and work. Narrow/coarse/reduced-motion behavior is covered by CSS and action tests; no live mobile browser result is claimed. Screenshot: test-results/source-badges-floating-card.png.

## Table view and first-visit guide — 7 October 2026

- Preference tests first failed on the absent module, then passed for initial display, explicit opt-out, reversing opt-out and blocked localStorage access. UI inspection before wiring found neither table toggle nor guide. All 17 Node tests pass; frontend Svelte check has zero errors/warnings and the production build succeeds.
- Real IAB desktop with 173 existing sets: guide advances through all three animated examples, opt-out hides it after reload, help reopens it with the checkbox checked, unchecking restores it on reload, and Escape dismisses it. No console errors.
- Artist filter yields three aespa table rows. Title opens the matching Karina detail with five stored images; switching back preserves titles/filter. Table mode survives reload through its URL. Date filtering/grouping yields one 15 Sep 2026 row. Keyboard ArrowRight moves table scrollLeft to 40. Table content is 1150px within an 869px scroll area; page width remains within the viewport.
- Scrolling the table appends 24 -> 48 rows; changing to cards and back retains all 48. Screenshots: test-results/gallery-table-view.png and test-results/gallery-short-guide.png.
- Added Playwright acceptance specifications for guide preference and shared table/card behavior, and opted existing gallery specs out of onboarding. These specs were not executed in this turn; live checks used the required CUA browser. No live mobile result is claimed (IAB viewport overrides did not apply in this session).

Short-guide follow-up: live IAB confirms Choose your view/demo-view first, Find your favorites/demo-filter second, and Explore each set/demo-browse third. No Thai text remains in the dialog; Get started closes it. Frontend check/build and three preference tests pass. Acceptance-spec text/order updated; screenshot: test-results/gallery-short-guide-english.png.

Group badge follow-up: pre-change live inspection found 6px group vs 12px shop, 100px maximum and ellipsis. After the CSS change, full LE SSERAFIM labels render at 12px equal to shop badges, with blue background and bright text; names wrap without ellipsis. Page width stays within viewport and console is clean. Frontend check/build pass. Narrow-screen CSS uses 11px for both badges. Screenshot: test-results/gallery-group-badges.png.
