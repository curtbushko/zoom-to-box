#!/bin/bash

# Script to compare first field of two CSV files
# Usage: ./compare-csv-files.sh file1.csv file2.csv

set -e

# Check if both arguments are provided
if [[ $# -ne 2 ]]; then
    echo "Usage: $0 <file1.csv> <file2.csv>"
    echo "Compares if first field from file1 exists in first field of file2"
    exit 1
fi

FILE1="$1"
FILE2="$2"

# Check if files exist
if [[ ! -f "$FILE1" ]]; then
    echo "ERROR: File '$FILE1' not found"
    exit 1
fi

if [[ ! -f "$FILE2" ]]; then
    echo "ERROR: File '$FILE2' not found"
    exit 1
fi

echo "Comparing files:"
echo "  File 1: $FILE1"
echo "  File 2: $FILE2"
echo ""

# Extract first fields from both files (skipping header line)
# Remove quotes and handle CSV properly
declare -a file1_fields
declare -a file2_fields

# Read file1 first column (skip header)
while IFS=',' read -r first_field rest; do
    # Remove quotes if present
    first_field=$(echo "$first_field" | sed 's/^"//;s/"$//')
    if [[ -n "$first_field" ]]; then
        file1_fields+=("$first_field")
    fi
done < <(tail -n +2 "$FILE1")

# Read file2 first column (skip header)
while IFS=',' read -r first_field rest; do
    # Remove quotes if present
    first_field=$(echo "$first_field" | sed 's/^"//;s/"$//')
    if [[ -n "$first_field" ]]; then
        file2_fields+=("$first_field")
    fi
done < <(tail -n +2 "$FILE2")

echo "File 1 contains ${#file1_fields[@]} entries"
echo "File 2 contains ${#file2_fields[@]} entries"
echo ""

# Check which entries from file1 exist in file2
missing_count=0
found_count=0

echo "Checking which entries from File 1 exist in File 2..."
echo ""

for entry in "${file1_fields[@]}"; do
    found=false
    for entry2 in "${file2_fields[@]}"; do
        if [[ "$entry" == "$entry2" ]]; then
            found=true
            break
        fi
    done

    if [[ "$found" == "true" ]]; then
        echo "✓ FOUND: $entry"
        ((found_count++))
    else
        echo "✗ MISSING: $entry"
        ((missing_count++))
    fi
done

echo ""
echo "=========================================="
echo "Summary:"
echo "  Found in both files: $found_count"
echo "  Missing from file 2: $missing_count"
echo "=========================================="

if [[ $missing_count -gt 0 ]]; then
    exit 1
else
    exit 0
fi
