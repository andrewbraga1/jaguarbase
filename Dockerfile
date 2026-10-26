# =========================================================================
# STAGE 1: Builder
# =========================================================================
# Use the official Go image based on Debian Bookworm
FROM golang:1.23-bookworm AS builder

ENV GOTOOLCHAIN=auto

# Install C/C++ compilers required for CGO and go-duckdb
RUN apt-get update && apt-get install -y build-essential gcc g++

WORKDIR /app

# Cache optimization: Copy only go.mod and go.sum first.
# If dependencies haven't changed, Docker won't download them again.
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the source code
COPY . .

# Compile the optimized binary
# - CGO_ENABLED=1: Required for DuckDB (C++)
# - GOOS=linux: Forces the target OS to Linux
# - ldflags "-s -w": Strips debug symbols, reducing binary size
RUN CGO_ENABLED=1 GOOS=linux go build -ldflags="-s -w" -o jaguarbase ./cmd/jaguarbase/main.go

# =========================================================================
# STAGE 2: Runner (Edge / Production Image)
# =========================================================================
# Use Debian Bookworm Slim, minimal but with the correct glibc
FROM debian:bookworm-slim

# Install security certificates (for HTTPS requests) and clean apt cache
RUN apt-get update && apt-get install -y ca-certificates tzdata \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Copy the compiled binary from Stage 1
COPY --from=builder /app/jaguarbase /app/jaguarbase

# Create directories for configuration and DuckDB data persistence
RUN mkdir -p /app/config /app/data

# Execution permission
RUN chmod +x /app/jaguarbase

# Expose default application ports
# 8080 : HTTP (MCP JSON-RPC)
# 50051: gRPC (High Performance)
EXPOSE 8080 50051

# Entry point
CMD ["/app/jaguarbase"]