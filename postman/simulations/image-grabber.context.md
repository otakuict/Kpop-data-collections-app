# Image grabber simulation

Created 2026-10-26 from gank-image-grabber's README and live extraction responses. The Postman owner-workspace search was skipped because the CLI is signed out. This is a local contract fixture; nothing is published.

## Purpose

Exercise the real Go internal HTTP client, image selection, and handling of the grabber's 422 and 500 responses. This simulation does not download CDN images or run Kubernetes DNS.

## Execution

From the gallery repository, start:

```sh
postman simulation run postman/simulations/image-grabber.sim.yaml --port 4910 --output ndjson --no-history
```

After its `listening` event, in another terminal:

```sh
cd backend
GALLERY_GRABBER_SIM_URL=http://127.0.0.1:4910/grabber go test ./internal/gallery -run '^TestImageGrabberSimulation$' -count=1 -v
```

Stop the foreground simulation with Ctrl-C. Temporary verification logs for the initial run were `/tmp/gallery-grabber-simulation.ndjson` and `/tmp/gallery-grabber-sim-tests.log`.

## Assumptions

- 2026-10-26: API root paths are preserved; `/grabber` routes to the mock and the client appends `/api/extract`. Source identity, image type and URL fields follow the owner's README and observed live JSON.
- 2026-10-26: 422 means no images and 500 means extraction failed, as defined by the owner. CDN HTTPS validation, image bytes and storage are checked separately by backend tests and explicit live smoke tests.

## Observations

<!-- begin:observations -->
2026-10-26: the single image-grabber member received all three GET requests and returned 200, 422 and 500. The Go simulation test passed. Separate live tests retrieved real Google previews through the public gateway; the internal cluster route requires deployment verification.
<!-- end:observations -->

## What didn't work

- 2026-10-26: CLI 1.75.0 rejected a directory in the simulation mock path; use the explicit config.yaml path. Local server execution requires loopback socket permission in the Codex sandbox.
