# The one Dockerfile (ADR-015): pure COPY, so no target binary is ever
# executed and cross-architecture builds need no QEMU — buildx pulls the
# base per platform and copies. Two consumers, one contract:
#
#   - goreleaser release builds (buildx, --platform linux/amd64,linux/arm64)
#     stage the cross-compiled binaries at linux/<arch>/patchcaster;
#   - `just container` compiles locally with the pinned toolchain
#     (SYSARCH-005: go.mod + CGO off) into the same linux/<arch>/ layout and
#     builds the single-architecture image with podman.
#
# TARGETPLATFORM below is buildx's per-platform injection in the release
# flow; the local flow passes the identical value explicitly. Both resolve
# to exactly one staged file.
#
# Base pinned by digest (manifest list verified against the registry);
# pull the current :nonroot digest before bumping.
ARG BASE_IMAGE=gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab
FROM ${BASE_IMAGE}

ARG TARGETPLATFORM
COPY $TARGETPLATFORM/* /usr/local/bin/

EXPOSE 8080 9090
USER nonroot
ENTRYPOINT ["/usr/local/bin/patchcaster"]
CMD ["serve"]
