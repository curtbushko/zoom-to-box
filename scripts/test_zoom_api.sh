#!/bin/bash

# Script to test Zoom API using OAuth credentials from config file
# This script gets a bearer token and fetches user recordings

set -e

# Config file path
CONFIG_FILE="${CONFIG_FILE:-config.yaml}"

# Check if config file exists
if [[ ! -f "$CONFIG_FILE" ]]; then
    echo "Error: Config file '$CONFIG_FILE' not found"
    echo "Create one from config.example.yaml or set CONFIG_FILE environment variable"
    exit 1
fi

# Function to extract values from YAML config
get_config_value() {
    local key="$1"
    grep "^[[:space:]]*${key}:" "$CONFIG_FILE" | sed 's/.*:[[:space:]]*//' | sed 's/^"//' | sed 's/"$//'
}

# Read Zoom credentials from config file
ZOOM_ACCOUNT_ID=$(get_config_value "account_id")
ZOOM_CLIENT_ID=$(get_config_value "client_id") 
ZOOM_CLIENT_SECRET=$(get_config_value "client_secret")
ZOOM_BASE_URL=$(get_config_value "base_url")

# Override with environment variables if set
ZOOM_ACCOUNT_ID="${ZOOM_ACCOUNT_ID:-$ZOOM_ACCOUNT_ID}"
ZOOM_CLIENT_ID="${ZOOM_CLIENT_ID:-$ZOOM_CLIENT_ID}"
ZOOM_CLIENT_SECRET="${ZOOM_CLIENT_SECRET:-$ZOOM_CLIENT_SECRET}"
ZOOM_BASE_URL="${ZOOM_BASE_URL:-https://api.zoom.us/v2}"

# Check required values
if [[ -z "$ZOOM_CLIENT_ID" || -z "$ZOOM_CLIENT_SECRET" || -z "$ZOOM_ACCOUNT_ID" ]]; then
    echo "Error: Missing required Zoom configuration:"
    echo "  account_id: '$ZOOM_ACCOUNT_ID'"
    echo "  client_id: '$ZOOM_CLIENT_ID'"
    echo "  client_secret: '${ZOOM_CLIENT_SECRET:+***}'"
    echo ""
    echo "Please check your $CONFIG_FILE file or set environment variables:"
    echo "  ZOOM_ACCOUNT_ID"
    echo "  ZOOM_CLIENT_ID"
    echo "  ZOOM_CLIENT_SECRET"
    exit 1
fi

# Default user ID (can be overridden)
USER_ID="${1:-me}"

echo "Getting OAuth token from Zoom..."

# Get OAuth token using server-to-server OAuth
TOKEN_RESPONSE=$(curl -s -X POST "https://zoom.us/oauth/token" \
    -H "Content-Type: application/x-www-form-urlencoded" \
    -d "grant_type=account_credentials&account_id=${ZOOM_ACCOUNT_ID}" \
    -u "${ZOOM_CLIENT_ID}:${ZOOM_CLIENT_SECRET}")

# Extract access token from response
ACCESS_TOKEN=$(echo "$TOKEN_RESPONSE" | grep -o '"access_token":"[^"]*' | cut -d'"' -f4)

if [[ -z "$ACCESS_TOKEN" ]]; then
    echo "Error: Failed to get access token"
    echo "Response: $TOKEN_RESPONSE"
    exit 1
fi

echo "Successfully obtained access token"
echo "Token starts with: ${ACCESS_TOKEN:0:20}..."

# Test API call to get user recordings
echo ""
echo "Fetching recordings for user: $USER_ID"

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

echo "Date range: $FROM_DATE to $TO_DATE"
echo "Using base URL: $ZOOM_BASE_URL"

RECORDINGS_RESPONSE=$(curl -s -X GET \
    "${ZOOM_BASE_URL}/users/${USER_ID}/recordings?from=${FROM_DATE}&to=${TO_DATE}&page_size=30" \
    -H "Authorization: Bearer ${ACCESS_TOKEN}" \
    -H "Content-Type: application/json")

# Pretty print the response
echo ""
echo "API Response:"
echo "$RECORDINGS_RESPONSE" | python3 -m json.tool 2>/dev/null || echo "$RECORDINGS_RESPONSE"