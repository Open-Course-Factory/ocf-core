#!/bin/bash
# =============================================================================
# Setup multi-backend test environment for org-context-aware plan routing
# =============================================================================
# Business model:
#   - Trial (free, auto-assigned on registration) → free-infra backend
#   - Any paid plan (Trainer, Pro, School)        → premium-infra backend
#   - School with contract (ESITECH)              → esitech-dedicated backend (org-level)
#
# All 3 backends point to the SAME Incus daemon, using different projects.
# Run from: /home/tom/soli/ocf/ocf-core
# =============================================================================

set -e

TT_ADMIN_URL="${TT_ADMIN_URL:-http://localhost:8090}"
TT_ADMIN_KEY="${TT_ADMIN_KEY:-ocf-dev-admin-key}"
DB_USER="go_user"
DB_NAME="go_db"

echo "=== Step 1: Create Incus projects ==="

for project in free-infra premium-infra esitech-dedicated; do
    if incus project show "$project" &>/dev/null; then
        echo "  Project '$project' already exists"
    else
        echo "  Creating project '$project'"
        incus project create "$project" \
            -c features.images=true \
            -c features.profiles=true \
            -c features.storage.volumes=true \
            -c features.networks=false
        incus profile show default | incus profile edit default --project "$project"
        echo "  Done"
    fi
done

echo ""
echo "=== Step 2: Copy base instances into each project ==="

for project in free-infra premium-infra esitech-dedicated; do
    if incus info alp --project OCFDemo &>/dev/null; then
        if ! incus info alp --project "$project" &>/dev/null; then
            echo "  Copying 'alp' to '$project'"
            incus copy alp alp --project OCFDemo --target-project "$project" 2>/dev/null || \
                echo "  Warning: could not copy 'alp' to '$project'"
        else
            echo "  'alp' exists in '$project'"
        fi
    else
        echo "  Warning: 'alp' not found in OCFDemo, skipping copy"
    fi
done

echo ""
echo "=== Step 3: Read certs from existing 'default' backend ==="

# Fetch the existing default backend config via admin API
DEFAULT_BACKEND=$(curl -s "$TT_ADMIN_URL/1.0/admin/backends" \
    -H "X-API-Key: $TT_ADMIN_KEY")

