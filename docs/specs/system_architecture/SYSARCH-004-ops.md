---
id: "SYSARCH-004"
type: system_architecture
name: "Operability surface"
description: "Health, readiness, metrics, and logging served by the process on an internal listener."
satisfies:
  - "SYSREQ-018"
  - "SYSREQ-019"
  - "SYSREQ-020"
  - "SYSREQ-021"
  - "SYSREQ-023"
---

# System Architecture: Operability surface

Normative: health, readiness, metrics, and logging are served by the process itself on a separate
internal HTTP listener.

## Technical Strategy

A second `http.Server` bound to an internal address serves `/livez`, `/readyz`, and `/metrics`;
the public ingest listener therefore never exposes internals. Metrics use the official
Prometheus client through one internal wrapper package so metric names and the `patchcaster_`
prefix are declared once. Logging is `log/slog` with the tint handler (ADR-004); a per-request
slog attribute carrying `webhook-id` provides SYSREQ-020 correlation without threading an
argument through every call.

## Static View (Structure)

Components: `InternalServer` (probes + metrics), `Readiness` (store writability check), `Metrics`
(registry, the funnel counters of SYSREQ-019, and the wake-up listener gauge of ADR-019), `Logging` (handler setup at `main`). No
component outside this package writes metrics or decides log-handler shape.

## Dynamic View (Behavior)

Startup brings the internal listener up first so probes can observe the ingest listener's
readiness honestly. On `SIGTERM` the public listener closes first, workers drain, then the
internal listener — readiness flaps to `503` at signal time so the Service removes the pod
during the drain window (SYSREQ-023).
