# Find example links in the backoffice

When adding an image set, fill in the photographed date, name, group, and Source.
In **Find an example link**, check the Gank seller nickname (initially copied from
Source), review or edit **Search date (YYMMDD)** and **Search name preview**,
optionally enter the file count, and click **Find example links**. The preview
splits a date-prefixed Name using Finder’s existing paste logic. You can also
paste `261018 sullyoon` into either search field to populate both. Editing the
search preview does not change the set metadata saved by the main form.
The nickname must be the seller's Gank account name, not a display name or URL.

The backoffice shows matching public post and service URLs with their titles,
excerpts, status, and finder match explanation. TypeSafe marks its recommendation
and confidence. Click **Use this link** on any result to fill Example; you may
choose a different result from the recommendation. Then **Save image set** uses
the existing metadata, Google Sheet writeback, and image extraction flow.
Searching and choosing do not save a set or download images. Manual URL entry
continues to work. Changing the search filters cancels pending searches and
clears the suggestions. Closing the modal also cancels the request.

## Backend configuration

| Environment variable | Purpose |
| --- | --- |
| `GANK_FINDER_URL` | Finder origin, without `/api`, e.g. `http://127.0.0.1:3001` for a native backend. Empty disables lookup. |
| `TYPESAFE_API_KEY` | Server-only TypeSafe bearer key. Empty enables manual selection without AI advice. |
| `TYPESAFE_MODEL` | TypeSafe model alias; defaults to `jev-latest`. |

For local Docker Compose, set these in the ignored `.env` file and recreate the
backend. The Compose files pass them only to the backend. The supplied development
key is stored in local `.env`; it is excluded from Git and Docker build contexts.
Do not put it in frontend `VITE_` variables or source files. Rotate the key by
changing the runtime secret and restarting/recreating the backend.

To connect a Docker gallery backend to the finder published on the same Linux
host, use `GANK_FINDER_URL=http://host.docker.internal:3001`. The Compose files map
this name to the host gateway. The finder must be listening on an interface the
container can reach; a host port bound only to `127.0.0.1` is not reachable through
the gateway. Alternatively connect both containers to a shared Docker network
and use the finder's container name. Do not expose a new public gallery API port.

For the existing direct Docker deployment workflow, set GitHub repository
secret `TYPESAFE_API_KEY`. `GANK_FINDER_URL` defaults to the host gateway on
port 3001 in release Compose; `GANK_FINDER_URL` and `TYPESAFE_MODEL` variables
remain optional overrides. The backend deploy job passes them to Compose. That
script deliberately does not read the checkout's `.env`.

For the current production GitOps deployment, no new GitHub variables are
required. The DevOps runtime PR sets the finder origin to
`http://gank-data-finder.gank.svc.cluster.local:3001` and the model to `jev-latest`.
Before merging, add **`TYPESAFE_API_KEY` to the existing Kubernetes Secret
`bias-archive-admin` in namespace `bias-archive`**, preserving `ADMIN_TOKEN`.
This is a Kubernetes Secret, not a GitHub Actions secret. The optional reference
allows the archive to start and supports manual link selection if the key is
absent.

Merge the DevOps runtime PR and this source PR, then merge the backend, frontend, and
backoffice image update PRs that CI opens in DevOps. Argo CD handles the rollouts.
No server commands, new route, finder deployment, or extra runtime variables are
needed. Set the Secret key before merging so the new Pod reads it. The current
DevOps production overlays already run finder and gallery at one replica each.

## API contract

The gallery exposes authenticated `POST /api/admin/example-candidates`:

```json
{"seller":"loveshakedata","setDate":"2026-08-27","name":"chaewon","group":"LE SSERAFIM","count":2075}
```

If Name starts with a valid `YYMMDD` date followed by a space (for example
`261018 sullyoon`), lookup uses that date and searches the remaining name. This
matches the existing Save convention, even if the selected Set date differs.
Otherwise it uses the selected Set date and the full name.

The backend translates the photographed date to `YYMMDD` and calls the finder's
existing `GET /api/search` with seller, date, name, count, `source=both`,
`includeInactive=true`, and `mode=broad`. Count is optional and influences finder
ranking; it does not exclude all different counts. Group is context for TypeSafe,
not a finder filter. At least date, name, or count must be supplied. Finder limits
name to 100 characters; longer set names can still be saved using a manual URL.

Each unique HTTPS Gank post/service link becomes a selectable candidate. The
response contains `candidates`, `recommendedUrl`, optional `confidence` and
`warning`, and `truncated`. At most 60 candidate links are shown; the UI reports
truncation so the operator can narrow the query. External payment/shop links
are not image-source candidates.

TypeSafe receives only the query metadata and candidate descriptions, using its
[Choice API](https://docs.typesafe.ai/primitives/choice). It returns an option ID;
the backend validates it against the supplied candidates before recommending a
URL. A `none` choice produces no recommendation. Missing keys, invalid responses,
timeouts, and AI errors leave candidates selectable with a warning. Finder
failures return a retryable error while leaving manual input available. The
backend limits finder calls to 90 seconds, AI advice to 20 seconds, and upstream
responses to 2 MiB, with one search active per backend instance. It forwards
neither the administrator token nor the TypeSafe key to finder.

## Verification

```sh
cd backend
go test -race ./...
go vet ./...
cd ..
npm run check --workspace backoffice
npm run build --workspace backoffice
```

With the backoffice running on port 5184, the isolated browser tests mock the
internal gallery API and cover selection-before-save, a human overriding AI,
missing AI advice, empty results, errors, and canceled searches on both desktop
and mobile:

```sh
PLAYWRIGHT_CHROME_PATH='/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge' \
  npx playwright test tests/backoffice-finder.spec.js
```

`PLAYWRIGHT_CHROME_PATH` is the existing test config's executable override; it
also accepts Microsoft Edge. No live set is created by these browser tests.

To reproduce the reported `girlskpopphoto` lookup against a real finder, set
`GALLERY_FINDER_LIVE_URL` to that finder's origin and run
`go test ./internal/gallery -run TestExampleFinderLiveReportedBOPayload -v -count=1`
from `backend/`. Set `TYPESAFE_API_KEY` in the process environment to include real
AI advice. This opt-in test uses the original BO date-prefixed-name payload,
creates only a temporary local database, and logs public candidate links. It is
skipped in ordinary CI; a configured TypeSafe key may incur one API request.
