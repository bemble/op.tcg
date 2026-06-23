# syntax=docker/dockerfile:1

# ---- Stage 1: build the TypeScript SPA ----
FROM node:22-alpine AS frontend
WORKDIR /app/frontend
COPY frontend/package.json frontend/package-lock.json* ./
RUN npm ci
COPY frontend/ ./
RUN npm run build
# -> /app/frontend/dist

# ---- Stage 2: build the Go server (static, CGO-free) ----
FROM golang:1.25-alpine AS backend
WORKDIR /app/backend
COPY backend/ ./
# Download deps verified against the committed go.sum (deterministic, no
# network mutation of go.mod/go.sum), then build a static binary.
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -mod=readonly -trimpath -ldflags="-s -w" -o /server .

# ---- Stage 3: minimal runtime ----
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata && adduser -D -u 10001 app
WORKDIR /app
COPY --from=backend /server /app/server
COPY --from=frontend /app/frontend/dist /app/web
# No catalogue is bundled: on first run /data/catalogue.json is absent, so the
# startup background sync (SYNC_ON_START) rebuilds it from the official card
# list. Set CATALOGUE_SEED to a bundled JSON if you want offline-first instead.
ENV PORT=8080 \
    DB_PATH=/data/onepiece.db \
    WEB_DIR=/app/web \
    CATALOGUE_PATH=/data/catalogue.json
RUN mkdir -p /data && chown -R app:app /data
USER app
VOLUME ["/data"]
EXPOSE 8080
CMD ["/app/server"]
