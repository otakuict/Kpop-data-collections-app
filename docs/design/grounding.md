# Grounding

Greenfield: directory contained no files, repository, or previous ownership constraints. Skip subsystem how/why because there is no existing system.

User requires Svelte gallery, Gin API, SQLite metadata AND image storage, Svelte back-office manual creation and Google Sheet import, seed every data row from provided Sheet, extract images from example post links, one GitHub repo with independently scoped FE/BE/back-office CI/CD, Docker Compose local.

Source CSV fetched successfully to /tmp/kpop-source.csv. 169 CSV rows, row 4 header: Date, Name, GROUP, Source, anything, Example, REMARK, then spare columns. Dates like 260915 are YYMMDD. Example posts mostly ganknow.com/post/UUID. Ignore decorative rows above header, preserve raw data rows/remarks, use deterministic import identity and per-row diagnostics. Do not fake images when extraction fails.

Runtime: Node 24/npm available. Go absent. Docker CLI and colima available but daemon stopped. No GitHub remote exists. Prepare workflows without publishing an unrequested repository.

Design artifact must include caller usage, module map, Go/JS signatures and contracts, schema/indexes, extraction/import idempotency/error semantics, authentication, SSRF defenses, CI scope and verification.

Rubric (1–5): requirements coverage, single writer/storage integrity, small coherent interfaces, source fidelity/retry behavior, operational feasibility.
