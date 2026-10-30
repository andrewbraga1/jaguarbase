# =========================================================================
# STAGE 1: Builder
# =========================================================================
# Use the official Go image based on Debian Bookworm
FROM golang:1.23-bookworm AS builder

ENV GOTOOLCHAIN=go1.26.10

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

# Create directories for configuration and DuckDB data persistence
RUN mkdir -p /app/config /app/data && chmod +x /app/jaguarbase

# =========================================================================
# STAGE 2: Runner (Edge / Production Image)
# =========================================================================
# Use Distroless Base NoSSL Debian 12 (nonroot) + C++ runtime libs for DuckDB
FROM gcr.io/distroless/base-nossl-debian12:nonroot

WORKDIR /app

# Copy C++ runtime libraries required by CGO / go-duckdb (without libssl3)
COPY --from=builder /usr/lib/x86_64-linux-gnu/libstdc++.so.6 /usr/lib/x86_64-linux-gnu/libstdc++.so.6
COPY --from=builder /lib/x86_64-linux-gnu/libgcc_s.so.1 /lib/x86_64-linux-gnu/libgcc_s.so.1

# Copy the compiled binary and runtime directories from Stage 1 with nonroot ownership
COPY --from=builder --chown=nonroot:nonroot /app/jaguarbase /app/jaguarbase
COPY --from=builder --chown=nonroot:nonroot /app/config /app/config
COPY --from=builder --chown=nonroot:nonroot /app/data /app/data

USER nonroot:nonroot

# Expose default application ports
# 8080 : HTTP (MCP JSON-RPC)
# 50051: gRPC (High Performance)
EXPOSE 8080 50051

# Entry point
ENTRYPOINT ["/app/jaguarbase"]