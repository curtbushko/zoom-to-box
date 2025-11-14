#!/bin/bash

# Script to list all active Zoom users and count their recordings
# Uses OAuth credentials from config file to talk to the Zoom API

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

# Function to get recording count for a user
get_recording_count() {
    local user_id="$1"
    local user_email="$2"

    # Zoom API only supports max 30-day ranges, so we need to split 90 days into 3 chunks
    local total_count=0

    # Loop through three 30-day periods (90 days total)
    for period in 0 1 2; do
        local days_back_start=$((period * 30 + 30))
        local days_back_end=$((period * 30))

        # Get recordings from each 30-day period
        # Compatible with both macOS (BSD date) and Linux (GNU date)
        if date -v-${days_back_start}d >/dev/null 2>&1; then
            # macOS/BSD date
            local from_date=$(date -v-${days_back_start}d '+%Y-%m-%d')
            local to_date=$(date -v-${days_back_end}d '+%Y-%m-%d')
        else
            # Linux/GNU date
            local from_date=$(date -d "${days_back_start} days ago" '+%Y-%m-%d')
            local to_date=$(date -d "${days_back_end} days ago" '+%Y-%m-%d')
        fi

        # Query recordings for this user in this period
        local recordings_response=$(curl -s -X GET \
            "${ZOOM_BASE_URL}/users/${user_id}/recordings?from=${from_date}&to=${to_date}&page_size=300" \
            -H "Authorization: Bearer ${ACCESS_TOKEN}" \
            -H "Content-Type: application/json")

        # Extract total_records from the response
        # The Zoom API returns a total_records field that indicates the total number of recordings
        local period_records=$(echo "$recordings_response" | grep -o '"total_records":[0-9]*' | cut -d':' -f2)

        # If grep didn't find total_records, default to 0
        if [[ -z "$period_records" ]]; then
            period_records=0
        fi

        # Add to running total
        total_count=$((total_count + period_records))
    done

    echo "$total_count"
}

# List all active users
log "Fetching list of active users..."

# Initialize pagination variables
page_number=1
page_size=300
has_more_pages=true

# Arrays to store user data
declare -a user_ids
declare -a user_emails
declare -a user_names

# Fetch all users using pagination
while [[ "$has_more_pages" == "true" ]]; do
    log "Fetching page $page_number of users..."

    USERS_RESPONSE=$(curl -s -X GET \
        "${ZOOM_BASE_URL}/users?status=active&page_size=${page_size}&page_number=${page_number}" \
        -H "Authorization: Bearer ${ACCESS_TOKEN}" \
        -H "Content-Type: application/json")

    # Extract user information from this page
    # Using jq to parse JSON if available, otherwise use grep/sed
    if command -v jq >/dev/null 2>&1; then
        # Use jq to extract user data
        page_users=$(echo "$USERS_RESPONSE" | jq -r '.users[] | "\(.id)|||:\(.email)|||:\(.first_name) \(.last_name)"' 2>/dev/null)

        # Add users from this page to arrays
        while IFS='|||' read -r uid email name; do
            if [[ -n "$uid" ]]; then
                user_ids+=("$uid")
                user_emails+=("$email")
                user_names+=("$name")
            fi
        done <<< "$page_users"

        # Check if there are more pages
        page_count=$(echo "$USERS_RESPONSE" | jq -r '.page_count // 0' 2>/dev/null)

        if [[ $page_number -ge $page_count ]]; then
            has_more_pages=false
        else
            ((page_number++))
        fi
    else
        # Fallback: just show the raw response and exit
        log "WARNING: jq not found. Showing raw API response:"
        echo "$USERS_RESPONSE" | grep -o '"users":\[.*\]' || echo "$USERS_RESPONSE"
        has_more_pages=false
    fi
done

log "Found ${#user_ids[@]} active users"

# Output file
OUTPUT_FILE="active_users.txt"

# Initialize output file with header
{
    echo "Email,Name,Recordings (last 90 days)"
} | tee "$OUTPUT_FILE"

# Get recording count for each user
for i in "${!user_ids[@]}"; do
    user_id="${user_ids[$i]}"
    user_email="${user_emails[$i]}"
    user_name="${user_names[$i]}"

    log "Fetching recording count for: $user_email"
    recording_count=$(get_recording_count "$user_id" "$user_email")

    # Only add users with recordings to the output
    if [[ "$recording_count" -gt 0 ]]; then
        # Print user info and recording count as CSV (both to stdout and file)
        echo "$user_email,$user_name,$recording_count" | tee -a "$OUTPUT_FILE"
    fi
done
log "Done! Results saved to $OUTPUT_FILE"
