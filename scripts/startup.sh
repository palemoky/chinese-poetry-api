#!/bin/sh
set -e

echo "=== Chinese Poetry API Startup ==="
echo "Database mode: ${DATABASE_MODE:-1}"

# Function to download database
download_db() {
    local db_type=$1
    local db_file="poetry-${db_type}.db"

    if [ -f "$db_file" ]; then
        echo "Database found: $db_file"
        return 0
    fi

    echo "Downloading ${db_type} database..."
    local url="https://github.com/palemoky/chinese-poetry-api/releases/latest/download/${db_file}.gz"

    if ! curl -L -f -o "${db_file}.gz" "$url"; then
        echo "ERROR: Failed to download ${db_type} database"
        echo "URL: $url"
        return 1
    fi

    echo "Extracting ${db_file}..."
    gunzip "${db_file}.gz"
    echo "Database ready: $db_file"
}

# Download based on DATABASE_MODE (0=both, 1=simplified, 2=traditional)
case "${DATABASE_MODE:-1}" in
    0)
        download_db "simplified" || exit 1
        download_db "traditional" || exit 1
        ;;
    1)
        download_db "simplified" || exit 1
        ;;
    2)
        download_db "traditional" || exit 1
        ;;
    *)
        echo "ERROR: Invalid DATABASE_MODE: ${DATABASE_MODE}"
        echo "Valid options: 0 (both), 1 (simplified), 2 (traditional)"
        exit 1
        ;;
esac

echo "Starting API server..."
exec ./server
