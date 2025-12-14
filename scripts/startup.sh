#!/bin/sh
set -e

# Configuration
DATA_DIR="data"
DB_FILE="poetry.db"
DB_PATH="${DATA_DIR}/${DB_FILE}"
DB_GZ="${DB_PATH}.gz"
CHECKSUM_FILE="${DATA_DIR}/checksums.txt"
GITHUB_RELEASE_URL="https://github.com/palemoky/chinese-poetry-api/releases/latest/download"

echo "=== Chinese Poetry API Startup ==="

# Create data directory if it doesn't exist
mkdir -p "${DATA_DIR}"

# Function to download and verify database
download_database() {
    echo "Downloading database and checksums..."

    # Download both files
    if ! curl -Lfso "${DB_GZ}" "${GITHUB_RELEASE_URL}/${DB_FILE}.gz"; then
        echo "ERROR: Failed to download database"
        exit 1
    fi

    if ! curl -Lfso "${CHECKSUM_FILE}" "${GITHUB_RELEASE_URL}/checksums.txt"; then
        echo "ERROR: Failed to download checksums"
        rm -f "${DB_GZ}"
        exit 1
    fi

    # Verify downloaded .gz file
    echo "Verifying download integrity..."
    expected_checksum=$(grep "${DB_FILE}.gz" "${CHECKSUM_FILE}" | awk '{print $1}')

    if [ -z "$expected_checksum" ]; then
        echo "ERROR: Could not find checksum for ${DB_FILE}.gz"
        rm -f "${DB_GZ}" "${CHECKSUM_FILE}"
        exit 1
    fi

    actual_checksum=$(sha256sum "${DB_GZ}" | awk '{print $1}')

    if [ "$actual_checksum" != "$expected_checksum" ]; then
        echo "ERROR: Checksum mismatch!"
        echo "  Expected: $expected_checksum"
        echo "  Actual:   $actual_checksum"
        rm -f "${DB_GZ}" "${CHECKSUM_FILE}"
        exit 1
    fi

    echo "✓ Download verified"

    # Extract database
    echo "Extracting ${DB_FILE}..."
    gunzip -f "${DB_GZ}"

    echo "✓ Database ready: $DB_PATH"
}

# Function to check for updates
check_for_updates() {
    echo "Checking for updates..."

    # Download latest checksums
    temp_checksum=$(mktemp)
    if ! curl -Lfso "$temp_checksum" "${GITHUB_RELEASE_URL}/checksums.txt"; then
        echo "Warning: Could not fetch latest checksums, skipping update check"
        rm -f "$temp_checksum"
        return 1
    fi

    # Compare with local checksums
    if [ -f "$CHECKSUM_FILE" ]; then
        if cmp -s "$temp_checksum" "$CHECKSUM_FILE"; then
            echo "✓ Database is up to date"
            rm -f "$temp_checksum"
            return 0
        else
            echo "→ New database version available"
            # Show what changed
            remote_checksum=$(grep "${DB_FILE}.gz" "$temp_checksum" | awk '{print $1}')
            local_checksum=$(grep "${DB_FILE}.gz" "$CHECKSUM_FILE" | awk '{print $1}')
            echo "  Local:  ${local_checksum:0:16}..."
            echo "  Remote: ${remote_checksum:0:16}..."
        fi
    fi

    rm -f "$temp_checksum"
    return 1
}

# Main logic
if [ -f "$DB_PATH" ] && [ -f "$CHECKSUM_FILE" ]; then
    echo "Database found: $DB_PATH"

    # Check for updates
    if ! check_for_updates; then
        echo "Updating database..."
        download_database
    fi
else
    echo "Database not found, downloading..."
    download_database
fi

echo "Starting API server..."
exec ./server
