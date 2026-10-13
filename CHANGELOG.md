# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [v0.1.0](https://github.com/patchcaster-app/patchcaster/releases/tag/v0.1.0) - 2026-10-13

### Added

- Forward GitLab merge request activity into Mattermost: each merge request gets one thread whose root post is a live status board for its pipeline, reviewers, comments and edits.
- Reply in the thread for comments, pipeline results, review verdicts, pushes with their count of new commits, merges and closes.
- Route projects to channels with a routing table. Several projects may share a channel, and unmapped projects are never posted.
- Accept only webhooks with a valid Standard Webhooks signature, with several signing secrets active at once for rotation, and suppress duplicate and stale deliveries.
- Deliver through a durable PostgreSQL outbox: failed posts are retried with backoff, survive restarts, and are parked for triage when the retries run out.
- Serve liveness, readiness and Prometheus metrics on an internal listener, and drain in-flight work on shutdown.
- Publish static Linux binaries and multi-architecture container images for amd64 and arm64, signed keylessly with cosign.