# Extract certs and URL using python3 (no pyyaml needed, just json)
eval $(echo "$DEFAULT_BACKEND" | python3 -c "
import sys, json
data = json.load(sys.stdin)
backends = data.get('data', data) if isinstance(data, dict) else data
b = backends[0]
# Write shell variables, escaping for JSON embedding
print(f'SERVER_URL={json.dumps(b[\"server_url\"])}')
print(f'SERVER_CERT={json.dumps(b[\"server_certificate\"])}')
print(f'CLIENT_CERT={json.dumps(b[\"client_certificate\"])}')
")

# Client key is not returned by the API (security), read it from config.yaml
CLIENT_KEY=$(python3 -c "
import re
content = open('/home/tom/soli/ocf/tt-backend/config.yaml').read()
# Find the key block under incus.client
m = re.search(r'incus:.*?client:.*?key: \|-\n((?:\s+.*\n)+)', content, re.DOTALL)
if m:
    import json
    key = ''.join(line.strip() + '\n' for line in m.group(1).strip().split('\n'))
    print(json.dumps(key.strip()))
")

echo "  Server URL: $SERVER_URL"
echo "  Certs extracted successfully"

echo ""
echo "=== Step 4: Register backends in tt-backend ==="

create_backend() {
    local id="$1" name="$2" project="$3"

    existing=$(curl -s "$TT_ADMIN_URL/1.0/admin/backends" -H "X-API-Key: $TT_ADMIN_KEY")
    if echo "$existing" | grep -q "\"backend_id\":\"$id\""; then
        echo "  Backend '$id' already exists"
        return
    fi

    echo "  Creating backend '$id' (project: $project)"
    curl -s -X POST "$TT_ADMIN_URL/1.0/admin/backends" \
        -H "X-API-Key: $TT_ADMIN_KEY" \
        -H "Content-Type: application/json" \
        -d "{
            \"backend_id\": \"$id\",
            \"name\": \"$name\",
            \"server_url\": $SERVER_URL,
            \"client_certificate\": $CLIENT_CERT,
            \"client_key\": $CLIENT_KEY,
            \"server_certificate\": $SERVER_CERT,
            \"project\": \"$project\",
            \"is_active\": true
        }" > /dev/null
    echo "  Created '$id'"
}

create_backend "free-infra" "Shared Infrastructure (Free)" "free-infra"
create_backend "premium-infra" "Premium Infrastructure (Paid)" "premium-infra"
create_backend "esitech-dedicated" "ESITECH Dedicated" "esitech-dedicated"

echo "  Reloading backends..."
curl -s -X POST "$TT_ADMIN_URL/1.0/admin/backends/reload" \
    -H "X-API-Key: $TT_ADMIN_KEY" > /dev/null

echo ""
echo "=== Step 5: Verify backends ==="
curl -s "$TT_ADMIN_URL/1.0/backends" -H "X-API-Key: $TT_ADMIN_KEY" 2>/dev/null | \
    python3 -c "import sys,json; [print(f'  {b[\"backend_id\"]:20s} connected={b[\"connected\"]}  default={b.get(\"is_default\",False)}  project={b.get(\"project\",\"?\")}') for b in json.load(sys.stdin).get('data', json.load(open('/dev/stdin')))]" 2>/dev/null || \
    curl -s http://localhost:8090/1.0/backends

echo ""
echo "=== Step 6: Update subscription plans + org backend config ==="

docker compose exec -T postgres psql -U "$DB_USER" -d "$DB_NAME" <<'SQL'

-- Trial plan (free, auto-assigned) → free-infra
UPDATE subscription_plans
SET default_backend = 'free-infra', allowed_backends = '["free-infra"]'
WHERE name = 'Trial';

-- Free plan → free-infra
UPDATE subscription_plans
SET default_backend = 'free-infra', allowed_backends = '["free-infra"]'
WHERE name = 'Free';

-- Trainer plan (paid) → premium-infra
UPDATE subscription_plans
SET default_backend = 'premium-infra', allowed_backends = '["premium-infra","free-infra"]'
WHERE name = 'Trainer';

-- Pro plan (paid) → premium-infra
UPDATE subscription_plans
SET default_backend = 'premium-infra', allowed_backends = '["premium-infra","free-infra"]'
WHERE name = 'Pro';

-- Member Pro (paid) → premium-infra
UPDATE subscription_plans
SET default_backend = 'premium-infra', allowed_backends = '["premium-infra","free-infra"]'
WHERE name = 'Member Pro';

-- School plan (paid) → premium-infra (schools override at org level)
UPDATE subscription_plans
SET default_backend = 'premium-infra', allowed_backends = '["premium-infra","free-infra"]'
WHERE name = 'School';

-- ESITECH org → dedicated backend (org-level override, takes precedence over plan)
UPDATE organizations
SET default_backend = 'esitech-dedicated',
    allowed_backends = '["esitech-dedicated"]'
WHERE name = 'esitech';

-- FormaTech and DataSkills have no org-level backend config → plan default applies

\echo ''
\echo '--- Plans with backend routing ---'
SELECT name, priority, default_backend, allowed_backends
FROM subscription_plans ORDER BY priority;

\echo ''
\echo '--- Orgs with backend config ---'
SELECT display_name, organization_type, default_backend, allowed_backends
FROM organizations
WHERE default_backend != '' OR (allowed_backends IS NOT NULL AND allowed_backends != '[]' AND allowed_backends != 'null')
ORDER BY display_name;

SQL

echo ""
echo "=========================================="
echo "  Setup complete! Test matrix:"
echo "=========================================="
echo ""
echo "  Persona         | Org context      | Plan      | Expected backend"
echo "  ─────────────────────────────────────────────────────────────────"
echo "  Any new user     | Personal (Trial) | Trial     | free-infra"
echo "  Marc             | Personal         | Trainer   | premium-infra"
echo "  Marc             | FormaTech        | Pro       | premium-infra (plan default)"
echo "  Marc             | DataSkills       | Pro       | premium-infra (plan default)"
echo "  Marc             | ESITECH          | School    | esitech-dedicated (org override)"
echo "  Karim            | Personal (Trial) | Trial     | free-infra"
echo "  Karim            | FormaTech        | Pro       | premium-infra"
echo "  Léa              | Personal (Trial) | Trial     | free-infra"
echo "  Léa              | ESITECH          | School    | esitech-dedicated"
echo "  Jean-Pierre      | Personal (Free)  | Free      | free-infra"
echo "  Jean-Pierre      | FormaTech        | Pro       | premium-infra"
echo "  Jean-Pierre      | ESITECH          | School    | esitech-dedicated"
echo ""
