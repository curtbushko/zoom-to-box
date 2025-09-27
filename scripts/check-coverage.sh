#!/bin/bash

# Coverage validation script for zoom-to-box
# Ensures unit test coverage meets the 90% requirement specified in Feature 5.1

set -e

echo "🧪 Running comprehensive unit test coverage analysis..."

# Generate coverage profile
echo "📊 Generating coverage profile..."
go test -coverprofile=coverage.out ./... || {
    echo "❌ Tests failed. Coverage analysis cannot proceed."
    exit 1
}

# Generate coverage report
echo "📋 Generating detailed coverage report..."
go tool cover -func=coverage.out > coverage-report.txt

# Extract overall coverage percentage
COVERAGE=$(go tool cover -func=coverage.out | grep total | awk '{print $3}' | sed 's/%//')

echo "📈 Overall Coverage: ${COVERAGE}%"

# Display coverage by package
echo ""
echo "📦 Coverage by Package:"
echo "======================"
go test -cover ./... | grep -E "coverage:|FAIL" | sort

# Check if coverage meets the 90% requirement
REQUIRED_COVERAGE=90
if (( $(echo "$COVERAGE >= $REQUIRED_COVERAGE" | bc -l) )); then
    echo ""
    echo "✅ SUCCESS: Coverage ($COVERAGE%) meets the requirement (>= $REQUIRED_COVERAGE%)"
    echo "🎉 Feature 5.1 requirement satisfied!"
else
    echo ""
    echo "❌ FAILURE: Coverage ($COVERAGE%) is below the requirement (>= $REQUIRED_COVERAGE%)"
    echo "📋 Analysis of packages below 90% coverage:"
    echo ""
    
    # Show packages that need improvement
    go test -cover ./... | grep -E "coverage:" | while read line; do
        PKG=$(echo "$line" | awk '{print $2}')
        COV=$(echo "$line" | awk '{print $5}' | sed 's/coverage://' | sed 's/%//' | sed 's/statements//')
        
        if (( $(echo "$COV < $REQUIRED_COVERAGE" | bc -l) )); then
            echo "  📉 $PKG: ${COV}% (needs +$(echo "$REQUIRED_COVERAGE - $COV" | bc)% improvement)"
        fi
    done
    
    echo ""
    echo "💡 To improve coverage, focus on adding tests for uncovered functions."
    echo "📖 Use 'go tool cover -html=coverage.out' to see detailed line-by-line coverage."
    exit 1
fi

# Clean up
rm -f coverage.out

echo ""
echo "✨ Coverage validation complete!"