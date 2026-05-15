# syntax=docker/dockerfile:1

# ----------------------------------------------------------------------------
# Stage A — build the React/Vite frontend (needs Node 20+)
# ----------------------------------------------------------------------------
FROM node:20 AS web

WORKDIR /web

# Copy manifests first so `npm ci` is cached unless deps change.
COPY web/package.json web/package-lock.json ./
RUN npm ci

# Now copy the rest of the frontend source and build it.
COPY web/ ./
RUN npm run build

# ----------------------------------------------------------------------------
# Stage B — build a fully static Go binary (CGO off, pure-Go SQLite driver)
# ----------------------------------------------------------------------------
FROM golang:1.25 AS build

WORKDIR /src

# Cache module downloads: only re-run when go.mod/go.sum change.
COPY go.mod go.sum ./
RUN go mod download

# Copy the Go source.
COPY cmd/ ./cmd/
COPY internal/ ./internal/

# Static build — no C deps thanks to modernc.org/sqlite.
ENV CGO_ENABLED=0 GOOS=linux
RUN go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

# ----------------------------------------------------------------------------
# Stage C — minimal runtime image (distroless, nonroot, static binary)
# ----------------------------------------------------------------------------
FROM gcr.io/distroless/static:nonroot

WORKDIR /app

# The binary serves the SPA from "web/dist" relative to its working directory.
COPY --from=build /out/server /app/server
COPY --from=web /web/dist /app/web/dist

EXPOSE 8080

USER nonroot:nonroot

ENTRYPOINT ["/app/server"]
