#!/bin/sh
# Wait for MySQL to be ready before starting Casdoor

set -e

host="$1"
shift
cmd="$@"

echo "Waiting for MySQL at $host..."

until nc -z casdoor-db-test 3306; do
  >&2 echo "MySQL is unavailable - sleeping"
  sleep 1
done

>&2 echo "MySQL is up - executing command"
exec $cmd
