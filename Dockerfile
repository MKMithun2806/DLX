# ---------- Build stage ----------
FROM golang:1.22-bookworm AS builder

WORKDIR /src

COPY go.mod ./
COPY go.sum* ./
ENV GOFLAGS=-mod=mod
RUN go mod download || true

COPY . .

# CGO disabled: modernc.org/sqlite is a pure-Go driver, so we get a fully
# static binary that needs no libc/libsqlite3 at runtime.
ENV CGO_ENABLED=0 GOOS=linux
RUN go mod tidy && go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

# ---------- Runtime stage ----------
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
        ca-certificates \
        curl \
        python3 \
        ffmpeg \
    && curl -L https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp -o /usr/local/bin/yt-dlp \
    && chmod a+rx /usr/local/bin/yt-dlp \
    && apt-get purge -y curl \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=builder /out/server ./server
COPY web ./web

RUN mkdir -p /config /downloads

ENV APP_PORT=8080 \
    DB_PATH=/config/app.db \
    DOWNLOADS_PATH=/downloads \
    YTDLP_PATH=/usr/local/bin/yt-dlp \
    TEMPLATES_DIR=/app/web/templates \
    STATIC_DIR=/app/web/static

EXPOSE 8080
VOLUME ["/config", "/downloads"]

ENTRYPOINT ["/app/server"]
