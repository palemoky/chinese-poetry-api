#!/bin/sh
set -e

DB_FILE="poetry-${DB_TYPE}.db"

echo "=== Chinese Poetry API Startup ==="
echo "Database type: ${DB_TYPE}"
echo "GitHub repo: ${GITHUB_REPO}"
echo "Release version: ${RELEASE_VERSION}"

# If database doesn't exist, download from GitHub Release
if [ ! -f "$DB_FILE" ]; then
    echo "Database not found locally, downloading from GitHub Release..."

    if [ -z "$GITHUB_REPO" ]; then
        echo "ERROR: GITHUB_REPO environment variable is not set"
        exit 1
    fi

    DOWNLOAD_URL="https://github.com/${GITHUB_REPO}/releases/download/${RELEASE_VERSION}/poetry-${DB_TYPE}.db.gz"

    echo "Downloading from: $DOWNLOAD_URL"

    if ! curl -L -f -o "${DB_FILE}.gz" "$DOWNLOAD_URL"; then
        echo "ERROR: Failed to download database"
        echo "Please check:"
        echo "  1. GITHUB_REPO is correct: ${GITHUB_REPO}"
        echo "  2. RELEASE_VERSION exists: ${RELEASE_VERSION}"
        echo "  3. Database file exists in release: poetry-${DB_TYPE}.db.gz"
        exit 1
    fi

    echo "Extracting database..."
    gunzip "${DB_FILE}.gz"

    echo "Database ready!"
else
    echo "Database found: $DB_FILE"
fi

# Verify database exists
if [ ! -f "$DB_FILE" ]; then
    echo "ERROR: Database file not found after download"
    exit 1
fi

echo "Starting API server..."
exec ./server
