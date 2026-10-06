FROM golang:1.25-bookworm AS build
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o /server ./cmd/server

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl && rm -rf /var/lib/apt/lists/* && useradd --uid 10001 --create-home gallery && mkdir -p /data /seed && chown gallery:gallery /data
COPY --from=build /server /usr/local/bin/gallery
COPY data/seed.sqlite /seed/seed.sqlite
COPY data/source.csv /seed/source.csv
USER gallery
ENV DATABASE_PATH=/data/gallery.sqlite SEED_DATABASE=/seed/seed.sqlite GIN_MODE=release
EXPOSE 8080
HEALTHCHECK --interval=15s --timeout=5s --start-period=10s CMD curl -fsS http://127.0.0.1:8080/api/health || exit 1
ENTRYPOINT ["gallery"]
