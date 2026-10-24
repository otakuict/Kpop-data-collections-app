# TeraBox private-drive import

The back office has an **Update from TeraBox** button. Connect either of two
TeraBox browser sessions, scan folders added in the past 72 hours, review names
and groups, and confirm selected sets into the existing Google Sheet and SQLite
save flow. Scanning does not create gallery sets or write to Google Sheets.

## Connect accounts

1. Log in to the intended account on `https://www.terabox.com` in your browser.
2. In Developer Tools → Network, open a folder and find its `/api/list` request.
3. In BO → Update from TeraBox → Accounts, choose slot 1 or 2. Enter a label,
   source/seller and absolute root folder path, such as `/` or `/Kpop`.
4. Copy the request's `Cookie` header, `bdstoken` and `jsToken` into the session
   fields and save. Cookie must include `ndus`. Reconnect after session expiry.
5. Repeat for the other account. Choose the account to scan in the dropdown.

Session values are never returned by the API or placed in browser storage.
They are encrypted with AES-GCM in `terabox_accounts`, using a separate key
domain derived from `ADMIN_TOKEN` and the slot ID as authenticated data. Keep
`ADMIN_TOKEN` stable across restarts; changing it requires reconnecting accounts.
Connections are read-only; no TeraBox upload, delete or share mutation is used.

This uses TeraBox's **undocumented browser-session interface**, not the paid
third-party `teraboxapi.com` extraction API or its 100-call trial. Session expiry,
anti-bot checks and provider changes may require reconnecting or adapter updates.
Do not expose the private session in client code, logs or public gallery data.

## Scan and review

- Recursively enumerate container folders under the selected root, paging in
  batches of 100. Date-prefixed set folders are treated as sets; their image
  contents are not traversed or downloaded.
- `server_ctime`, rather than the photo date in the folder title or the last
  modification time, determines whether a folder is inside the past 72 hours.
- Missing creation timestamps produce a visible warning and are excluded.
  A date such as `240404` can therefore still describe a newly added set.
- The scan is bounded to 100 requests, three minutes and 200 recent folders.
  Reaching a limit is explicitly marked as partial. Use a narrower root to
  inspect a larger drive. An upstream failure is an error, not “nothing new”.
- A folder title normally starts with `YYMMDD` and an artist name. Unknown
  names and malformed dates require manual review; no date is silently guessed.
- Edit the final title, group, photographed date, notes and optional Example
  URL. Duplicates and uncertain previous writes cannot be selected.
- Previews expire after one hour and become invalid when account settings change.

Confirm requires active Google Sheet sync. Each selected row uses
`SheetSync.InsertNew` followed by `Store.SaveStyled`, preserving Sheet row
placement, import identity and the usual New ribbon. The Sheet Date column is
the Bangkok date of insertion; the title retains the photographed date. Preview
images can subsequently use the existing upload/extraction tools. The importer
does not expose private download URLs as public Example links.

## Artist catalog and KOROMAN + Jev

`artist_groups` starts from the running gallery's distinct group names, including
its SOLO category. Public profile facts fill `artist_members` only within those
existing groups. Opening the catalog also picks up groups added through the
normal gallery flow. Existing sets and their group labels are not rewritten.

The bundled seed covers the checked-in gallery's **30 group categories and one
SOLO category**, with **215 memberships** including historical members, and four
solo artists already represented in the gallery. This is an archive name lookup,
not a claim about the current active lineup of each group. Every member includes
source provenance; no biographies or profile images are copied. BO's Artist
catalog shows counts, sources and aliases, and allows an operator to add or
correct members without editing existing gallery sets.

KOROMAN 1.0.16 is vendored unchanged under `backend/internal/gallery/assets/`,
with its MIT license. The backend invokes Node.js with JSON on stdin to obtain
phonetic romanization; no remote service is needed for that step. Both backend
Dockerfiles install Node.js. Native development needs `node` on PATH.

Name resolution:

1. Find exact, case-insensitive artist aliases in the backend catalog. Preserve
   title suffixes such as `2` and `premium`; return a unique membership directly.
