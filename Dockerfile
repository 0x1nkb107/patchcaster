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
# Both also stage LICENSE and LICENSES/LicenseRef-PatchCaster-Accepted-AGPL.txt
# at the context root: every image carries the licence (ADR-024).
#
# TARGETPLATFORM below is buildx's per-platform injection in the release
# flow; the local flow passes the identical value explicitly. Both resolve
# to exactly one staged file.
#
# Base pinned by digest (manifest list verified against the registry);
# pull the current :nonroot digest before bumping.
ARG BASE_IMAGE=gcr.io/distroless/static-debian13:nonroot@sha256:e2e927ec666bae08560abb3c55d0659eceabb657f56b6782ab500a9fc7f555e3
FROM ${BASE_IMAGE}

ARG TARGETPLATFORM
COPY $TARGETPLATFORM/* /usr/local/bin/
COPY LICENSE LICENSES/LicenseRef-PatchCaster-Accepted-AGPL.txt /usr/share/doc/patchcaster/

EXPOSE 8080 9090
USER nonroot
ENTRYPOINT ["/usr/local/bin/patchcaster"]
CMD ["serve"]
