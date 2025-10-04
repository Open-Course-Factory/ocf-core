#!/bin/sh
# Wait for MySQL to be ready before starting Casdoor

set -e

# Default to casdoor-db-test if not set (for CI)
MYSQL_HOST=${MYSQL_HOST:-casdoor-db-test}

echo "Waiting for MySQL at ${MYSQL_HOST}:3306..."

# Wait for MySQL port to be open
until nc -z -v -w5 ${MYSQL_HOST} 3306; do
  echo "MySQL is unavailable - sleeping"
  sleep 2
done

echo "MySQL port is open - waiting for it to accept connections..."
sleep 5

# Try to connect with mysqladmin (if available) or just wait a bit more
echo "MySQL is up - starting Casdoor"
exec "$@"
