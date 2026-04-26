# Stage 1: Build Vue frontend
FROM docker.m.daocloud.io/library/node:20-alpine AS frontend
WORKDIR /app/frontend
COPY frontend/package.json frontend/package-lock.json* ./
RUN rm -f package-lock.json && npm config set registry https://registry.npmmirror.com && npm install
COPY frontend/ .
# vite.config.js outputs to ../web/dist → /app/web/dist
RUN npm run build

# Stage 2: Build Go binary (includes go:embed of web/dist/)
FROM docker.m.daocloud.io/library/golang:1.24-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN GOPROXY=https://goproxy.cn,direct go mod download
COPY . .
COPY --from=frontend /app/web/dist ./web/dist
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /music-lib ./cmd/server

# Stage 3: Minimal runtime image
FROM docker.m.daocloud.io/library/alpine:3.20
RUN sed -i 's|https://dl-cdn.alpinelinux.org|https://mirrors.aliyun.com|g' /etc/apk/repositories && \
    apk --no-cache add ca-certificates tzdata
COPY --from=builder /music-lib /usr/local/bin/music-lib
EXPOSE 35280
VOLUME ["/music", "/data"]
ENV PORT=35280 GIN_MODE=release
ENTRYPOINT ["music-lib"]
