# ---- Build Stage ----
FROM golang:1.21-bookworm AS builder

WORKDIR /src
COPY go.mod ./
COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /app/music-lib-server ./cmd/server

# ---- Production Stage ----
FROM python:3.11-slim-bookworm

RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates tzdata \
    && rm -rf /var/lib/apt/lists/*
ENV TZ=Asia/Shanghai

RUN pip install --no-cache-dir playwright \
    && playwright install chromium \
    && playwright install-deps chromium

WORKDIR /app
COPY --from=builder /app/music-lib-server /usr/local/bin/music-lib-server
COPY web/ /app/web/
COPY scripts/login_helper.py /app/scripts/login_helper.py

EXPOSE 35280
ENV PORT=35280
ENV CONFIG_DIR=/app/config
ENV MUSIC_DIR=""
ENV DOWNLOAD_CONCURRENCY=3
ENV LOGIN_SCRIPT=/app/scripts/login_helper.py

VOLUME ["/mnt/music"]

ENTRYPOINT ["music-lib-server"]
