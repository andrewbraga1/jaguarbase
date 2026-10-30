#!/bin/bash
set -e

# Ensure Go binaries (like govulncheck) are in PATH
export PATH="$PATH:$(go env GOPATH)/bin"

LOG_DIR="sec_audit_logs"
SUMMARY_FILE="$LOG_DIR/audit_summary.log"

echo "🛡️ Starting Security Audit for JaguarBase..."

# 1. Create and clean the log directory
mkdir -p "$LOG_DIR"
rm -f "$LOG_DIR"/*

# 2. Initialize the aggregated summary file
echo "========================================" > "$SUMMARY_FILE"
echo " JAGUARBASE SECURITY AUDIT SUMMARY" >> "$SUMMARY_FILE"
echo " Date: $(date)" >> "$SUMMARY_FILE"
echo "========================================" >> "$SUMMARY_FILE"

# 3. Trivy: Docker Image Vulnerability Scan (--quiet + persistent DB cache)
echo "⏳ [1/3] Running Trivy (Docker Image Scan)..."
echo -e "\n\n--- TRIVY SCAN RESULTS ---" >> "$SUMMARY_FILE"
docker run --rm \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v trivy-cache:/root/.cache/ \
  aquasec/trivy image --quiet --severity HIGH,CRITICAL andrewbraga1/jaguarbase:latest > "$LOG_DIR/trivy.log" 2>&1 || true
cat "$LOG_DIR/trivy.log" >> "$SUMMARY_FILE"

# 4. GoSec: Static Application Security Testing (-quiet + quoted pwd)
echo "⏳ [2/3] Running GoSec (SAST)..."
echo -e "\n\n--- GOSEC SAST RESULTS ---" >> "$SUMMARY_FILE"
docker run --rm \
  -v "$(pwd):/app" \
  -w /app \
  securego/gosec:latest -quiet ./... > "$LOG_DIR/gosec.log" 2>&1 || true
cat "$LOG_DIR/gosec.log" >> "$SUMMARY_FILE"

# 5. Govulncheck: Go Dependency Vulnerability Check
echo "⏳ [3/3] Running Govulncheck (Dependencies)..."
echo -e "\n\n--- GOVULNCHECK RESULTS ---" >> "$SUMMARY_FILE"
govulncheck ./... > "$LOG_DIR/govulncheck.log" 2>&1 || true
cat "$LOG_DIR/govulncheck.log" >> "$SUMMARY_FILE"

echo "✅ Audit complete!"
echo "📂 Individual logs and aggregated summary saved in the '$LOG_DIR/' directory."