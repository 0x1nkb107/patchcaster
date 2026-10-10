# syntax=docker/dockerfile:1.28.0

# Static CGO-free binary, compiled under the project's own Go version
# (SYSARCH-005). The default pairs exactly with go.mod's toolchain line so the
# dev image, CI, and release binaries share one compiler. GOTOOLCHAIN=local
# refuses downloads, so an override older than go.mod fails loudly instead of
# drifting silently.
ARG GO_VERSION=1.27.1
ARG DEBIAN_VERSION=12

FROM docker.io/library/golang:${GO_VERSION} AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

# The deny-by-default .dockerignore makes this context exactly the module
# files plus cmd/ and internal/; tools/ modules and docs never reach the build.
COPY . .

RUN CGO_ENABLED=0 GOTOOLCHAIN=local \
    go build -mod=readonly -trimpath -ldflags="-s -w" -o /out/patchcaster "./cmd/patchcaster"

FROM gcr.io/distroless/static-debian${DEBIAN_VERSION}:nonroot
COPY --from=build /out/patchcaster /usr/local/bin/patchcaster
EXPOSE 8080 9090
USER nonroot
ENTRYPOINT ["/usr/local/bin/patchcaster"]
CMD ["serve"]
