#!/bin/bash
set -e

# Wait for all required services to be ready (PostgreSQL, MySQL, Casdoor)
# This script is used in the auth test job to ensure all dependencies are running
# before executing tests.
#
# Manual service startup order is required because:
# 1. Casdoor depends on MySQL being fully initialized
# 2. docker-compose doesn't guarantee startup order
# 3. Starting all at once causes Casdoor connection failures

wait_for_postgres() {
  echo "⏳ Waiting for PostgreSQL to be ready..."
  for i in $(seq 1 30); do
    if docker exec ocf-postgres-test pg_isready -U postgres 2>/dev/null; then
      echo "✅ PostgreSQL is ready!"
      return 0
    fi

    if [ $i -eq 30 ]; then
      echo "❌ PostgreSQL failed to start"
      docker-compose -f docker-compose.test.yml logs postgres-test
      return 1
    fi

    sleep 1
  done
}

wait_for_mysql() {
  echo "⏳ Waiting for MySQL (Casdoor DB) to be ready..."
  for i in $(seq 1 30); do
    if docker exec ocf-casdoor-db-test mysqladmin ping -h localhost --silent 2>/dev/null; then
      echo "✅ MySQL is ready!"
      return 0
    fi

    if [ $i -eq 30 ]; then
      echo "❌ MySQL failed to start"
      docker-compose -f docker-compose.test.yml logs casdoor-db-test
      return 1
    fi

    sleep 1
  done
}

wait_for_casdoor() {
  echo "⏳ Waiting for Casdoor to be ready..."
  for i in $(seq 1 90); do
    if docker exec ocf-casdoor-test wget --spider -q http://localhost:8000/api/get-account 2>/dev/null; then
      echo "✅ Casdoor is ready!"
      return 0
    fi

    if [ $i -eq 90 ]; then
      echo "❌ Casdoor failed to start"
      docker-compose -f docker-compose.test.yml logs casdoor-test
      docker-compose -f docker-compose.test.yml logs casdoor-db-test
      return 1
    fi

    sleep 2
  done
}

# Main execution
main() {
  wait_for_postgres || exit 1
  wait_for_mysql || exit 1
  wait_for_casdoor || exit 1

  echo "🎉 All services are ready!"
}

main
