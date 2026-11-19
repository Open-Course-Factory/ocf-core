#!/bin/sh
# Build a MySQL image whose datadir already holds Casdoor's schema and test init
# data, so test:auth skips MySQL init and Casdoor's first import (#677).
# Usage: build-casdoor-db-test.sh <casdoor-test image> <output image>
set -eu
CASDOOR_IMAGE=${1:?casdoor-test image}
OUT=${2:?output image}
NET=casdoor-preload-$$

docker network create "$NET" > /dev/null
# The datadir sits outside the image's /var/lib/mysql VOLUME, which docker commit drops.
docker run -d --name casdoor-db-test --network "$NET" \
  -e MYSQL_ROOT_PASSWORD=123456 -e MYSQL_DATABASE=casdoor \
  mysql:8.4.13 --datadir=/var/lib/mysql-preloaded > /dev/null
docker run -d --name casdoor-test --network "$NET" -e RUNNING_IN_DOCKER=true "$CASDOOR_IMAGE" > /dev/null

# Casdoor imports init_data before it starts serving.
if ! timeout 300 sh -c 'until docker exec casdoor-test wget -q --spider http://localhost:8000/api/get-account 2>/dev/null; do sleep 2; done'; then
  echo "Casdoor not ready after 300 s"; docker logs --tail 50 casdoor-test; docker logs --tail 50 casdoor-db-test; exit 1
fi

docker rm -f casdoor-test > /dev/null
# Clean InnoDB shutdown, so the committed datadir needs no crash recovery.
docker stop -t 120 casdoor-db-test > /dev/null
docker commit casdoor-db-test "$OUT" > /dev/null
docker rm casdoor-db-test > /dev/null
docker network rm "$NET" > /dev/null
