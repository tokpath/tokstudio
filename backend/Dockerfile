# syntax=docker/dockerfile:1
FROM golang:1.27-alpine AS toolchain
RUN apk add --no-cache gcc musl-dev

FROM toolchain AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    sh scripts/build-release.sh /out

FROM postgres:16-alpine AS backup
COPY --from=build /out/backup /usr/local/bin/tokenhub-backup

FROM gcr.io/distroless/static-debian12 AS runtime
COPY --from=build /out/api /api
COPY --from=build /out/worker /worker
COPY --from=build /out/migrate /migrate
COPY --from=build /out/seed-demo /seed-demo
USER nonroot
ENTRYPOINT ["/api"]
