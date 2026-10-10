# syntax=docker/dockerfile:1.28.0

# Static CGO-free binary, compiled under the project's own Go version
# (SYSARCH-005). GOTOOLCHAIN=local refuses to download a toolchain newer than
# the image's own, so a stale golang tag fails loudly instead of drifting.
ARG GO_VERSION=1.26
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
