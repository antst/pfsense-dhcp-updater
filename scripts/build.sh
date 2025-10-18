#!/bin/bash
# Build script for pfSense DHCP Static Mapping Updater
# Builds static binary with CGO disabled

set -e

# Configuration
BINARY_NAME="pfsense-dhcp-updater"
BUILD_DIR="bin"
VERSION="${VERSION:-dev}"
BUILD_DATE=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
COMMIT_HASH=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")

# Build flags
LDFLAGS="-s -w -X main.Version=${VERSION} -X main.BuildDate=${BUILD_DATE} -X main.CommitHash=${COMMIT_HASH}"

echo "Building ${BINARY_NAME}..."
echo "Version: ${VERSION}"
echo "Build Date: ${BUILD_DATE}"
echo "Commit: ${COMMIT_HASH}"

# Create build directory
mkdir -p "${BUILD_DIR}"

# Build static binary (CGO disabled for portability)
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags "${LDFLAGS}" \
    -o "${BUILD_DIR}/${BINARY_NAME}" \
    ./cmd/pfsense-dhcp-updater

echo "Build complete: ${BUILD_DIR}/${BINARY_NAME}"

# Display binary size
ls -lh "${BUILD_DIR}/${BINARY_NAME}"

# Verify it's static
if command -v ldd &> /dev/null; then
    echo "Checking if binary is statically linked..."
    if ldd "${BUILD_DIR}/${BINARY_NAME}" 2>&1 | grep -q "not a dynamic executable"; then
        echo "✓ Binary is statically linked"
    else
        echo "⚠ Binary appears to have dynamic dependencies:"
        ldd "${BUILD_DIR}/${BINARY_NAME}" || true
    fi
fi
