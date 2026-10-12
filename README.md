<div align="center">

# PatchCaster

**From forge to chat — one live thread per merge request.**

[![CI](https://github.com/patchcaster-app/patchcaster/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/patchcaster-app/patchcaster/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/patchcaster-app/patchcaster?sort=semver)](https://github.com/patchcaster-app/patchcaster/releases)
[![Go](https://img.shields.io/github/go-mod/go-version/patchcaster-app/patchcaster)](go.mod)
[![License: AGPL-3.0-only](https://img.shields.io/badge/license-AGPL--3.0--only-blue.svg)](LICENSE)

[Install](docs/install.md) ·
[Configuration](docs/configuration.md) ·
[Operations](docs/operations.md) ·
[Development](docs/development.md)

</div>

Forwards GitLab (19.0+) merge request activity into Mattermost channels —
one thread per merge request whose root message is a live status board
(pipeline, reviewer, comment and edit segments refresh in place), with thread
replies for notable events: comments, pipeline pass/fail, review verdicts,
pushes, merge and close.

## Getting started

1. [Create a Mattermost bot account](docs/integrations/mattermost.md) and
   add it to every channel it should post to.
2. [Install](docs/install.md) PatchCaster next to a PostgreSQL 18 database,
   with a [routing table](docs/configuration.md#routing-table) naming the
   channel for each project.
3. [Add the GitLab webhook](docs/integrations/gitlab.md) and enable its
   triggers.

Just want to watch it work? The [local demo](demo/README.md) runs without
GitLab, Mattermost or network access.

## Documentation

| Guide | Covers |
| --- | --- |
| [Install](docs/install.md) | prerequisites, Helm chart, container image, binaries, verifying releases |
| [Configuration](docs/configuration.md) | every flag and environment variable, the routing table |
| [GitLab](docs/integrations/gitlab.md) | webhook, signing token, required triggers |
| [Mattermost](docs/integrations/mattermost.md) | bot account, channel membership, channel ids |
| [Operations](docs/operations.md) | probes, metrics, drain, upgrades, delivery guarantees |
| [Changelog](CHANGELOG.md) | what changed in each release |
| [Development](docs/development.md) | building, testing, sending changes upstream |

## License

PatchCaster is free software under the [GNU Affero General Public License, version 3](LICENSE). Later versions of that licence apply once the project's licence proxy has accepted them, as the [proxy grant](LICENSES/LicenseRef-PatchCaster-Accepted-AGPL.txt) sets out. If you modify PatchCaster and let others interact with it over a network, the licence requires you to offer them the source of your modified version.
