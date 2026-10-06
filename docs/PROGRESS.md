# Implementation progress

- [x] Ground: empty repository; source has 165 rows and 31 groups.
- [x] Sketch: independent synchronous and durable-job designs, independent cross-judge, synthesis saved.
- [x] Agree: default implementation; no checkpoint requested.
- [x] Implement: Gin/SQLite, CSV/Google import, bounded preview ingestion, Svelte gallery and back-office, independent images/workflows.
- [x] Scrap/reconcile: design contract reconciled; Go race tests/vet, Svelte checks/builds, seed fidelity, three running Docker services, desktop/mobile browser acceptance (6 tests) all pass.

Arena: all phases complete. See design/architecture.md and judge.md.

Final state: local gallery :5173, back-office :5174, API :8080. Initial seed has 165 sets, 31 groups, 29 actual preview BLOBs; 37 source rows have no public URL and 99 failed extraction. Provider rate limiting remains visible and retryable. Independent GitHub verify/publish workflows and release compose prepared; no push requested. Test project/volume removed after acceptance.


6 October update: default Compose now bind-mounts all source directories with Vite HMR and Air. Back-office shows explicit set_id and image-completion counts; gallery cards support carousel arrows/wheel/keyboard/touch. Uploads enforce 2–5 distinct images atomically. Gank pacing, a shared request lease, daily budget and cooldown persist in SQLite. Enrichment added 34 real images and the seed now contains 63 images / 28 complete sets out of 165. Remaining sets need source images or a later extraction after the saved budget pause. Go race/vet, Svelte checks/builds, seed fidelity and 8 browser tests pass. No push was made.


6 October carousel UX update: replaced card left/right buttons and wheel/swipe image switching with a centered bottom drag bar. Mouse and touch drag continuously, release snaps to a photo, and keyboard navigation stays available on the bar. Wheel/trackpad scroll the page normally. Four read-only desktop/mobile gallery tests, the frontend Svelte check and production build pass; real 5-image cards were inspected in both layouts.
