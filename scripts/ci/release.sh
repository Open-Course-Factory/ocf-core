#!/usr/bin/env bash
# Commits a release version bump straight to main, from a main pipeline. That
# commit's own pipeline builds the image and check-version tags it.
# Usage: release.sh patch|minor
set -euo pipefail

level="$1"

git fetch --quiet origin main
# Releasing from an older pipeline would ship commits it never built.
if [ "$(git rev-parse origin/main)" != "$CI_COMMIT_SHA" ]; then
  echo "main moved since this pipeline ran: release from the newest main pipeline." >&2
  exit 1
fi

IFS=. read -r major minor patch < VERSION
case "$level" in
  patch) patch=$((patch + 1)) ;;
  minor) minor=$((minor + 1)); patch=0 ;;
  *) echo "unknown release level: $level" >&2; exit 1 ;;
esac
version="$major.$minor.$patch"
echo "$version" > VERSION

git config user.name "GitLab CI"
git config user.email "ci@gitlab.local"
git commit -m "chore: release $version" -- VERSION
git push "https://gitlab-ci-token:${CI_PUSH_TOKEN}@${CI_SERVER_HOST}/${CI_PROJECT_PATH}.git" "HEAD:refs/heads/main"
echo "Released $version: its main pipeline builds and tags it."
