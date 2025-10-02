#!/bin/bash

# Box Upload Script
# Uploads a file to Box using OAuth credentials from a config file

set -e

# Default values
CONFIG_FILE="config.yaml"
FOLDER_ID="0"
FILE_PATH=""
FILE_NAME=""

# Function to display usage
usage() {
    echo "Usage: $0 -f <file_path> [-c <config_file>] [-d <folder_id>] [-n <file_name>]"
    echo ""
    echo "Options:"
    echo "  -f <file_path>    Path to file to upload (required)"
    echo "  -c <config_file>  Path to YAML config file (default: config.yaml)"
    echo "  -d <folder_id>    Box folder ID to upload to (default: 0 for root folder)"
    echo "  -n <file_name>    Custom file name (optional, uses original filename if not specified)"
    echo "  -h                Show this help message"
    echo ""
    echo "The config file should be in YAML format with Box credentials:"
    echo "box:"
    echo "  client_id: \"your_client_id\""
    echo "  client_secret: \"your_client_secret\""
    exit 1
}

# Function to log messages
log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $1"
}

# Function to extract YAML value from box section
get_yaml_value() {
    local file="$1"
    local key="$2"
 
    # Extract the box section and get the specific key
    awk "
        /^box:/ { in_box = 1; next }
        in_box && /^[[:space:]]*$key:/ { 
            gsub(/^[[:space:]]*$key:[[:space:]]*/, \"\")
            gsub(/[\"']/, \"\")
            gsub(/[[:space:]]*#.*/, \"\")
            print
            exit
        }
        in_box && /^[[:alpha:]]/ && !/^[[:space:]]/ { in_box = 0 }
    " "$file"
}

# Function to get access token using client credentials
get_access_token() {
    local client_id="$1"
    local client_secret="$2"
 
    log "Getting access token using client credentials..."

    local response=$(curl -s -X POST "https://api.box.com/oauth2/token" \
        -H "Content-Type: application/x-www-form-urlencoded" \
        -d "grant_type=client_credentials" \
        -d "client_id=$client_id" \
        -d "client_secret=$client_secret" \
        -d "box_subject_type=enterprise" \
        -d "box_subject_id=0")

    # Check if token request was successful
    if echo "$response" | grep -q '"access_token"'; then
        # Extract access token using simple JSON parsing
        local access_token=$(echo "$response" | grep -o '"access_token"[[:space:]]*:[[:space:]]*"[^"]*"' | sed 's/.*"\([^"]*\)"/\1/')
        
        log "Access token obtained successfully"
        echo "$access_token"
    else
        log "ERROR: Failed to get access token: $response"
        exit 1
    fi
}

# Function to upload file
upload_file() {
    local file_path="$1"
    local folder_id="$2"
    local file_name="$3"
    local access_token="$4"

    # Use original filename if custom name not provided
    if [ -z "$file_name" ]; then
        file_name=$(basename "$file_path")
    fi

    log "Uploading file: $file_path"
    log "Destination folder ID: $folder_id"
    log "File name: $file_name"

    # Create attributes JSON
    local attributes="{\"name\":\"$file_name\",\"parent\":{\"id\":\"$folder_id\"}}"

    # Upload file with progress
    local response=$(curl -w "%{http_code}" -o /tmp/box_upload_response.json \
        -X POST "https://upload.box.com/api/2.0/files/content" \
        -H "Authorization: Bearer $access_token" \
        -F "attributes=$attributes" \
        -F "file=@$file_path" \
        --progress-bar)

    local http_code="${response: -3}"

    if [ "$http_code" = "201" ]; then
        log "Upload successful!"

        # Parse response and display file info
        if command -v jq >/dev/null 2>&1; then
            local file_info=$(jq -r '.entries[0]' /tmp/box_upload_response.json)
            local file_id=$(echo "$file_info" | jq -r '.id')
            local file_size=$(echo "$file_info" | jq -r '.size')
            local created_at=$(echo "$file_info" | jq -r '.created_at')

            echo ""
            echo "File ID: $file_id"
            echo "File Name: $file_name"
            echo "File Size: $file_size bytes"
            echo "Created: $created_at"
            echo "Parent Folder ID: $folder_id"
        else
            log "Upload completed (install jq for detailed file info)"
            cat /tmp/box_upload_response.json
        fi
    elif [ "$http_code" = "401" ]; then
        log "Received 401 Unauthorized, attempting to refresh token..."
        rm -f /tmp/box_upload_response.json
        return 1  # Signal that token refresh is needed
    else
        log "ERROR: Upload failed with HTTP code: $http_code"
        if [ -f /tmp/box_upload_response.json ]; then
            cat /tmp/box_upload_response.json
        fi
        exit 1
    fi

    # Clean up temp file
    rm -f /tmp/box_upload_response.json
}

# Parse command line arguments
while getopts "f:c:d:n:h" opt; do
    case $opt in
        f) FILE_PATH="$OPTARG" ;;
        c) CONFIG_FILE="$OPTARG" ;;
        d) FOLDER_ID="$OPTARG" ;;
        n) FILE_NAME="$OPTARG" ;;
        h) usage ;;
        *) usage ;;
    esac
done

# Validate required parameters
if [ -z "$FILE_PATH" ]; then
    echo "ERROR: File path is required (-f option)"
    usage
fi

if [ ! -f "$FILE_PATH" ]; then
    echo "ERROR: File does not exist: $FILE_PATH"
    exit 1
fi

if [ ! -f "$CONFIG_FILE" ]; then
    echo "ERROR: Config file does not exist: $CONFIG_FILE"
    echo "Create a YAML file with your Box OAuth credentials:"
    echo "box:"
    echo "  client_id: \"your_client_id\""
    echo "  client_secret: \"your_client_secret\""
    exit 1
fi

# Load credentials from YAML config file
CLIENT_ID=$(get_yaml_value "$CONFIG_FILE" "client_id")
CLIENT_SECRET=$(get_yaml_value "$CONFIG_FILE" "client_secret")

# Validate credentials
if [ -z "$CLIENT_ID" ] || [ -z "$CLIENT_SECRET" ]; then
    echo "ERROR: Missing required credentials in config file"
    echo "Required fields: client_id, client_secret"
    exit 1
fi

log "Starting Box upload process..."

# Get access token using client credentials
ACCESS_TOKEN=$(get_access_token "$CLIENT_ID" "$CLIENT_SECRET")

# Upload file
upload_file "$FILE_PATH" "$FOLDER_ID" "$FILE_NAME" "$ACCESS_TOKEN"

log "Box upload completed successfully!"
