#!/bin/bash

# Test script to verify CORS is working for frontend on port 4000
# This tests both the version endpoint and the features endpoint

echo "🧪 Testing CORS for http://localhost:4000"
echo "========================================="
echo ""

# Test 1: Version endpoint (no auth required)
echo "Test 1: OPTIONS on /api/v1/version"
echo "-----------------------------------"
curl -s -X OPTIONS http://localhost:8080/api/v1/version \
  -H "Origin: http://localhost:4000" \
  -H "Access-Control-Request-Method: GET" \
  -v 2>&1 | grep -i "access-control-allow-origin"

if [ $? -eq 0 ]; then
  echo "✅ Version endpoint CORS: WORKING"
else
  echo "❌ Version endpoint CORS: FAILED"
fi
echo ""

# Test 2: Features endpoint (with auth)
echo "Test 2: OPTIONS on /api/v1/features"
echo "------------------------------------"
curl -s -X OPTIONS http://localhost:8080/api/v1/features \
  -H "Origin: http://localhost:4000" \
  -H "Access-Control-Request-Method: POST" \
  -H "Access-Control-Request-Headers: Authorization,Content-Type" \
  -v 2>&1 | grep -i "access-control-allow-origin"

if [ $? -eq 0 ]; then
  echo "✅ Features endpoint CORS: WORKING"
else
  echo "❌ Features endpoint CORS: FAILED"
fi
echo ""

# Test 3: Full request with headers
echo "Test 3: Full response headers"
echo "------------------------------"
curl -s -X OPTIONS http://localhost:8080/api/v1/features \
  -H "Origin: http://localhost:4000" \
  -H "Access-Control-Request-Method: POST" \
  -H "Access-Control-Request-Headers: Authorization,Content-Type" \
  -i 2>&1 | grep -i "access-control"

echo ""
echo "========================================="
echo "✅ Test complete!"
echo ""
echo "📋 What to look for:"
echo "  - Access-Control-Allow-Origin: http://localhost:4000"
echo "  - Access-Control-Allow-Methods: should include GET, POST"
echo "  - Access-Control-Allow-Headers: should include Authorization"
echo ""
echo "🔧 If you still see CORS errors:"
echo "  1. Make sure you restarted the backend server"
echo "  2. Clear your browser cache (Ctrl+Shift+Del)"
echo "  3. Check that ENVIRONMENT=development in your .env file"
echo "  4. Look for this log in server startup:"
echo "     '🔓 Development mode: CORS allowing common localhost origins'"
