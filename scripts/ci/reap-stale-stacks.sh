#!/bin/bash
# Remove every compose stack left on the Docker daemon by earlier test:auth
# jobs whose after_script never ran (cancelled, timed out, runner crash):
# containers with their anonymous volumes, networks, named volumes, images.
# On a runner that exposes the host socket, that daemon is the host's.
#
# Safe only because test:auth holds `resource_group: dind-integration`: while
# it runs, no other job owns an object carrying the prefix. Run it BEFORE `up`.
# Same script as ocf-front's e2e/ci/reap-stale-stacks.sh — keep them in step.
#
# Usage: reap-stale-stacks.sh <project-prefix>
set -uo pipefail

PREFIX=${1:?usage: reap-stale-stacks.sh <project-prefix>}

docker ps -a --format '{{.ID}} {{.Label "com.docker.compose.project"}}' \
  | awk -v p="$PREFIX" 'index($2, p) == 1 { print $1 }' \
  | xargs -r docker rm -f -v
docker network ls --format '{{.Name}}' | grep "^${PREFIX}" | xargs -r docker network rm
docker volume ls --format '{{.Name}}' | grep "^${PREFIX}" | xargs -r docker volume rm
docker images --format '{{.Repository}}:{{.Tag}}' | grep "^${PREFIX}" | xargs -r docker rmi -f

# Never fail the job: a leftover we cannot remove is not this job's problem.
exit 0
