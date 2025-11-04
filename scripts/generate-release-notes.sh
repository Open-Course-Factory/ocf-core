#!/bin/sh

set -e

if [ -z "$1" ]; then
  echo "Usage: $0 <tag>"
  exit 1
fi

TAG=$1
LAST_TAG=$(git describe --tags --abbrev=0 $TAG~1 2>/dev/null || git rev-list --max-parents=0 HEAD)

if [ -z "$LAST_TAG" ]; then
  COMMITS=$(git log --pretty=format:"%s" $TAG)
else
  COMMITS=$(git log --pretty=format:"%s" $LAST_TAG..$TAG)
fi

FEAT=""
FIX=""
DOCS=""
CHORE=""
REFACTOR=""
TEST=""
OTHER=""

while IFS= read -r commit; do
  case "$commit" in
    feat*) if [ -z "$FEAT" ]; then FEAT="* $commit"; else FEAT="$FEAT\n* $commit"; fi ;;
    fix*) if [ -z "$FIX" ]; then FIX="* $commit"; else FIX="$FIX\n* $commit"; fi ;;
    docs*) if [ -z "$DOCS" ]; then DOCS="* $commit"; else DOCS="$DOCS\n* $commit"; fi ;;
    chore*) if [ -z "$CHORE" ]; then CHORE="* $commit"; else CHORE="$CHORE\n* $commit"; fi ;;
    refactor*) if [ -z "$REFACTOR" ]; then REFACTOR="* $commit"; else REFACTOR="$REFACTOR\n* $commit"; fi ;;
    test*) if [ -z "$TEST" ]; then TEST="* $commit"; else TEST="$TEST\n* $commit"; fi ;;
    *) if [ -z "$OTHER" ]; then OTHER="* $commit"; else OTHER="$OTHER\n* $commit"; fi ;;
  esac
done <<EOF
$COMMITS
EOF

if [ -n "$FEAT" ]; then
  echo "### Features"
  printf "%b\n" "$FEAT"
  echo ""
fi

if [ -n "$FIX" ]; then
  echo "### Bug Fixes"
  printf "%b\n" "$FIX"
  echo ""
fi

if [ -n "$DOCS" ]; then
  echo "### Documentation"
  printf "%b\n" "$DOCS"
  echo ""
fi

if [ -n "$CHORE" ]; then
  echo "### Chores"
  printf "%b\n" "$CHORE"
  echo ""
fi

if [ -n "$REFACTOR" ]; then
  echo "### Refactoring"
  printf "%b\n" "$REFACTOR"
  echo ""
fi

if [ -n "$TEST" ]; then
  echo "### Tests"
  printf "%b\n" "$TEST"
  echo ""
fi

if [ -n "$OTHER" ]; then
  echo "### Other"
  printf "%b\n" "$OTHER"
  echo ""
fi
