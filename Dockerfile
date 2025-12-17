# syntax=docker/dockerfile:1.7

# ---- Build stage ----
FROM golang:1.25-alpine AS build

WORKDIR /src

# Needed for fetching modules
RUN apk add --no-cache git ca-certificates

# Cache deps
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY backend ./backend

# Build the API binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/api ./backend/cmd/api

# ---- Run stage ----
FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app

# Copy binary
COPY --from=build /out/api /app/api

# Copy migrations so the backend can run goose migrations at startup
COPY --from=build /src/backend/migrations /app/backend/migrations

# Default listen address (can be overridden via env)
ENV BES_ADDR=":8080"
ENV BES_MIGRATIONS_DIR="/app/backend/migrations"

EXPOSE 8080

USER nonroot:nonroot

ENTRYPOINT ["/app/api"]