2. For unmatched or ambiguous names, supply Jev with the folder name/path and
   KOROMAN output, plus **only database artist IDs as choices** and `none`.
3. Accept only a supplied choice with valid confidence of at least 0.85. A
   suggested correction without an exact alias stays unselected for human review.
4. With no key, provider errors, low confidence or `none`, keep the phonetic
   fallback for manual review. Never invent an artist or auto-save a suggestion.

Jev uses the existing server-only `TYPESAFE_API_KEY` and `TYPESAFE_MODEL`
configuration. Only folder metadata is sent to TypeSafe; TeraBox sessions are
not included. Questions are batched in groups of 20 and known aliases avoid
AI calls. Configure these variables on the backend and restart to enable Jev.

Refresh public facts manually, review the resulting seed diff and deploy:

```sh
python3 scripts/refresh-artist-roster.py
# Or refresh one gallery category:
python3 scripts/refresh-artist-roster.py --group SOLO
```

The script only requests fixed public profile URLs. It stores factual names,
aliases and references. It retains prior data for failed group requests.
Source extraction is reviewed before deployment; it is not a live AI ingestion
pipeline. Runtime seeding is idempotent and preserves operator-edited members.

## Duplicate prevention and partial saves

Each confirmed folder has a receipt keyed by `(account_id, file_id)`. A receipt
is reserved before the Sheet write. Repeated confirmation does not resubmit a
saved folder. Matching existing title/group pairs are also skipped, including
sets already entered from another account.

Google Sheets and SQLite do not share a transaction. A timeout or interrupted
save leaves a `pending`/`reconcile` receipt rather than retrying an uncertain
external write. Import the Google Sheet using the existing flow; scanning again
repairs the receipt only when the matching title/group is actually present in
SQLite. Alternatively, **Check Sheet & recover** checks the live Sheet under the
Sheet write lock: a present row is reconciled into SQLite; an absent row is
released for a fresh scan and a separate confirmation. Recovery never replays
the external insertion. Failed Sheet reads retain pending receipts.

## Validation and remaining live checks

Go tests cover account encryption/isolation, pagination and recursive scanning,
creation-time filtering, actual KOROMAN execution, catalog boundaries, Jev choice
validation, preview without gallery writes, confirmation through a simulated
Sheet, repeated clicks and interrupted Sheet writes. Playwright covers account
switching, editing/confirming a preview, disabled Sheet sync and session errors.

On 2026-10-24 a live Jev request with the project's updated TypeSafe key matched
all five public example names against the 215 database memberships:

| Input | KOROMAN | Jev final name | Gallery group |
| --- | --- | --- | --- |
| 신위 | Sinwi | Xinyu | TripleS |
| 쯔위 | Jjeuwi | Tzuyu | TWICE |
| 가을 | Gaeul | Gaeul | IVE |
| 이지우 | Ijiu | Lee Jiwoo | TripleS |
| 김체원 | Gimchewon | Kim Chaewon | LE SSERAFIM |

The spelling correction stays in `review`, requiring explicit row selection.
This small sample establishes a working live integration, not general accuracy.
To repeat deliberately (makes a real, billable request): run
`GALLERY_LIVE_JEV=1 go test ./internal/gallery -run '^TestTeraLiveJevNames$' -v -count=1`
from `backend/` with the TypeSafe configuration in the environment.

A migration check on copies of the checked-in SQLite seed compared every set
and image row after starting the previous HEAD and the updated backend. All
165 sets and 550 images matched; the new database had 31 artist categories and
215 memberships. The source seed was not rewritten.

Live reads from the two private drives still need their browser sessions.
Verify the first scan's `server_ctime` against TeraBox's displayed added time
before relying on the three-day filter for a particular account.

References:

- [TeraBox browser-client file-list implementation](https://github.com/dangkhoa2016/terabox-api-ruby/blob/main/lib/terabox_api/client/files.rb)
- [TypeSafe Choice contract](https://docs.typesafe.ai/primitives/choice)
- [KOROMAN fork](https://github.com/otakuict/koroman)
- [tripleS official profiles](https://www.triples-official.jp/)
- Individual roster references are stored in `assets/artist-roster.json`.
