#!/bin/bash
# Diagnostic script to test CI tag push permissions
# Run this in GitLab CI to diagnose tag push issues

set -e

echo "🔍 Diagnosing GitLab CI Tag Push Configuration"
echo "=============================================="
echo ""

# Check if we're in CI
if [ -z "$CI" ]; then
  echo "❌ Not running in GitLab CI environment"
  exit 1
fi

echo "✅ Running in GitLab CI"
echo ""

# Check available tokens
echo "📋 Token Configuration:"
if [ -n "$CI_PUSH_TOKEN" ]; then
  echo "  ✅ CI_PUSH_TOKEN is configured (recommended)"
  TOKEN="$CI_PUSH_TOKEN"
  TOKEN_TYPE="CI_PUSH_TOKEN"
else
  echo "  ⚠️  CI_PUSH_TOKEN not configured, using CI_JOB_TOKEN"
  echo "      CI_JOB_TOKEN may lack permission to push tags"
  TOKEN="$CI_JOB_TOKEN"
  TOKEN_TYPE="CI_JOB_TOKEN"
fi
echo ""

# Check CI environment
echo "🌍 CI Environment:"
echo "  Server: $CI_SERVER_HOST"
echo "  Project: $CI_PROJECT_PATH"
echo "  Branch: $CI_COMMIT_BRANCH"
echo "  Token Type: $TOKEN_TYPE"
echo ""

# Configure git
echo "⚙️  Configuring Git..."
git config --global user.email "ci@ocf.soli.fr"
git config --global user.name "OCF CI"

# Try to fetch tags
echo "📥 Fetching tags..."
if git fetch --tags 2>&1; then
  echo "  ✅ Successfully fetched tags"
else
  echo "  ❌ Failed to fetch tags"
fi
echo ""

# List current tags
echo "🏷️  Current tags:"
git tag -l | head -10
echo ""

# Create a test tag
TEST_TAG="ci-test-$(date +%s)"
echo "🧪 Creating test tag: $TEST_TAG"
git tag "$TEST_TAG"
echo "  ✅ Test tag created locally"
echo ""

# Configure remote with token
echo "🔐 Configuring remote with authentication..."
git remote set-url origin "https://gitlab-ci-token:${TOKEN}@${CI_SERVER_HOST}/${CI_PROJECT_PATH}.git"
echo "  ✅ Remote configured"
echo ""

# Try to push the test tag
echo "📤 Attempting to push test tag..."
if git push origin "$TEST_TAG" 2>&1; then
  echo "  ✅ Successfully pushed test tag!"
  echo ""
  echo "🎉 Tag push is working correctly!"
  echo ""
  echo "Cleaning up test tag..."
  git push --delete origin "$TEST_TAG" 2>/dev/null || true
  git tag -d "$TEST_TAG"
else
  EXIT_CODE=$?
  echo "  ❌ Failed to push test tag (exit code: $EXIT_CODE)"
  echo ""
  echo "🔧 Troubleshooting Steps:"
  echo ""
  echo "1. Configure CI_PUSH_TOKEN:"
  echo "   - Go to: Settings → Access Tokens"
  echo "   - Name: CI_PUSH_TOKEN"
  echo "   - Role: Maintainer"
  echo "   - Scopes: write_repository"
  echo "   - Add as CI/CD variable: Settings → CI/CD → Variables"
  echo ""
  echo "2. Check Protected Tags:"
  echo "   - Go to: Settings → Repository → Protected tags"
  echo "   - Allow: Maintainers + No one (or configure for CI)"
  echo ""
  echo "3. Verify Token Permissions:"
  echo "   - Current token type: $TOKEN_TYPE"
  echo "   - CI_JOB_TOKEN has limited permissions"
  echo "   - Use CI_PUSH_TOKEN with write_repository scope"
  echo ""

  # Try to get more details about the error
  echo "📊 Detailed error information:"
  git push origin "$TEST_TAG" 2>&1 | grep -E "(error|denied|protected|permission)" || echo "  No specific error details available"

  exit 1
fi

echo "✅ Diagnosis complete - Everything working!"
