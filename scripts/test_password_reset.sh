#!/bin/bash

# Test script for password reset functionality
# Usage: ./scripts/test_password_reset.sh

set -e

API_URL="http://localhost:8080/api/v1"
TEST_EMAIL="1.supervisor@test.com"

echo "🔐 Testing Password Reset Functionality"
echo "========================================="
echo ""

# Test 1: Request password reset
echo "📧 Test 1: Request password reset for $TEST_EMAIL"
echo "---------------------------------------------------"
RESPONSE=$(curl -s -X POST "$API_URL/auth/password-reset/request" \
  -H "Content-Type: application/json" \
  -d "{\"email\": \"$TEST_EMAIL\"}")

echo "Response: $RESPONSE"
echo ""

SUCCESS=$(echo "$RESPONSE" | grep -o '"success":true' || echo "")
if [ -n "$SUCCESS" ]; then
    echo "✅ Password reset request successful!"
    echo ""
    echo "📬 Check the email inbox for $TEST_EMAIL"
    echo "   (Or check server logs for the email content if SMTP is not configured)"
    echo ""
else
    echo "❌ Password reset request failed!"
    echo ""
    exit 1
fi

# Test 2: Try requesting for non-existent email (should still return success for security)
echo "🔒 Test 2: Request password reset for non-existent email"
echo "--------------------------------------------------------"
RESPONSE=$(curl -s -X POST "$API_URL/auth/password-reset/request" \
  -H "Content-Type: application/json" \
  -d "{\"email\": \"nonexistent@example.com\"}")

echo "Response: $RESPONSE"
echo ""

SUCCESS=$(echo "$RESPONSE" | grep -o '"success":true' || echo "")
if [ -n "$SUCCESS" ]; then
    echo "✅ Correctly returns success (prevents user enumeration)"
    echo ""
else
    echo "❌ Security issue: should return success even for non-existent emails"
    echo ""
    exit 1
fi

# Test 3: Test reset with invalid token
echo "🔑 Test 3: Try resetting password with invalid token"
echo "----------------------------------------------------"
RESPONSE=$(curl -s -X POST "$API_URL/auth/password-reset/confirm" \
  -H "Content-Type: application/json" \
  -d "{\"token\": \"invalid_token_12345\", \"new_password\": \"NewPassword123\"}")

echo "Response: $RESPONSE"
echo ""

FAILED=$(echo "$RESPONSE" | grep -o '"success":false' || echo "")
if [ -n "$FAILED" ]; then
    echo "✅ Correctly rejects invalid token"
    echo ""
else
    echo "❌ Security issue: should reject invalid tokens"
    echo ""
    exit 1
fi

# Test 4: Database check
echo "💾 Test 4: Check password reset tokens in database"
echo "--------------------------------------------------"

# Try to connect to the database and show recent tokens
if command -v psql &> /dev/null; then
    echo "Querying database for recent password reset tokens..."
    PGPASSWORD=root psql -h localhost -U ocf -d ocf -c \
        "SELECT id, user_id, LEFT(token, 16) || '...' as token_preview, expires_at, used_at, created_at
         FROM password_reset_tokens
         ORDER BY created_at DESC
         LIMIT 5;" 2>/dev/null || echo "⚠️  Could not connect to database (this is OK if running in Docker)"
    echo ""
else
    echo "⚠️  psql not available - skipping database check"
    echo ""
fi

echo "✅ All tests completed!"
echo ""
echo "📋 Next steps:"
echo "1. Configure SMTP in .env file (see PASSWORD_RESET_SETUP.md)"
echo "2. Request a password reset for a real user"
echo "3. Check your email inbox"
echo "4. Use the token from the email to reset the password"
echo ""
echo "Example:"
echo "  curl -X POST $API_URL/auth/password-reset/confirm \\"
echo "    -H 'Content-Type: application/json' \\"
echo "    -d '{\"token\": \"YOUR_TOKEN_FROM_EMAIL\", \"new_password\": \"NewSecurePassword123\"}'"
echo ""
