# Design cross-judge

## Scores (1–5)

| Candidate | Requirements coverage | Single writer / storage | Small interfaces | Source fidelity / retries | Operational feasibility |
|---|---:|---:|---:|---:|---:|
| A | 5 | 5 | 5 | 4 | 5 |
| B | 5 | 5 | 3 | 5 | 3 |

## Recommendation

Use Candidate A as the base, then borrow B's immutable source snapshot and durable per-row extraction outcomes only if implementation shows the synchronous, one-entry extraction endpoint cannot support the required retry flow. A best matches a small first release: the gallery and back office are independently built Svelte apps, one Gin process owns SQLite, and import metadata commits before bounded image work. Its stable URL-based identity handles ordinary row reordering better than B's physical-row identity. B's lease-based worker, polling API, and job tables are coherent but add lifecycle machinery for 165 rows and complicate an initially local deployment whose Docker daemon is currently stopped.

## Red flags / requirements to resolve

- Both designs cover the requested gallery, Gin API, SQLite image bytes, manual entry creation, Sheet import, seed rows, extraction, and separately scoped front-end/back-office/API workflows. Both specify honest missing-image states, auth/CSRF, SSRF controls, and bounded requests.
- A should specify whether the seed's source identifier is the actual Sheet identity and how its checked-in snapshot is refreshed; its proposed URL/name identity cannot perfectly recognize edits that change every identifying field, and occurrence suffixes can shift across duplicate-row edits.
- A returns extraction candidates and asks the browser to sequence per-entry calls. Persist enough attempt state to reconstruct retry work after reload; the text says this, but the report/API behavior should make that explicit. Bound total work per extraction request.
- B's source row number is a fragile durable key when users insert, delete, or reorder Sheet rows. Marking rows absent from a snapshot inactive is only safe when the import is known to be a complete snapshot; the text says complete snapshot, but the API should enforce that distinction.
- B stores `csv_bytes` in SQLite but does not specify retention/cleanup, and its job lease/retry transition rules need a clear terminal state for successful jobs and a clear policy for media-pending lease recovery. Its claim statement names queued plus expired running/media_pending jobs; implementation must prevent a live worker's lease expiring during a long fetch from allowing concurrent duplicate work.
- Neither design establishes the exact source Sheet ID/gid or actual media CDN allowlist. Treat extraction as visibly unavailable until those are grounded in source evidence; do not broaden host access to make it work.
- Grounding notes say 165 data rows and 28 cells where present, while candidate A mentions both. B preserves every CSV cell, but neither should hard-code width: retain trailing and blank cells as actually parsed.

## Decision

Candidate A is the stronger first-release base; transplant only B's snapshot reproducibility if external Sheet mutation during retries proves to be a real operational problem.
