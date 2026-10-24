# HTTP request and response logs

The backend records every incoming API exchange and every outbound HTTP exchange
made by its TeraBox, TypeSafe, Google Sheets, Finder and media clients.

| Event | Direction | Details |
| --- | --- | --- |
| `http.in.request` | `incoming` | ID, method, URL path, query, headers, request body |
| `http.in.response` | `incoming` | Same ID, route, status, bytes, duration, headers, response body |
| `http.out.request` | `outgoing` | Parent ID, unique call ID, method, destination, query, headers, request body |
| `http.out.response` | `outgoing` | Parent/call IDs, destination, status, duration, headers, response body |
| `http.out.error` | `outgoing` | Parent/call IDs, destination, duration, sanitized transport error |

Incoming responses also expose `X-Request-ID`, so a browser failure can be matched
to its backend logs. All outbound calls triggered by that incoming request retain
its ID. Each outbound request, including a redirect, receives its own `call_id`.
Direct CLI/background requests without an incoming ID get a generated one.

Query, headers and structured bodies appear as escaped JSON within a single log
line. Newlines in content are escaped; they cannot forge additional log entries.
For a TeraBox failure, inspect its `http.out.response` for the actual upstream
status/body and then the matching `http.in.response` for the error returned to BO.

## Redaction and body limits

- Credentials are replaced by `[REDACTED]`: Authorization, Cookie/Set-Cookie,
  API keys, passwords, tokens including `jsToken`/`bdstoken`/`ndus`, signatures,
  OAuth assertions, session data and private keys. Nested JSON and form fields
  are processed too. Signed query parameters inside body/header URLs are masked.
- Known credentials from the current request are also removed if they appear
  in a plain-text response or transport error. Userinfo and URL fragments are
  excluded from logged URLs.
- Text/JSON/form bodies are bounded to **16 KiB**. Oversized bodies are marked
  omitted and `body_truncated=true`; partial JSON is not logged because it
  cannot be redacted reliably. Invalid JSON is also omitted.
- Binary images/video/audio, multipart uploads and event streams show an omitted
  marker instead of payload data. Ordinary gallery images are never dumped into
  logs. Body handling preserves the data for the handler/HTTP caller.

These logs intentionally contain non-secret request/response data, such as folder
names, gallery metadata and artist choices. Use the normal restricted server log
access and retention policy when sharing diagnostics.

To inspect recent exchanges on the K3s deployment:

```sh
sudo k3s kubectl -n bias-archive logs deploy/bias-archive-backend --since=10m
```

The detailed logger requires rebuilding and deploying the backend. Existing Pods
keep their previous logging behavior until the updated image runs.
