#!/bin/bash

# Box Upload Script
# Uploads a file to Box using OAuth credentials from a config file

set -e

# Default values
CONFIG_FILE="config.yaml"
FOLDER_ID="0"
FOLDER_PATH=""
FILE_PATH=""
FILE_NAME=""
USER_ID=""

# Function to display usage
usage() {
    echo "Usage: $0 -f <file_path> -u <user_id> [-c <config_file>] [-d <folder_id>] [-p <folder_path>] [-n <file_name>]"
    echo ""
    echo "Options:"
    echo "  -f <file_path>    Path to file to upload (required)"
    echo "  -u <user_id>      Box user ID to upload as (required)"
    echo "  -c <config_file>  Path to YAML config file (default: config.yaml)"
    echo "  -d <folder_id>    Box folder ID to upload to (default: 0 for root folder)"
    echo "  -p <folder_path>  Folder path to create (e.g., 'recordings/2024/01/15')"
    echo "  -n <file_name>    Custom file name (optional, uses original filename if not specified)"
    echo "  -h                Show this help message"
    echo ""
    echo "Note: Use either -d (folder_id) or -p (folder_path), not both."
    echo ""
    echo "Examples:"
    echo "  # Upload to a specific folder ID"
    echo "  $0 -f video.mp4 -u 12345678 -d 98765432"
    echo ""
    echo "  # Upload to a folder path (creates if doesn't exist)"
    echo "  $0 -f video.mp4 -u 12345678 -p \"recordings/2024/01/15\""
    echo ""
    echo "  # Upload to root with custom name"
    echo "  $0 -f video.mp4 -u 12345678 -n \"my-recording.mp4\""
    echo ""
    echo "The config file should be in YAML format with Box credentials:"
    echo "box:"
    echo "  client_id: \"your_client_id\""
    echo "  client_secret: \"your_client_secret\""
    echo "  enterprise_id: \"your_enterprise_id\""
    exit 1
}

# Function to log messages
log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $1" >&2
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
    local enterprise_id="$3"

    log "Getting access token using client credentials..."

    local response=$(curl -s -X POST "https://api.box.com/oauth2/token" \
        -H "Content-Type: application/x-www-form-urlencoded" \
        -d "grant_type=client_credentials" \
        -d "client_id=$client_id" \
        -d "client_secret=$client_secret" \
        -d "box_subject_type=enterprise" \
        -d "box_subject_id=$enterprise_id")

    # Check if token request was successful
    if echo "$response" | grep -q '"access_token"'; then
        # Extract access token using simple JSON parsing
        local access_token=$(echo "$response" | grep -o '"access_token"[[:space:]]*:[[:space:]]*"[^"]*"' | sed 's/.*:[[:space:]]*"\([^"]*\)".*/\1/')

        log "Access token obtained successfully"
        echo "$access_token"
    else
        log "ERROR: Failed to get access token: $response"
        exit 1
    fi
}

# Function to create a folder
create_folder() {
    local parent_id="$1"
    local folder_name="$2"
    local access_token="$3"
    local user_id="$4"

    log "Creating folder '$folder_name' in parent $parent_id"

    local json_body="{\"name\":\"$folder_name\",\"parent\":{\"id\":\"$parent_id\"}}"

    local response=$(curl -s -X POST "https://api.box.com/2.0/folders" \
        -H "Authorization: Bearer $access_token" \
        -H "As-User: $user_id" \
        -H "Content-Type: application/json" \
        -d "$json_body")

    # Check if folder was created or already exists
    if echo "$response" | grep -q '"type":"folder"'; then
        if command -v jq >/dev/null 2>&1; then
            local folder_id=$(echo "$response" | jq -r '.id')
            echo "$folder_id"
            return 0
        else
            local folder_id=$(echo "$response" | grep -o '"id":"[^"]*"' | head -1 | sed 's/.*"\([^"]*\)"/\1/')
            echo "$folder_id"
            return 0
        fi
    elif echo "$response" | grep -q '"code":"item_name_in_use"'; then
        log "Folder already exists, extracting folder ID from conflict response"
        # Extract folder ID from the conflict response
        if command -v jq >/dev/null 2>&1; then
            local folder_id=$(echo "$response" | jq -r '.context_info.conflicts.id // empty')
            if [ -n "$folder_id" ] && [ "$folder_id" != "null" ]; then
                echo "$folder_id"
                return 0
            fi
        fi
        log "ERROR: Folder exists but could not extract ID from response: $response"
        return 1
    else
        log "ERROR: Failed to create folder: $response"
        return 1
    fi
}


# Function to create folder path (e.g., "recordings/2024/01/15")
create_folder_path() {
    local folder_path="$1"
    local access_token="$2"
    local user_id="$3"
    local start_folder_id="${4:-0}"  # Optional starting folder ID, defaults to root (0)

    # Start from specified folder or root
    local current_folder_id="$start_folder_id"

    # Split path by / and create each folder
    IFS='/' read -ra FOLDERS <<< "$folder_path"
    for folder_name in "${FOLDERS[@]}"; do
        # Skip empty folder names
        if [ -z "$folder_name" ]; then
            continue
        fi

        # Create folder - if it already exists, create_folder will handle it
        folder_id=$(create_folder "$current_folder_id" "$folder_name" "$access_token" "$user_id")
        if [ $? -eq 0 ] && [ -n "$folder_id" ]; then
            log "Using folder: $folder_name (ID: $folder_id)"
            current_folder_id="$folder_id"
        else
            log "ERROR: Failed to create folder: $folder_name"
            return 1
        fi
    done

    echo "$current_folder_id"
    return 0
}

