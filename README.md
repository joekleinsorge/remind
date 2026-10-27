# Remind

A personal Kindle highlight library with a web interface, spaced repetition, and optional daily email. One Go binary serves the UI and API, stores imports and reviews on disk, and schedules SendGrid digests. No frontend build step or separate database is required.

## Run locally

```sh
go run . --serve=127.0.0.1:8080
```

Open http://localhost:8080. Existing `clippings.txt` highlights appear automatically. Import **My Clippings.txt** from your Kindle's `documents` directory to add more. Reimporting merges new highlights without resetting reviews.

- **Rediscover:** browse a few passages from across your books. **Next highlight** moves on and brings the passage back later (after 7, 30, 90, then 180 days on subsequent visits). **Show again soon** brings it back tomorrow without advancing that interval. **Skip** moves on without changing the schedule. **Undo** reverses the latest action in this browser session; changing that highlight elsewhere can invalidate Undo. There is no recall test or memorization rating.
- **Your library:** full-text search (including personal notes), book and due filters, favorites, archive, and JSON export of highlights and review state. Archived highlights stay in your library but leave rediscovery and email selection. Favorites receive priority among passages with the same review or delivery count.
- **Daily email:** preview the next digest without sending or changing review state. Delivery status reflects the server's configuration.

The web app has no built-in login. Bind it to loopback locally. The Kubernetes deployment binds the app to pod loopback and exposes only the Dex-authenticated OAuth2 Proxy sidecar, restricted to `freshlab-admins`.

## Persistent storage and email

```sh
go run . --serve=127.0.0.1:8080 \
  --state=/path/to/data/remind-state.json \
  --clippings=/path/to/My-Clippings.txt
```

The state directory contains `library.json`, `remind-state.json`, `email-day.json`, and `delivery-status.json`. Writes use atomic replacement. Only one process may write this directory. Keep the old email CronJob suspended when running the web app.

To enable automatic email, supply `SENDGRID_API_KEY`, `SENDER_EMAIL`, `RECIPIENT_EMAIL`, and `REMIND_EMAIL_TIME=05:00`. `REMIND_TIMEZONE` defaults to `America/New_York`. Without a schedule, the app runs without SendGrid credentials. An invalid schedule or incomplete email configuration fails startup.

The server attempts a digest once per local calendar day after the configured time. A persisted attempt marker prevents duplicate deliveries on restart, including after ambiguous delivery errors. Failures are logged and are not retried that day. Successful delivery records a separate email count and acceptance time; it does not count as a review or postpone a passage. Email selection rotates across due passages using delivery counts. Only explicit reviews advance the review intervals. Existing review schedules are preserved. No email is sent when no highlights are due.

The **Daily email** tab shows the latest recorded outcome and last provider acceptance time. `GET /api/status` exposes the same persisted status with attempted highlight IDs, completion time, and the last successful IDs. Status distinguishes failures, incomplete attempts, and a provider acceptance followed by a delivery-history write failure. Acceptance means SendGrid accepted the request; it does not confirm arrival in the inbox. Previews leave this status unchanged.

JSON configuration remains supported:

```json
{
  "clippings_path": "/data/clippings.txt",
  "count": 5,
  "state_path": "/data/remind-state.json",
  "template_path": "/app/email.tmpl",
  "review_intervals_days": [7, 30, 90, 180]
}
```

## Build and Freshlab deployment

```sh
docker build -t remind .
docker run --rm -p 127.0.0.1:8080:8080 \
  -v remind-data:/data remind \
  --serve=:8080 --state=/data/remind-state.json
```

The `Publish Image` workflow tests the application, builds an amd64 image, publishes it to `ghcr.io/joekleinsorge/remind`, and attaches a **freshlab-remind** artifact. The artifact contains Kustomize manifests pinned to the published image's SHA256 digest, ready for Freshlab's `apps/remind` directory. The unpinned `k8s` manifests are a development template; deploy the pinned artifact.

Freshlab integration uses its existing Istio Gateway, wildcard TLS, external DNS, Dex, and retained Longhorn storage. See [deployment instructions](docs/freshlab.md) for the OIDC/secret prerequisites, activation, and recovery. The intended URL is `https://remind.kleinsorge.dev`.

An optional [External Secrets overlay](k8s/external-secrets/README.md) manages both SendGrid and SSO credentials through an existing secret store. The base deployment continues to support Freshlab's SOPS-managed secrets.

`renovate.json` configures container digest pinning and updates for Dockerfiles, Kubernetes, Kustomize, and GitHub Actions. Enable the Renovate GitHub App for this repository to receive update PRs. Updates require review; automatic merging is disabled. Deployment bundles always use the digest returned by the publishing build.

## Import from the command line

Import through a running local server:

```sh
go run . --import="My Clippings.txt"
# For a different local port:
go run . --import="My Clippings.txt" --import-url=http://127.0.0.1:8081/api/import
```

For Kubernetes, stream the file directly to the running app:

```sh
scripts/import-clippings.sh "My Clippings.txt"
# Optional namespace argument:
scripts/import-clippings.sh "My Clippings.txt" remind
```

The script needs Kubernetes permission to exec into the Remind pod. It uses the binary's `--import=-` mode and the pod's loopback API, so authentication at the public endpoint remains unchanged. Imports add new highlights, report duplicate counts, and preserve reviews. Files up to 10 MB are accepted; invalid imports leave the library unchanged. Import results report new highlights, duplicates (including repeats inside the file), malformed records, and unsupported notes or bookmarks. No upload pod or persistent-volume copy is needed.


## Restore a backup

In **Your library**, choose **Restore backup** and select a version 1 Remind JSON export (up to 50 MB), then choose **Merge backup into library**. Missing highlights and review records are added; existing highlights, review history, notes, favorites, and archive choices are preserved. Export includes all highlights, including archived ones. Invalid exports are rejected before any changes are written.

Restore uses a temporary `restore-pending.json` journal in the state directory so interruption between library and review writes can be recovered. If storage fails during restoration, further requests are blocked; fix the storage issue and restart Remind to complete recovery. The journal is removed after success. Back up the full data directory for complete recovery of delivery status as well as your collection. Undo history is temporary and resets on restart or restore.

## Existing command-line workflows

The email CLI remains available when `--serve` is omitted:

```sh
go run . --preview preview.html
go run . --dry-run
go run . --config remind.json
```

Do not run a sending CLI against the web app's active state volume. Dry runs and previews do not update review state.

## Validation

```sh
gofmt -l .
go mod verify
go vet ./...
go test -race ./...
go build ./...
node --check web/app.js
kubectl kustomize k8s
```
