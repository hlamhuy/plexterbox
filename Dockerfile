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

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o plexterboxd ./cmd/server/


# ── Stage 3: minimal runtime image ───────────────────────────────────────────
FROM alpine:3.21

# ca-certificates: needed for outbound HTTPS (Plex, Letterboxd APIs)
RUN apk add --no-cache ca-certificates

WORKDIR /app
COPY --from=go-builder /app/plexterboxd .

# Data dir: mount a volume here to persist session.json and plexterboxd.db
# across container restarts.
VOLUME ["/root/.config/plexterboxd"]

EXPOSE 12349

ENTRYPOINT ["./plexterboxd"]
