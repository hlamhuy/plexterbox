# ── Stage 1: build the React frontend ────────────────────────────────────────
FROM node:22-alpine AS ui-builder

WORKDIR /app/web
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
RUN npm run build
# Output lands at /app/cmd/server/dist (outDir: '../cmd/server/dist' in vite.config.ts)


# ── Stage 2: build the Go binary ─────────────────────────────────────────────
FROM golang:1.25-alpine AS go-builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
# Copy the built frontend into the embed directory
COPY --from=ui-builder /app/cmd/server/dist ./cmd/server/dist

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o plexterbox ./cmd/server/


# ── Stage 3: minimal runtime image ───────────────────────────────────────────
FROM alpine:latest

# ca-certificates: needed for outbound HTTPS (Plex, Letterboxd APIs)
# tzdata: enables TZ env var for user-configurable timezone
RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app
COPY --from=go-builder /app/plexterbox .

# DATA_DIR tells the app where to store session.json and plexterbox.db.
# Mount a volume here to persist data across container restarts/updates.
ENV DATA_DIR=/config
VOLUME ["/config"]

EXPOSE 12349

ENTRYPOINT ["./plexterbox"]
