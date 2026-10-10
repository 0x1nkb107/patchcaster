# Static CGO-free binary, compiled under the project's own Go version
# (SYSARCH-005). No tools/ modules participate in the runtime image.
FROM docker.io/library/golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 GOBIN=/out go build -trimpath -ldflags="-s -w" -o /out/patchcaster ./cmd/patchcaster

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/patchcaster /usr/local/bin/patchcaster
EXPOSE 8080 9090
USER nonroot
ENTRYPOINT ["/usr/local/bin/patchcaster"]
CMD ["serve"]
