# ---- Build Stage ----
FROM golang:1.21-alpine AS builder

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /src
COPY go.mod ./
COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /app/music-lib-server ./cmd/server

# ---- Production Stage ----
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata
ENV TZ=Asia/Shanghai

WORKDIR /app
COPY --from=builder /app/music-lib-server /usr/local/bin/music-lib-server
COPY web/ /app/web/

EXPOSE 35280
ENV PORT=35280
ENV MUSIC_DIR=""
ENV DOWNLOAD_CONCURRENCY=3

VOLUME ["/mnt/music"]

ENTRYPOINT ["music-lib-server"]
