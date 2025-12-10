#!/bin/sh
set -e

echo "=== Chinese Poetry API Startup ==="
echo "Database mode: ${DATABASE_MODE:-simplified}"

# Function to download database
download_db() {
    local db_type=$1
    local db_file="poetry-${db_type}.db"

    if [ -f "$db_file" ]; then
        echo "Database found: $db_file"
        return 0
    fi

    echo "Downloading ${db_type} database..."
    local url="https://github.com/palemoky/chinese-poetry-api/releases/download/latest/${db_file}.gz"

    if ! curl -L -f -o "${db_file}.gz" "$url"; then
        echo "ERROR: Failed to download ${db_type} database"
        echo "URL: $url"
        return 1
    fi

    echo "Extracting ${db_file}..."
    gunzip "${db_file}.gz"
    echo "Database ready: $db_file"
}

# Download based on DATABASE_MODE
case "${DATABASE_MODE:-simplified}" in
    simplified)
        download_db "simplified" || exit 1
        ;;
    traditional)
        download_db "traditional" || exit 1
        ;;
    both)
        download_db "simplified" || exit 1
        download_db "traditional" || exit 1
        ;;
    *)
        echo "ERROR: Invalid DATABASE_MODE: ${DATABASE_MODE}"
        echo "Valid options: simplified, traditional, both"
        exit 1
        ;;
esac

echo "Starting API server..."
exec ./server
