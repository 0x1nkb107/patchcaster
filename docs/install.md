# Installing PatchCaster

PatchCaster ships as a Helm chart, a container image and a static Linux
binary; all three run the same program. Pick one, verify it, configure it,
then point GitLab at it.

## What you need

* **PostgreSQL 18** that PatchCaster can reach. It holds the delivery queue,
  thread mappings and dedupe records; PatchCaster creates its own schema.
* **A [Mattermost bot account](integrations/mattermost.md)** and its token.
  The bot must be a member of every channel it posts to.
* **GitLab 19.0 or later**, able to reach PatchCaster with
  [signed webhooks](integrations/gitlab.md).
* **A routing table** naming the channel for each project
  ([format](configuration.md#routing-table)).

## Kubernetes: Helm chart

The chart is `oci://ghcr.io/patchcaster-app/charts/patchcaster`, built in the
sibling [`patchcaster-app/charts`](https://github.com/patchcaster-app/charts)
repository (SYSREQ-033, ADR-017). Its README is the reference for installing
it, its values, and verifying it. Chart versions follow releases
(`appVersion` = tag) and sign under *that* repository's workflow identity, so
the verify regexp differs from the one below — its README carries the exact
command.

## Container image

Release images for linux/amd64 and linux/arm64 live at
`ghcr.io/patchcaster-app/patchcaster`, tagged with the release version and
`latest`. The image is distroless, runs as a non-root user, and serves on
`8080` (webhooks) and `9090` (probes and metrics). All configuration comes
from the environment; the [configuration reference](configuration.md) lists
every variable:

```sh
podman run -d --name patchcaster \
  -p 8080:8080 -p 9090:9090 \
  -v "$PWD/routing.yaml:/etc/patchcaster/routing.yaml:ro" \
  -e PATCHCASTER_ROUTING_FILE=/etc/patchcaster/routing.yaml \
  -e PATCHCASTER_DATABASE_URL='postgres://patchcaster:…@db.example.com:5432/patchcaster?sslmode=require' \
  -e PATCHCASTER_MATTERMOST_URL=https://mattermost.example.com \
  -e PATCHCASTER_MATTERMOST_TOKEN=… \
  -e PATCHCASTER_SIGNING_SECRETS=… \
  ghcr.io/patchcaster-app/patchcaster:vX.Y.Z
```

## Release binaries

Each [GitHub release](https://github.com/patchcaster-app/patchcaster/releases)
carries `patchcaster_<version>_linux_amd64.tar.gz` and
`patchcaster_<version>_linux_arm64.tar.gz`. The binary is static and needs no
runtime libraries; run it under your service manager with the
[configuration](configuration.md) as flags or environment.

## Verify what you deploy

Release images, and the checksum file covering the archives, are signed
keylessly with cosign by the release workflow (ADR-016). Verify an image
before deploying it:

```sh
cosign verify ghcr.io/patchcaster-app/patchcaster:vX.Y.Z \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  --certificate-identity-regexp 'https://github.com/patchcaster-app/patchcaster/\.github/workflows/release\.yml@refs/tags/v[0-9].*'
```

The GitHub release carries `checksums.txt.sigstore.json`, a Sigstore bundle
proving the archives match the signed digests:

```sh
cosign verify-blob --bundle checksums.txt.sigstore.json \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  --certificate-identity-regexp 'https://github.com/patchcaster-app/patchcaster/\.github/workflows/release\.yml@refs/tags/v[0-9].*' \
  checksums.txt
sha256sum --ignore-missing -c checksums.txt
```

## First start

On startup PatchCaster loads the routing table, connects to PostgreSQL,
applies any pending migrations, then listens. Once `GET :9090/readyz`
answers 200, [add the GitLab webhook](integrations/gitlab.md) and open a
merge request in a routed project; its thread should appear in the mapped
channel.

From here on, [operations](operations.md) covers probes, metrics, shutdown,
upgrades and what to do with parked messages.
