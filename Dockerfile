# Stage 1: build Go binary
# Target arm64 for Raspberry Pi 4B.
FROM golang:1.25-alpine AS backend
WORKDIR /build
COPY server/go.mod server/go.sum ./
RUN go mod download
COPY server/ .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o app .

# Stage 2: minimal runtime
FROM scratch
WORKDIR /app
EXPOSE 8080
COPY --from=backend /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=backend /build/app ./app
# seed.json is baked into the image and used once to populate the SQLite DB on first boot.
# The DB file itself must be on a named volume (set DB_PATH env var in docker-compose).
# Default DB_PATH is ./chore-chart.db — mount a volume there to persist data across restarts.
COPY server/data ./data
ENTRYPOINT ["./app"]
