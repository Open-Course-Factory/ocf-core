#!/bin/bash

# Terminal Permissions Test Script
# This verifies backend terminal permissions are correctly set up

echo "==================================="
echo "Terminal Permissions Test"
echo "==================================="
echo ""

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Step 1: Login and get token
echo "1. Testing login..."
LOGIN_RESPONSE=$(curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"1.sup","password":"123"}')

TOKEN=$(echo $LOGIN_RESPONSE | grep -o '"accessToken":"[^"]*' | sed 's/"accessToken":"//')

if [ -z "$TOKEN" ]; then
  echo -e "${RED}❌ Login failed${NC}"
  echo "Response: $LOGIN_RESPONSE"
  exit 1
fi
echo -e "${GREEN}✅ Login successful${NC}"
echo ""

# Step 2: Get user permissions
echo "2. Checking user permissions..."
PERMISSIONS_RESPONSE=$(curl -s -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/v1/auth/permissions)

echo "$PERMISSIONS_RESPONSE" | python3 -m json.tool 2>/dev/null || echo "$PERMISSIONS_RESPONSE"
echo ""

# Check if terminal permissions exist
if echo "$PERMISSIONS_RESPONSE" | grep -q "terminals"; then
  echo -e "${GREEN}✅ Terminal permissions found in user permissions${NC}"
else
  echo -e "${RED}❌ Terminal permissions NOT found${NC}"
fi
echo ""

# Step 3: Test terminal list endpoint
echo "3. Testing GET /api/v1/terminals..."
TERMINALS_RESPONSE=$(curl -s -w "\nHTTP_CODE:%{http_code}" \
  -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/v1/terminals)

HTTP_CODE=$(echo "$TERMINALS_RESPONSE" | grep "HTTP_CODE:" | cut -d: -f2)
BODY=$(echo "$TERMINALS_RESPONSE" | sed '/HTTP_CODE:/d')

if [ "$HTTP_CODE" = "200" ]; then
  echo -e "${GREEN}✅ GET /api/v1/terminals - Success (200)${NC}"
  echo "Response: $BODY"
elif [ "$HTTP_CODE" = "403" ]; then
  echo -e "${RED}❌ GET /api/v1/terminals - Forbidden (403)${NC}"
  echo "Response: $BODY"
  echo -e "${YELLOW}This means backend permissions are still not working correctly${NC}"
elif [ "$HTTP_CODE" = "401" ]; then
  echo -e "${RED}❌ GET /api/v1/terminals - Unauthorized (401)${NC}"
  echo "Response: $BODY"
  echo -e "${YELLOW}Token might be invalid or expired${NC}"
else
  echo -e "${RED}❌ GET /api/v1/terminals - Unexpected status ($HTTP_CODE)${NC}"
  echo "Response: $BODY"
fi
echo ""

# Step 4: Check user roles in database
echo "4. Checking user roles in database..."
USER_ID=$(echo $PERMISSIONS_RESPONSE | grep -o '"user_id":"[^"]*' | sed 's/"user_id":"//')

if [ -n "$USER_ID" ]; then
  echo "User ID: $USER_ID"
  PGPASSWORD=root psql -h postgres -U ocf -d ocf -c \
    "SELECT v0 as user_id, STRING_AGG(v1, ', ' ORDER BY v1) as roles
     FROM casbin_rule
     WHERE ptype = 'g' AND v0 = '$USER_ID'
     GROUP BY v0;" 2>/dev/null || echo "Could not query database"
else
  echo -e "${YELLOW}Could not extract user ID from permissions response${NC}"
fi
echo ""

# Step 5: Check terminal policies for roles
echo "5. Checking terminal policies for member/student roles..."
PGPASSWORD=root psql -h postgres -U ocf -d ocf -c \
  "SELECT v0 as role, v1 as resource, v2 as methods
   FROM casbin_rule
   WHERE ptype = 'p'
   AND v0 IN ('member', 'student')
   AND v1 LIKE '%terminal%'
   ORDER BY v0, v1;" 2>/dev/null || echo "Could not query database"
echo ""

echo "==================================="
echo "Test Summary"
echo "==================================="
echo ""
echo "If GET /api/v1/terminals returned 200:"
echo -e "  ${GREEN}✅ Backend permissions are working correctly${NC}"
echo -e "  ${YELLOW}⚠️  Issue is in FRONTEND permission checking${NC}"
echo ""
echo "If GET /api/v1/terminals returned 403:"
echo -e "  ${RED}❌ Backend permissions still have issues${NC}"
echo "  - Check that user has 'member' or 'student' role"
echo "  - Verify Casbin policies exist for terminal routes"
echo "  - Restart the server to reload permissions"
echo ""
echo "Next steps:"
echo "  - Share FRONTEND_TERMINAL_MENU_INVESTIGATION.md with frontend team"
echo "  - Frontend should check for 'member' role (not 'member_pro')"
echo "  - Frontend should verify permission checking logic"
