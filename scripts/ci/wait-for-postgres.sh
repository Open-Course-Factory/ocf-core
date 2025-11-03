#!/bin/bash
set -e

# Wait for PostgreSQL service to be ready
# Environment variables required:
# - POSTGRES_HOST
# - POSTGRES_USER
# - POSTGRES_PASSWORD
# - POSTGRES_DB

echo "⏳ Waiting for PostgreSQL service to be ready..."

for i in $(seq 1 30); do
  if PGPASSWORD=$POSTGRES_PASSWORD psql -h $POSTGRES_HOST -U $POSTGRES_USER -d $POSTGRES_DB -c '\q' 2>/dev/null; then
    echo "✅ PostgreSQL is ready!"
    exit 0
  fi

  if [ $i -eq 30 ]; then
    echo "❌ PostgreSQL failed to start after 30 seconds"
    exit 1
  fi

  sleep 1
done
