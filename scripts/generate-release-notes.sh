#!/bin/sh

set -e

echo "Generating release notes..."

if [ -z "$1" ]; then
  echo "Usage: $0 <tag>"
  exit 1
fi

TAG=$1
LAST_TAG=$(git describe --tags --abbrev=0 $TAG~1 2>/dev/null || git rev-list --max-parents=0 HEAD)

echo "TAG: $TAG"
echo "LAST_TAG: $LAST_TAG"

if [ -z "$LAST_TAG" ]; then
  COMMITS=$(git log --pretty=format:"%s" $TAG)
else
  COMMITS=$(git log --pretty=format:"%s" $LAST_TAG..$TAG)
fi

echo "COMMITS:"
echo "$COMMITS"
echo "----"

FEAT=""
FIX=""
DOCS=""
CHORE=""
REFACTOR=""
TEST=""
OTHER=""

while IFS= read -r commit; do
  case "$commit" in
    feat*) FEAT="$FEAT\n* $commit" ;;
    fix*) FIX="$FIX\n* $commit" ;;
    docs*) DOCS="$DOCS\n* $commit" ;;
    chore*) CHORE="$CHORE\n* $commit" ;;
    refactor*) REFACTOR="$REFACTOR\n* $commit" ;;
    test*) TEST="$TEST\n* $commit" ;;
    *) OTHER="$OTHER\n* $commit" ;;
  esac
done <<EOF
$COMMITS
EOF

if [ -n "$FEAT" ]; then
  echo "### Features"
  echo "$FEAT"
  echo ""
fi

if [ -n "$FIX" ]; then
  echo "### Bug Fixes"
  echo "$FIX"
  echo ""
fi

if [ -n "$DOCS" ]; then
  echo "### Documentation"
  echo "$DOCS"
  echo ""
fi

if [ -n "$CHORE" ]; then
  echo "### Chores"
  echo "$CHORE"
  echo ""
fi

if [ -n "$REFACTOR" ]; then
  echo "### Refactoring"
  echo "$REFACTOR"
  echo ""
fi

if [ -n "$TEST" ]; then
  echo "### Tests"
  echo "$TEST"
  echo ""
fi

if [ -n "$OTHER" ]; then
  echo "### Other"
  echo "$OTHER"
  echo ""
fi
