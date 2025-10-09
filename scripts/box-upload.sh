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
EMAIL=""

# Function to display usage
usage() {
    echo "Usage: $0 -f <file_path> [-e <email>] [-c <config_file>] [-d <folder_id>] [-p <folder_path>] [-n <file_name>]"
    echo ""
    echo "Options:"
    echo "  -f <file_path>    Path to file to upload (required)"
    echo "  -e <email>        User email address (creates folder structure: <YYYY>/<MM>/<DD>)"
    echo "  -c <config_file>  Path to YAML config file (default: config.yaml)"
    echo "  -d <folder_id>    Box folder ID to upload to (default: 0 for root folder)"
    echo "  -p <folder_path>  Folder path to create (e.g., 'user/2024/01/15')"
    echo "  -n <file_name>    Custom file name (optional, uses original filename if not specified)"
    echo "  -h                Show this help message"
    echo ""
    echo "Note: Use -e (email) OR -p (folder_path) OR -d (folder_id), not multiple."
    echo "      When using -e, the folder structure is automatically created based on email and current date."
    echo ""
    echo "Examples:"
    echo "  # Upload for a specific user (auto-creates folder: 2024/10/08)"
    echo "  $0 -f video.mp4 -e user@example.com"
    echo ""
    echo "  # Upload to a specific folder ID"
    echo "  $0 -f video.mp4 -d 98765432"
    echo ""
    echo "  # Upload to a custom folder path"
    echo "  $0 -f video.mp4 -p \"recordings/2024/01/15\""
    echo ""
    echo "  # Upload with custom name"
    echo "  $0 -f video.mp4 -e user@example.com -n \"my-recording.mp4\""
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

# Function to extract username from email
extract_username() {
    local email="$1"
    # Extract everything before the @ symbol
    echo "$email" | cut -d'@' -f1
}

# Function to create date-based folder path for a user
create_user_date_folder_path() {
    local email="$1"

    # Get current date in UTC
    local year=$(date -u '+%Y')
    local month=$(date -u '+%m')
    local day=$(date -u '+%d')

    # Create path: YYYY/MM/DD (no username)
    echo "$year/$month/$day"
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

# Function to get folder ID by searching for a folder name in a parent folder
get_folder_id() {
    local parent_id="$1"
    local folder_name="$2"
    local access_token="$3"

    log "Searching for folder '$folder_name' in parent $parent_id"

    # Get folder items using service account
    local response=$(curl -s -X GET "https://api.box.com/2.0/folders/$parent_id/items?fields=id,name,type&limit=1000" \
        -H "Authorization: Bearer $access_token")

    # Search for the folder in the response
    local folder_id=""
    if command -v jq >/dev/null 2>&1; then
        folder_id=$(echo "$response" | jq -r ".entries[] | select(.type==\"folder\" and .name==\"$folder_name\") | .id" | head -1)
    else
        # Fallback without jq - less reliable but works
        if echo "$response" | grep -q "\"name\":\"$folder_name\""; then
            folder_id=$(echo "$response" | grep -B1 "\"name\":\"$folder_name\"" | grep '"id"' | head -1 | sed 's/.*"id":"\([^"]*\)".*/\1/')
        fi
    fi

    # Strip any whitespace/newlines
    folder_id=$(echo "$folder_id" | tr -d '\n\r' | xargs)

    if [ -n "$folder_id" ] && [ "$folder_id" != "null" ]; then
        log "Found folder '$folder_name' with ID: $folder_id"
        echo "$folder_id"
        return 0
    fi

    log "Folder '$folder_name' not found in parent $parent_id"
    return 1
}

# Function to create a folder
create_folder() {
    local parent_id="$1"
    local folder_name="$2"
    local access_token="$3"

    # Strip any trailing whitespace/newlines from parent_id
    parent_id=$(echo "$parent_id" | tr -d '\n\r' | xargs)

    log "Creating folder '$folder_name' in parent $parent_id"

    # First, check if folder already exists
    local existing_id=$(get_folder_id "$parent_id" "$folder_name" "$access_token")
    if [ $? -eq 0 ] && [ -n "$existing_id" ]; then
        log "Folder '$folder_name' already exists with ID: $existing_id"
        existing_id=$(echo "$existing_id" | tr -d '\n\r' | xargs)
        echo "$existing_id"
        return 0
    fi

    # Folder doesn't exist, create it
    # Escape special characters in folder name for JSON
    local escaped_folder_name=$(echo "$folder_name" | sed 's/\\/\\\\/g' | sed 's/"/\\"/g')

    local json_body="{\"name\":\"$escaped_folder_name\",\"parent\":{\"id\":\"$parent_id\"}}"

    # Use service account (no As-User header) since service account is co-owner of zoom folder
    local response=$(curl -s -X POST "https://api.box.com/2.0/folders" \
        -H "Authorization: Bearer $access_token" \
        -H "Content-Type: application/json" \
        -d "$json_body")

    # Check if folder was created
    if echo "$response" | grep -q '"type":"folder"'; then
        local folder_id=""
        if command -v jq >/dev/null 2>&1; then
            folder_id=$(echo "$response" | jq -r '.id')
        else
            folder_id=$(echo "$response" | grep -o '"id":"[^"]*"' | head -1 | sed 's/"id":"\([^"]*\)"/\1/')
        fi
        # Strip any whitespace/newlines and output only the folder_id
        folder_id=$(echo "$folder_id" | tr -d '\n\r' | xargs)
        log "Created folder '$folder_name' with ID: $folder_id"
        echo "$folder_id"
        return 0
    elif echo "$response" | grep -q '"code":"item_name_in_use"'; then
        # Race condition: folder was created between our check and create attempt
        log "Folder already exists (race condition), extracting folder ID from conflict response"
        # Extract folder ID from the conflict response
        local folder_id=""
        if command -v jq >/dev/null 2>&1; then
            folder_id=$(echo "$response" | jq -r '.context_info.conflicts[0].id // empty')
        else
            # Fallback without jq - extract from conflicts array
            folder_id=$(echo "$response" | grep -o '"conflicts":\[{"type":"folder","id":"[^"]*"' | grep -o '"id":"[^"]*"' | head -1 | sed 's/"id":"\([^"]*\)"/\1/')
        fi

        # Strip any whitespace/newlines
        folder_id=$(echo "$folder_id" | tr -d '\n\r' | xargs)

        if [ -n "$folder_id" ] && [ "$folder_id" != "null" ]; then
            echo "$folder_id"
            return 0
        fi

        # If we couldn't extract from conflict response, try searching again
        log "Could not extract folder ID from conflict response, searching for existing folder"
        existing_id=$(get_folder_id "$parent_id" "$folder_name" "$access_token")
        if [ $? -eq 0 ] && [ -n "$existing_id" ]; then
            existing_id=$(echo "$existing_id" | tr -d '\n\r' | xargs)
            echo "$existing_id"
            return 0
        fi

        log "ERROR: Folder exists but could not determine its ID: $response"
        return 1
    else
        log "ERROR: Failed to create folder: $response"
        log "Response: $response"
        return 1
    fi
}


# Function to create folder path (e.g., "recordings/2024/01/15")
create_folder_path() {
    local folder_path="$1"
    local access_token="$2"
    local start_folder_id="${3:-0}"  # Optional starting folder ID, defaults to root (0)

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
        folder_id=$(create_folder "$current_folder_id" "$folder_name" "$access_token")
        # Strip any trailing whitespace/newlines from folder_id
        folder_id=$(echo "$folder_id" | tr -d '\n\r' | xargs)
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

# Function to check if file exists in folder
get_file_id() {
    local folder_id="$1"
    local file_name="$2"
    local access_token="$3"

    log "Checking if file '$file_name' exists in folder $folder_id"

    # Get folder items
    local response=$(curl -s -X GET "https://api.box.com/2.0/folders/$folder_id/items?fields=id,name,type&limit=1000" \
        -H "Authorization: Bearer $access_token")

    # Search for the file in the response
    local file_id=""
    if command -v jq >/dev/null 2>&1; then
        file_id=$(echo "$response" | jq -r ".entries[] | select(.type==\"file\" and .name==\"$file_name\") | .id" | head -1)
    else
        # Fallback without jq
        if echo "$response" | grep -q "\"name\":\"$file_name\""; then
            file_id=$(echo "$response" | grep -B1 "\"name\":\"$file_name\"" | grep '"id"' | head -1 | sed 's/.*"id":"\([^"]*\)".*/\1/')
        fi
    fi

    # Strip any whitespace/newlines
    file_id=$(echo "$file_id" | tr -d '\n\r' | xargs)

    if [ -n "$file_id" ] && [ "$file_id" != "null" ]; then
        log "Found file '$file_name' with ID: $file_id"
        echo "$file_id"
        return 0
    fi

    log "File '$file_name' not found in folder $folder_id"
    return 1
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

    # Strip any trailing whitespace/newlines from folder_id
    folder_id=$(echo "$folder_id" | tr -d '\n\r' | xargs)

    # Check if file already exists
    local existing_file_id=$(get_file_id "$folder_id" "$file_name" "$access_token")
    if [ $? -eq 0 ] && [ -n "$existing_file_id" ]; then
        log "File '$file_name' already exists with ID: $existing_file_id"

        # Get file details
        if command -v jq >/dev/null 2>&1; then
            local file_info=$(curl -s -X GET "https://api.box.com/2.0/files/$existing_file_id" \
                -H "Authorization: Bearer $access_token")
            local file_size=$(echo "$file_info" | jq -r '.size')
            local created_at=$(echo "$file_info" | jq -r '.created_at')

            echo ""
            echo "File already exists:"
            echo "File ID: $existing_file_id"
            echo "File Name: $file_name"
            echo "File Size: $file_size bytes"
            echo "Created: $created_at"
            echo "Parent Folder ID: $folder_id"
        else
            echo ""
            echo "File already exists with ID: $existing_file_id"
        fi
        return 0
    fi

    # File doesn't exist, proceed with upload
    # Create attributes JSON
    local attributes="{\"name\":\"$file_name\",\"parent\":{\"id\":\"$folder_id\"}}"

    # Use service account (no As-User header) since service account is co-owner of zoom folder
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
    elif [ "$http_code" = "409" ]; then
        # Race condition: file was uploaded between our check and upload attempt
        log "File already exists (race condition), extracting file ID from conflict response"

        if [ -f /tmp/box_upload_response.json ]; then
            local file_id=""
            if command -v jq >/dev/null 2>&1; then
                file_id=$(cat /tmp/box_upload_response.json | jq -r '.context_info.conflicts.id // empty')
            fi

            if [ -z "$file_id" ] || [ "$file_id" = "null" ]; then
                # Try to get the file ID by searching again
                file_id=$(get_file_id "$folder_id" "$file_name" "$access_token")
            fi

            if [ -n "$file_id" ] && [ "$file_id" != "null" ]; then
                echo ""
                echo "File already exists with ID: $file_id"
                rm -f /tmp/box_upload_response.json
                return 0
            fi
        fi

        log "WARNING: File exists but could not determine file ID"
        if [ -f /tmp/box_upload_response.json ]; then
            cat /tmp/box_upload_response.json
        fi
        rm -f /tmp/box_upload_response.json
        return 0  # Don't fail, just warn
    elif [ "$http_code" = "401" ]; then
        log "Received 401 Unauthorized, attempting to refresh token..."
        rm -f /tmp/box_upload_response.json
        return 1  # Signal that token refresh is needed
    else
        log "ERROR: Upload failed with HTTP code: $http_code"
        if [ -f /tmp/box_upload_response.json ]; then
            cat /tmp/box_upload_response.json
        fi
        rm -f /tmp/box_upload_response.json
        exit 1
    fi

    # Clean up temp file
    rm -f /tmp/box_upload_response.json
}

# Parse command line arguments
while getopts "f:e:c:d:p:n:h" opt; do
    case $opt in
        f) FILE_PATH="$OPTARG" ;;
        e) EMAIL="$OPTARG" ;;
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

if [ ! -f "$FILE_PATH" ]; then
    echo "ERROR: File does not exist: $FILE_PATH"
    exit 1
fi

# Check that only one of email, folder_id, or folder_path is specified
specified_count=0
if [ -n "$EMAIL" ]; then
    specified_count=$((specified_count + 1))
fi
if [ -n "$FOLDER_PATH" ]; then
    specified_count=$((specified_count + 1))
fi
if [ -n "$FOLDER_ID" ] && [ "$FOLDER_ID" != "0" ]; then
    specified_count=$((specified_count + 1))
fi

if [ $specified_count -gt 1 ]; then
    echo "ERROR: Cannot specify multiple destination options (-e, -d, -p)"
    echo "       Use only one: -e (email) OR -d (folder_id) OR -p (folder_path)"
    usage
fi

# If email is specified, generate folder path from email and date
if [ -n "$EMAIL" ]; then
    log "Generating folder path from email: $EMAIL"
    FOLDER_PATH=$(create_user_date_folder_path "$EMAIL")
    if [ $? -ne 0 ] || [ -z "$FOLDER_PATH" ]; then
        echo "ERROR: Failed to generate folder path from email"
        exit 1
    fi
    log "Generated folder path: $FOLDER_PATH"
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
if [ -n "$EMAIL" ]; then
    echo "  User: $EMAIL"
    echo "  Destination: /$FOLDER_PATH/"
elif [ -n "$FOLDER_PATH" ]; then
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
echo ""
read -p "Do you want to continue? (y/n): " -n 1 -r
echo ""

if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    log "Upload cancelled by user"
    exit 0
fi

# Get the zoom folder ID - use service account (no As-User header) to find shared folder
log "Getting zoom folder ID from root using service account"
# Call API without As-User header to use service account
response=$(curl -s -X GET "https://api.box.com/2.0/folders/0/items?fields=id,name,type,owned_by&limit=1000" \
    -H "Authorization: Bearer $ACCESS_TOKEN")

log "Root folder response (first 500 chars): ${response:0:500}"

# If EMAIL is provided, extract username to find the correct zoom folder
ZOOM_FOLDER_ID=""
if [ -n "$EMAIL" ]; then
    USERNAME=$(extract_username "$EMAIL")
    log "Looking for zoom folder owned by: $USERNAME"

    # Search for zoom folder owned by the user
    if command -v jq >/dev/null 2>&1; then
        # Get all zoom folders, then check each one's owner details
        zoom_folder_ids=$(echo "$response" | jq -r '.entries[] | select(.type=="folder" and .name=="zoom") | .id')

        # For each zoom folder, get detailed info to check owner
        for folder_id in $zoom_folder_ids; do
            folder_details=$(curl -s -X GET "https://api.box.com/2.0/folders/$folder_id?fields=id,name,owned_by" \
                -H "Authorization: Bearer $ACCESS_TOKEN")

            owner_login=$(echo "$folder_details" | jq -r '.owned_by.login // empty')
            owner_name=$(echo "$folder_details" | jq -r '.owned_by.name // empty')

            log "Checking zoom folder $folder_id - owner login: $owner_login, owner name: $owner_name"

            # Check if owner login matches the username (extract username from owner login if it's an email)
            owner_username=$(extract_username "$owner_login")
            if [ "$owner_username" = "$USERNAME" ]; then
                ZOOM_FOLDER_ID="$folder_id"
                log "Found matching zoom folder: $ZOOM_FOLDER_ID for user $USERNAME"
                break
            fi
        done
    else
        # Fallback without jq - more complex but possible
        log "WARNING: jq not installed, cannot reliably match zoom folder by owner"
        if echo "$response" | grep -q '"name":"zoom"'; then
            ZOOM_FOLDER_ID=$(echo "$response" | grep -B1 '"name":"zoom"' | grep '"id"' | head -1 | sed 's/.*"id":"\([^"]*\)".*/\1/')
        fi
    fi
else
    # No email provided, just get the first zoom folder
    log "No email provided, using first zoom folder found"
    if command -v jq >/dev/null 2>&1; then
        ZOOM_FOLDER_ID=$(echo "$response" | jq -r '.entries[] | select(.type=="folder" and .name=="zoom") | .id' | head -1)
    else
        # Fallback without jq
        if echo "$response" | grep -q '"name":"zoom"'; then
            ZOOM_FOLDER_ID=$(echo "$response" | grep -B1 '"name":"zoom"' | grep '"id"' | head -1 | sed 's/.*"id":"\([^"]*\)".*/\1/')
        fi
    fi
fi

# Strip any whitespace/newlines
ZOOM_FOLDER_ID=$(echo "$ZOOM_FOLDER_ID" | tr -d '\n\r' | xargs)

if [ -z "$ZOOM_FOLDER_ID" ] || [ "$ZOOM_FOLDER_ID" = "null" ]; then
    log "ERROR: Failed to find zoom folder in root directory"
    if [ -n "$EMAIL" ]; then
        log "Could not find zoom folder owned by user: $USERNAME"
    fi
    log "Make sure the 'zoom' folder exists and is shared with your Box app service account"
    exit 1
fi
log "Zoom folder ID: $ZOOM_FOLDER_ID"

# If folder path is specified, create the folder structure within zoom folder
if [ -n "$FOLDER_PATH" ]; then
    log "Creating folder path within zoom folder: $FOLDER_PATH"
    # Start from the zoom folder
    FOLDER_ID=$(create_folder_path "$FOLDER_PATH" "$ACCESS_TOKEN" "$ZOOM_FOLDER_ID")
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
upload_file "$FILE_PATH" "$FOLDER_ID" "$FILE_NAME" "$ACCESS_TOKEN"

log "Box upload completed successfully!"
