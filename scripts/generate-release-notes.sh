#!/bin/sh

set -e

if [ -z "$1" ]; then
  echo "Usage: $0 <tag>"
  exit 1
fi

TAG=$1

# Ensure we have all tags (important for CI shallow clones)
git fetch --tags 2>/dev/null || true

# Find previous version tag using proper version sorting
# This is more reliable than git describe, especially in shallow clones
LAST_TAG=$(git tag -l 'v*.*.*' | grep -v "^${TAG}$" | sort -V | tail -1)

if [ -z "$LAST_TAG" ]; then
  echo "No previous version tag found, generating notes from beginning"
  # For first release, use initial commit (limit history to avoid huge notes)
  FIRST_COMMIT=$(git rev-list --max-parents=0 HEAD)
  COMMITS=$(git log --pretty=format:"%s" --max-count=100 ${FIRST_COMMIT}..HEAD)
else
  echo "Generating notes from $LAST_TAG to $TAG"
  # Use HEAD instead of TAG to work when tag doesn't exist locally yet
  COMMITS=$(git log --pretty=format:"%s" ${LAST_TAG}..HEAD)
fi

# Count commits for verification
COMMIT_COUNT=$(echo "$COMMITS" | grep -c . || echo "0")
echo "Found $COMMIT_COUNT commits to process"

FEAT=""
FIX=""
DOCS=""
CHORE=""
REFACTOR=""
TEST=""
CI=""
OTHER=""

while IFS= read -r commit; do
  case "$commit" in
    feat:*|feat\(*) FEAT="${FEAT:+$FEAT\n}* $commit" ;;
    fix:*|fix\(*) FIX="${FIX:+$FIX\n}* $commit" ;;
    docs:*|docs\(*) DOCS="${DOCS:+$DOCS\n}* $commit" ;;
    chore:*|chore\(*) CHORE="${CHORE:+$CHORE\n}* $commit" ;;
    refactor:*|refactor\(*) REFACTOR="${REFACTOR:+$REFACTOR\n}* $commit" ;;
    test:*|test\(*) TEST="${TEST:+$TEST\n}* $commit" ;;
    ci:*|ci\(*) CI="${CI:+$CI\n}* $commit" ;;
    *) OTHER="${OTHER:+$OTHER\n}* $commit" ;;
  esac
done <<EOF
$COMMITS
EOF

echo "## What's Changed"
echo ""

if [ -n "$FEAT" ]; then
  echo "### ✨ Features"
  printf "%b\n" "$FEAT"
  echo ""
fi

if [ -n "$FIX" ]; then
  echo "### 🐛 Bug Fixes"
  printf "%b\n" "$FIX"
  echo ""
fi

if [ -n "$REFACTOR" ]; then
  echo "### ♻️ Refactoring"
  printf "%b\n" "$REFACTOR"
  echo ""
fi

if [ -n "$TEST" ]; then
  echo "### 🧪 Tests"
  printf "%b\n" "$TEST"
  echo ""
fi

if [ -n "$CI" ]; then
  echo "### 🔧 CI/CD"
  printf "%b\n" "$CI"
  echo ""
fi

if [ -n "$DOCS" ]; then
  echo "### 📝 Documentation"
  printf "%b\n" "$DOCS"
  echo ""
fi

if [ -n "$CHORE" ]; then
  echo "### 🏗️ Chores"
  printf "%b\n" "$CHORE"
  echo ""
fi

if [ -n "$OTHER" ]; then
  echo "### 📦 Other Changes"
  printf "%b\n" "$OTHER"
  echo ""
fi

if [ -n "$LAST_TAG" ]; then
  echo "---"
  echo ""
  echo "**Full Changelog**: \`${LAST_TAG}...${TAG}\`"
fi
