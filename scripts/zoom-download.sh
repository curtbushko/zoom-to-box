#!/bin/bash

# Script to test Zoom API using OAuth credentials from config file
# This script gets a bearer token and fetches user recordings

set -e

# Config file path
CONFIG_FILE="${CONFIG_FILE:-config.yaml}"

# Function to log messages
log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $1"
}

# Check if config file exists
if [[ ! -f "$CONFIG_FILE" ]]; then
    log "ERROR: Config file '$CONFIG_FILE' not found"
    log "Create one from config.example.yaml or set CONFIG_FILE environment variable"
    exit 1
fi

# Function to extract YAML value from zoom section
get_yaml_value() {
    local file="$1"
    local section="$2"
    local key="$3"

    # Extract the specified section and get the specific key
    awk -v section="$section" -v key="$key" '
        $0 ~ "^" section ":" { in_section = 1; next }
        in_section && $0 ~ "^[[:space:]]*" key ":" {
            gsub("^[[:space:]]*" key ":[[:space:]]*", "")
            gsub(/["'\'']/, "")
            gsub(/[[:space:]]*#.*/, "")
            print
            exit
        }
        in_section && /^[[:alpha:]]/ && !/^[[:space:]]/ { in_section = 0 }
    ' "$file"
}

# Read Zoom credentials from config file
CONFIG_ACCOUNT_ID=$(get_yaml_value "$CONFIG_FILE" "zoom" "account_id")
CONFIG_CLIENT_ID=$(get_yaml_value "$CONFIG_FILE" "zoom" "client_id")
CONFIG_CLIENT_SECRET=$(get_yaml_value "$CONFIG_FILE" "zoom" "client_secret")
CONFIG_BASE_URL=$(get_yaml_value "$CONFIG_FILE" "zoom" "base_url")

# Override with environment variables if set, otherwise use config values
ZOOM_ACCOUNT_ID="${ZOOM_ACCOUNT_ID:-$CONFIG_ACCOUNT_ID}"
ZOOM_CLIENT_ID="${ZOOM_CLIENT_ID:-$CONFIG_CLIENT_ID}"
ZOOM_CLIENT_SECRET="${ZOOM_CLIENT_SECRET:-$CONFIG_CLIENT_SECRET}"
ZOOM_BASE_URL="${ZOOM_BASE_URL:-${CONFIG_BASE_URL:-https://api.zoom.us/v2}}"

# Check required values
if [[ -z "$ZOOM_CLIENT_ID" || -z "$ZOOM_CLIENT_SECRET" || -z "$ZOOM_ACCOUNT_ID" ]]; then
    log "ERROR: Missing required Zoom configuration:"
    log "  account_id: '$ZOOM_ACCOUNT_ID'"
    log "  client_id: '$ZOOM_CLIENT_ID'"
    log "  client_secret: '${ZOOM_CLIENT_SECRET:+***}'"
    log ""
    log "Please check your $CONFIG_FILE file or set environment variables:"
    log "  ZOOM_ACCOUNT_ID"
    log "  ZOOM_CLIENT_ID"
    log "  ZOOM_CLIENT_SECRET"
    exit 1
fi

# Default user ID (can be overridden)
USER_ID="${1:-me}"

log "Getting OAuth token from Zoom..."

# Get OAuth token using server-to-server OAuth
TOKEN_RESPONSE=$(curl -s -X POST "https://zoom.us/oauth/token" \
    -H "Content-Type: application/x-www-form-urlencoded" \
    -d "grant_type=account_credentials&account_id=${ZOOM_ACCOUNT_ID}" \
    -u "${ZOOM_CLIENT_ID}:${ZOOM_CLIENT_SECRET}")

# Extract access token from response
ACCESS_TOKEN=$(echo "$TOKEN_RESPONSE" | grep -o '"access_token":"[^"]*' | cut -d'"' -f4)

if [[ -z "$ACCESS_TOKEN" ]]; then
    log "ERROR: Failed to get access token"
    log "Response: $TOKEN_RESPONSE"
    exit 1
fi

log "Successfully obtained access token"
log "Token starts with: ${ACCESS_TOKEN:0:20}..."

# Test API call to get user recordings
echo ""
log "Fetching recordings for user: $USER_ID"

# Get recordings from the last 30 days
# Compatible with both macOS (BSD date) and Linux (GNU date)
if date -v-30d >/dev/null 2>&1; then
    # macOS/BSD date
    FROM_DATE=$(date -v-30d '+%Y-%m-%d')
else
    # Linux/GNU date
    FROM_DATE=$(date -d '30 days ago' '+%Y-%m-%d')
fi
TO_DATE=$(date '+%Y-%m-%d')

log "Date range: $FROM_DATE to $TO_DATE"
log "Using base URL: $ZOOM_BASE_URL"

RECORDINGS_RESPONSE=$(curl -s -X GET \
    "${ZOOM_BASE_URL}/users/${USER_ID}/recordings?from=${FROM_DATE}&to=${TO_DATE}&page_size=30" \
    -H "Authorization: Bearer ${ACCESS_TOKEN}" \
    -H "Content-Type: application/json")

# Pretty print the response
echo ""
echo "API Response:"
echo "$RECORDINGS_RESPONSE" | python3 -m json.tool 2>/dev/null || echo "$RECORDINGS_RESPONSE"
