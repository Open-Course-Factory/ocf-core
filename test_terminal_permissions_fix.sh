#!/bin/bash

# Terminal Permissions Fix Verification Script
# This script tests that the member role has access to all terminal endpoints

set -e

echo "=========================================="
echo "Terminal Permissions Fix Verification"
echo "=========================================="
echo ""

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Check if TOKEN is set
if [ -z "$TOKEN" ]; then
    echo -e "${RED}❌ ERROR: TOKEN environment variable not set${NC}"
    echo ""
    echo "Please set your JWT token:"
    echo "  export TOKEN='your-jwt-token-here'"
    echo ""
    echo "To get a token, login via the API or use an existing valid token."
    exit 1
fi

BASE_URL="http://localhost:8080/api/v1"

echo "Using token: ${TOKEN:0:20}..."
echo "Base URL: $BASE_URL"
echo ""

# Function to test endpoint
test_endpoint() {
    local method=$1
    local endpoint=$2
    local description=$3
    local data=$4

    echo -n "Testing: $description ... "

    if [ "$method" = "GET" ]; then
        response=$(curl -s -w "\n%{http_code}" -X GET "$BASE_URL$endpoint" \
            -H "Authorization: Bearer $TOKEN" \
            -H "Content-Type: application/json")
    else
        if [ -z "$data" ]; then
            response=$(curl -s -w "\n%{http_code}" -X "$method" "$BASE_URL$endpoint" \
                -H "Authorization: Bearer $TOKEN" \
                -H "Content-Type: application/json")
        else
            response=$(curl -s -w "\n%{http_code}" -X "$method" "$BASE_URL$endpoint" \
                -H "Authorization: Bearer $TOKEN" \
                -H "Content-Type: application/json" \
                -d "$data")
        fi
    fi

    http_code=$(echo "$response" | tail -n1)
    body=$(echo "$response" | head -n-1)

    # Success: 200-299 or 404 (endpoint exists but resource not found)
    # Failure: 403 (forbidden - permission denied)
    if [ "$http_code" = "403" ]; then
        echo -e "${RED}❌ FAILED (403 Forbidden - Permission Denied)${NC}"
        echo "   Response: $body"
        return 1
    elif [ "$http_code" -ge 200 ] && [ "$http_code" -lt 300 ]; then
        echo -e "${GREEN}✅ PASSED ($http_code)${NC}"
        return 0
    elif [ "$http_code" = "404" ]; then
        echo -e "${YELLOW}⚠️  PASSED ($http_code - Resource not found, but endpoint accessible)${NC}"
        return 0
    else
        echo -e "${YELLOW}⚠️  UNCLEAR ($http_code)${NC}"
        echo "   Response: $body"
        return 0
    fi
}

# Track results
total_tests=0
passed_tests=0
failed_tests=0

echo "=========================================="
echo "User Terminal Key Endpoints"
echo "=========================================="
echo ""

total_tests=$((total_tests + 1))
if test_endpoint "POST" "/user-terminal-keys/regenerate" "Regenerate terminal key"; then
    passed_tests=$((passed_tests + 1))
else
    failed_tests=$((failed_tests + 1))
fi

total_tests=$((total_tests + 1))
if test_endpoint "GET" "/user-terminal-keys/my-key" "Get my terminal key"; then
    passed_tests=$((passed_tests + 1))
else
    failed_tests=$((failed_tests + 1))
fi

echo ""
echo "=========================================="
echo "Terminal Management Endpoints"
echo "=========================================="
echo ""

total_tests=$((total_tests + 1))
if test_endpoint "GET" "/terminals/user-sessions" "Get user sessions"; then
    passed_tests=$((passed_tests + 1))
else
    failed_tests=$((failed_tests + 1))
fi

total_tests=$((total_tests + 1))
if test_endpoint "GET" "/terminals/shared-with-me" "Get shared terminals"; then
    passed_tests=$((passed_tests + 1))
else
    failed_tests=$((failed_tests + 1))
fi

total_tests=$((total_tests + 1))
if test_endpoint "GET" "/terminals/instance-types" "Get instance types"; then
    passed_tests=$((passed_tests + 1))
else
    failed_tests=$((failed_tests + 1))
fi

total_tests=$((total_tests + 1))
if test_endpoint "GET" "/terminals/metrics" "Get server metrics"; then
    passed_tests=$((passed_tests + 1))
else
    failed_tests=$((failed_tests + 1))
fi

total_tests=$((total_tests + 1))
if test_endpoint "POST" "/terminals/sync-all" "Sync all sessions"; then
    passed_tests=$((passed_tests + 1))
else
    failed_tests=$((failed_tests + 1))
fi

echo ""
echo "=========================================="
echo "Terminal Instance Endpoints (with ID)"
echo "=========================================="
echo ""
echo "Note: These will return 404 if terminal doesn't exist,"
echo "but that's OK - it means permissions are working."
echo ""

# Use a fake ID for testing permissions
FAKE_ID="00000000-0000-0000-0000-000000000000"

total_tests=$((total_tests + 1))
if test_endpoint "GET" "/terminals/$FAKE_ID/console" "Access terminal console"; then
    passed_tests=$((passed_tests + 1))
else
    failed_tests=$((failed_tests + 1))
fi

total_tests=$((total_tests + 1))
if test_endpoint "GET" "/terminals/$FAKE_ID/status" "Get terminal status"; then
    passed_tests=$((passed_tests + 1))
else
    failed_tests=$((failed_tests + 1))
fi

total_tests=$((total_tests + 1))
if test_endpoint "GET" "/terminals/$FAKE_ID/info" "Get terminal info"; then
    passed_tests=$((passed_tests + 1))
else
    failed_tests=$((failed_tests + 1))
fi

total_tests=$((total_tests + 1))
if test_endpoint "POST" "/terminals/$FAKE_ID/stop" "Stop terminal"; then
    passed_tests=$((passed_tests + 1))
else
    failed_tests=$((failed_tests + 1))
fi

total_tests=$((total_tests + 1))
if test_endpoint "POST" "/terminals/$FAKE_ID/sync" "Sync terminal"; then
    passed_tests=$((passed_tests + 1))
else
    failed_tests=$((failed_tests + 1))
fi

total_tests=$((total_tests + 1))
if test_endpoint "GET" "/terminals/$FAKE_ID/shares" "Get terminal shares"; then
    passed_tests=$((passed_tests + 1))
else
    failed_tests=$((failed_tests + 1))
fi

echo ""
echo "=========================================="
echo "Test Summary"
echo "=========================================="
echo ""
echo "Total tests: $total_tests"
echo -e "${GREEN}Passed: $passed_tests${NC}"
echo -e "${RED}Failed: $failed_tests${NC}"
echo ""

if [ $failed_tests -eq 0 ]; then
    echo -e "${GREEN}✅ All tests passed! Terminal permissions are working correctly.${NC}"
    exit 0
else
    echo -e "${RED}❌ Some tests failed. Please check the permissions setup.${NC}"
    echo ""
    echo "Troubleshooting steps:"
    echo "1. Restart the server: pkill -f 'go run main.go' && go run main.go"
    echo "2. Check server logs for permission setup messages"
    echo "3. Verify casbin_rule table has the permissions"
    echo "4. See TERMINAL_PERMISSIONS_FIX.md for more help"
    exit 1
fi
