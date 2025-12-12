#!/bin/sh
set -e

# Hardcoded data directory (matches docker-compose volume mount)
DATA_DIR="data"
DB_FILE="poetry.db"
DB_PATH="${DATA_DIR}/${DB_FILE}"

echo "=== Chinese Poetry API Startup ==="

# Create data directory if it doesn't exist
mkdir -p "${DATA_DIR}"

# Download unified database if not present
if [ -f "$DB_PATH" ]; then
    echo "Database found: $DB_PATH"
else
    echo "Downloading unified database..."
    url="https://github.com/palemoky/chinese-poetry-api/releases/latest/download/${DB_FILE}.gz"

    if ! curl -L -f -o "${DB_PATH}.gz" "$url"; then
        echo "ERROR: Failed to download database"
        echo "URL: $url"
        exit 1
    fi

    echo "Extracting ${DB_FILE}..."
    gunzip "${DB_PATH}.gz"
    echo "Database ready: $DB_PATH"
fi

echo "Starting API server..."
exec ./server
