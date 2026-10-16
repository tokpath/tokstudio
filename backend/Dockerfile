FROM golang:1.27-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/api ./cmd/api \
 && CGO_ENABLED=0 GOOS=linux go build -o /out/worker ./cmd/worker \
 && CGO_ENABLED=0 GOOS=linux go build -o /out/migrate ./cmd/migrate \
 && CGO_ENABLED=0 GOOS=linux go build -o /out/backup ./cmd/backup

FROM postgres:16-alpine AS backup
COPY --from=build /out/backup /usr/local/bin/tokenhub-backup

FROM gcr.io/distroless/static-debian12 AS runtime
COPY --from=build /out/api /api
COPY --from=build /out/worker /worker
COPY --from=build /out/migrate /migrate
USER nonroot
ENTRYPOINT ["/api"]
