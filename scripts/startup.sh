#!/bin/sh
set -e

# Configuration
DATA_DIR="data"
DB_FILE="poetry.db"
DB_PATH="${DATA_DIR}/${DB_FILE}"
DB_GZ="${DB_PATH}.gz"
CHECKSUM_FILE="${DATA_DIR}/checksums.txt"
GITHUB_RELEASE_URL="https://github.com/palemoky/chinese-poetry-api/releases/latest/download"

# Drop root privileges. In the Docker image the container starts as root so the
# data volume can be handed over to the unprivileged user first: volumes created
# by older images are root-owned, and the server needs write access there even
# with a read-only database (SQLite creates the -shm file next to a WAL database).
RUN_AS="poetry"
if [ "$(id -u)" = "0" ] && id "$RUN_AS" >/dev/null 2>&1 && command -v su-exec >/dev/null 2>&1; then
    mkdir -p "${DATA_DIR}"
    chown -R "$RUN_AS:$RUN_AS" "${DATA_DIR}"
    exec su-exec "$RUN_AS" "$0" "$@"
fi

echo "=== Chinese Poetry API Startup ==="

# Create data directory if it doesn't exist
mkdir -p "${DATA_DIR}"

# Download, verify and install the database.
# Everything goes to temporary files first and only replaces the local copy once
# verified, so a failed download never leaves a broken or half-updated data dir.
# Returns non-zero on failure instead of exiting, letting the caller fall back
# to an existing local database.
download_database() {
    echo "Downloading database and checksums..."

    tmp_gz="${DB_GZ}.tmp"
    tmp_checksum="${CHECKSUM_FILE}.tmp"

    if ! curl -Lfo "${tmp_gz}" "${GITHUB_RELEASE_URL}/${DB_FILE}.gz"; then
        echo "ERROR: Failed to download database"
        rm -f "${tmp_gz}"
        return 1
    fi

    if ! curl -Lfo "${tmp_checksum}" "${GITHUB_RELEASE_URL}/checksums.txt"; then
        echo "ERROR: Failed to download checksums"
        rm -f "${tmp_gz}" "${tmp_checksum}"
        return 1
    fi

    # Verify downloaded .gz file
    echo "Verifying download integrity..."
    expected_checksum=$(grep "${DB_FILE}.gz" "${tmp_checksum}" | awk '{print $1}')

    if [ -z "$expected_checksum" ]; then
        echo "ERROR: Could not find checksum for ${DB_FILE}.gz"
        rm -f "${tmp_gz}" "${tmp_checksum}"
        return 1
    fi

    actual_checksum=$(sha256sum "${tmp_gz}" | awk '{print $1}')

    if [ "$actual_checksum" != "$expected_checksum" ]; then
        echo "ERROR: Checksum mismatch!"
        echo "  Expected: $expected_checksum"
        echo "  Actual:   $actual_checksum"
        rm -f "${tmp_gz}" "${tmp_checksum}"
        return 1
    fi

    echo "✓ Download verified"

    # Extract next to the target, then swap it in with a rename
    echo "Extracting ${DB_FILE}..."
    if ! gunzip -c "${tmp_gz}" > "${DB_PATH}.tmp"; then
        echo "ERROR: Failed to extract database"
        rm -f "${tmp_gz}" "${tmp_checksum}" "${DB_PATH}.tmp"
        return 1
    fi

    # The server opens the database in WAL mode. If it was killed rather than
    # stopped cleanly, the old database's -wal/-shm files are still here, and
    # SQLite would replay that WAL onto the new file and corrupt it.
    rm -f "${DB_PATH}-wal" "${DB_PATH}-shm"
    mv -f "${DB_PATH}.tmp" "${DB_PATH}"
    mv -f "${tmp_checksum}" "${CHECKSUM_FILE}"
    rm -f "${tmp_gz}"

    echo "✓ Database ready: $DB_PATH"
}

# Check whether a newer database has been released.
# Returns 0 when up to date, 1 when an update is available, and 2 when the
# check itself failed (e.g. no network) - the latter must not be mistaken for
# "update available", or an offline restart would fail on the download.
check_for_updates() {
    echo "Checking for updates..."

    temp_checksum=$(mktemp)
    if ! curl -Lfo "$temp_checksum" "${GITHUB_RELEASE_URL}/checksums.txt"; then
        echo "Warning: Could not fetch latest checksums, skipping update check"
        rm -f "$temp_checksum"
        return 2
    fi

    if cmp -s "$temp_checksum" "$CHECKSUM_FILE"; then
        echo "✓ Database is up to date"
        rm -f "$temp_checksum"
        return 0
    fi

    echo "→ New database version available"
    remote_checksum=$(grep "${DB_FILE}.gz" "$temp_checksum" | awk '{print $1}')
    local_checksum=$(grep "${DB_FILE}.gz" "$CHECKSUM_FILE" | awk '{print $1}')
    echo "  Local:  $(echo "$local_checksum" | cut -c1-16)..."
    echo "  Remote: $(echo "$remote_checksum" | cut -c1-16)..."

    rm -f "$temp_checksum"
    return 1
}

# Main logic
if [ -f "$DB_PATH" ] && [ -f "$CHECKSUM_FILE" ]; then
    echo "Database found: $DB_PATH"

    status=0
    check_for_updates || status=$?

    case "$status" in
        1)
            echo "Updating database..."
            if ! download_database; then
                echo "Warning: Update failed, keeping the existing database"
            fi
            ;;
        2)
            echo "Using the existing database"
            ;;
    esac
elif [ -f "$DB_PATH" ]; then
    # A database without checksums can't be checked for updates; try to
    # refresh it, but a working local copy is still better than not starting.
    echo "Database found without checksums, downloading..."
    if ! download_database; then
        echo "Warning: Download failed, using the existing database"
    fi
else
    echo "Database not found, downloading..."
    download_database || exit 1
fi

echo "Starting API server..."
exec ./server