# Function to upload file
upload_file() {
    local file_path="$1"
    local folder_id="$2"
    local file_name="$3"
    local access_token="$4"
    local user_id="$5"

    # Use original filename if custom name not provided
    if [ -z "$file_name" ]; then
        file_name=$(basename "$file_path")
    fi

    log "Uploading file: $file_path"
    log "Destination folder ID: $folder_id"
    log "File name: $file_name"
    log "As user: $user_id"

    # Create attributes JSON
    local attributes="{\"name\":\"$file_name\",\"parent\":{\"id\":\"$folder_id\"}}"

    # Upload file with progress
    local response=$(curl -w "%{http_code}" -o /tmp/box_upload_response.json \
        -X POST "https://upload.box.com/api/2.0/files/content" \
        -H "Authorization: Bearer $access_token" \
        -H "As-User: $user_id" \
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
while getopts "f:u:c:d:p:n:h" opt; do
    case $opt in
        f) FILE_PATH="$OPTARG" ;;
        u) USER_ID="$OPTARG" ;;
        c) CONFIG_FILE="$OPTARG" ;;
        d) FOLDER_ID="$OPTARG" ;;
        p) FOLDER_PATH="$OPTARG" ;;
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

if [ -z "$USER_ID" ]; then
    echo "ERROR: User ID is required (-u option)"
    usage
fi

if [ ! -f "$FILE_PATH" ]; then
    echo "ERROR: File does not exist: $FILE_PATH"
    exit 1
fi

# Check that only one of folder_id or folder_path is specified
if [ -n "$FOLDER_ID" ] && [ "$FOLDER_ID" != "0" ] && [ -n "$FOLDER_PATH" ]; then
    echo "ERROR: Cannot specify both -d (folder_id) and -p (folder_path)"
    usage
fi

if [ ! -f "$CONFIG_FILE" ]; then
    echo "ERROR: Config file does not exist: $CONFIG_FILE"
    echo "Create a YAML file with your Box OAuth credentials:"
    echo "box:"
    echo "  client_id: \"your_client_id\""
    echo "  client_secret: \"your_client_secret\""
    echo "  enterprise_id: \"your_enterprise_id\""
    exit 1
fi

# Load credentials from YAML config file
CLIENT_ID=$(get_yaml_value "$CONFIG_FILE" "client_id")
CLIENT_SECRET=$(get_yaml_value "$CONFIG_FILE" "client_secret")
ENTERPRISE_ID=$(get_yaml_value "$CONFIG_FILE" "enterprise_id")

# Validate credentials
if [ -z "$CLIENT_ID" ] || [ -z "$CLIENT_SECRET" ] || [ -z "$ENTERPRISE_ID" ]; then
    echo "ERROR: Missing required credentials in config file"
    echo "Required fields: client_id, client_secret, enterprise_id"
    exit 1
fi

log "Starting Box upload process..."

# Get access token using client credentials
ACCESS_TOKEN=$(get_access_token "$CLIENT_ID" "$CLIENT_SECRET" "$ENTERPRISE_ID")

# Ask for confirmation
echo "You are about to upload:"
echo "  File: $FILE_PATH"
if [ -n "$FOLDER_PATH" ]; then
    echo "  Destination: /$FOLDER_PATH/"
elif [ "$FOLDER_ID" != "0" ]; then
    echo "  Destination: Folder ID $FOLDER_ID"
else
    echo "  Destination: Root folder (ID: 0)"
fi
if [ -n "$FILE_NAME" ]; then
    echo "  As: $FILE_NAME"
else
    echo "  As: $(basename "$FILE_PATH")"
fi
echo "  User ID: $USER_ID"
echo ""
read -p "Do you want to continue? (y/n): " -n 1 -r
echo ""

if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    log "Upload cancelled by user"
    exit 0
fi

# First create the zoom folder at root
log "Creating zoom folder at root"
ZOOM_FOLDER_ID=$(create_folder "0" "zoom" "$ACCESS_TOKEN" "$USER_ID")
if [ $? -ne 0 ] || [ -z "$ZOOM_FOLDER_ID" ]; then
    log "ERROR: Failed to create zoom folder"
    exit 1
fi
log "Zoom folder ID: $ZOOM_FOLDER_ID"

# If folder path is specified, create the folder structure within zoom folder
if [ -n "$FOLDER_PATH" ]; then
    log "Creating folder path within zoom folder: $FOLDER_PATH"
    # Start from the zoom folder
    FOLDER_ID=$(create_folder_path "$FOLDER_PATH" "$ACCESS_TOKEN" "$USER_ID" "$ZOOM_FOLDER_ID")
    if [ $? -ne 0 ] || [ -z "$FOLDER_ID" ]; then
        log "ERROR: Failed to create folder path"
        exit 1
    fi
    log "Target folder ID: $FOLDER_ID"
elif [ "$FOLDER_ID" = "0" ]; then
    # If uploading to root, use zoom folder instead
    FOLDER_ID="$ZOOM_FOLDER_ID"
    log "Using zoom folder as target (ID: $FOLDER_ID)"
fi

# Upload file
upload_file "$FILE_PATH" "$FOLDER_ID" "$FILE_NAME" "$ACCESS_TOKEN" "$USER_ID"

log "Box upload completed successfully!"
