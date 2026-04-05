-- =============================================================================
-- TEST DATA: Org-Context-Aware Plan Resolution + Backend Routing
-- =============================================================================
-- Run: docker compose exec -T postgres psql -U go_user -d go_db < scripts/seed_test_data_org_context.sql
--
-- This creates test data covering the persona scenarios:
--   Marc (freelance trainer) — personal paid plan + member of 2 client orgs
--   Sophie's FormaTech — training org with Pro plan, multiple trainers + learners
--   Nadia's ESITECH — school org with dedicated backend
--   Karim — learner in FormaTech only
--   Léa — student in ESITECH only
--
-- Prerequisites: Users must exist in Casdoor first. This script uses the
-- existing user IDs from the database. Adjust UUIDs if needed.
-- =============================================================================

BEGIN;

-- ---------------------------------------------------------------------------
-- 1. SUBSCRIPTION PLANS (3 tiers + 1 school-specific)
-- ---------------------------------------------------------------------------

-- Free plan (for personal orgs without payment)
INSERT INTO subscription_plans (id, name, description, priority, price_amount, currency, billing_interval,
    max_concurrent_terminals, max_session_duration_minutes, max_courses, allowed_machine_sizes,
    features, is_active, is_catalog, default_backend, allowed_backends,
    created_at, updated_at)
VALUES (
    'a0000000-0001-0000-0000-000000000001',
    'Free', 'Free plan with basic terminal access', 0, 0, 'eur', 'month',
    1, 30, 1, '["XS"]',
    '["machine_size_xs"]',
    true, true, '', '[]',
    NOW(), NOW()
) ON CONFLICT (id) DO UPDATE SET
    name = EXCLUDED.name, default_backend = EXCLUDED.default_backend,
    allowed_backends = EXCLUDED.allowed_backends, updated_at = NOW();

-- Trainer plan (Marc's personal plan — decent for course prep)
INSERT INTO subscription_plans (id, name, description, priority, price_amount, currency, billing_interval,
    max_concurrent_terminals, max_session_duration_minutes, max_courses, allowed_machine_sizes,
    features, is_active, is_catalog, default_backend, allowed_backends,
    created_at, updated_at)
VALUES (
    'a0000000-0001-0000-0000-000000000002',
    'Trainer', 'For independent trainers — course prep and small classes', 5, 1990, 'eur', 'month',
    3, 120, 10, '["XS","S","M"]',
    '["machine_size_xs","machine_size_s","machine_size_m","unlimited_courses","export"]',
    true, true, '', '[]',
    NOW(), NOW()
) ON CONFLICT (id) DO UPDATE SET
    name = EXCLUDED.name, default_backend = EXCLUDED.default_backend,
    allowed_backends = EXCLUDED.allowed_backends, updated_at = NOW();

-- Pro plan (for training orgs like FormaTech)
INSERT INTO subscription_plans (id, name, description, priority, price_amount, currency, billing_interval,
    max_concurrent_terminals, max_session_duration_minutes, max_courses, allowed_machine_sizes,
    features, is_active, is_catalog, default_backend, allowed_backends,
    created_at, updated_at)
VALUES (
    'a0000000-0001-0000-0000-000000000003',
    'Pro', 'For training organizations — full feature set', 20, 4900, 'eur', 'month',
    10, 240, -1, '["XS","S","M","L"]',
    '["machine_size_xs","machine_size_s","machine_size_m","machine_size_l","unlimited_courses","advanced_labs","export","custom_themes","group_management","multiple_groups","bulk_purchase"]',
    true, true, '', '[]',
    NOW(), NOW()
) ON CONFLICT (id) DO UPDATE SET
    name = EXCLUDED.name, default_backend = EXCLUDED.default_backend,
    allowed_backends = EXCLUDED.allowed_backends, updated_at = NOW();

-- School plan (for ESITECH — on-premise backend routing)
INSERT INTO subscription_plans (id, name, description, priority, price_amount, currency, billing_interval,
    max_concurrent_terminals, max_session_duration_minutes, max_courses, allowed_machine_sizes,
    features, is_active, is_catalog, default_backend, allowed_backends,
    created_at, updated_at)
VALUES (
    'a0000000-0001-0000-0000-000000000004',
    'School', 'For engineering schools — on-premise backends, large scale', 25, 9900, 'eur', 'month',
    5, 180, -1, '["XS","S","M","L","XL"]',
    '["machine_size_xs","machine_size_s","machine_size_m","machine_size_l","machine_size_xl","unlimited_courses","advanced_labs","export","custom_themes","group_management","multiple_groups","bulk_purchase","data_persistence","network_access"]',
    true, true, '', '[]',
    NOW(), NOW()
) ON CONFLICT (id) DO UPDATE SET
    name = EXCLUDED.name, default_backend = EXCLUDED.default_backend,
    allowed_backends = EXCLUDED.allowed_backends, updated_at = NOW();


-- ---------------------------------------------------------------------------
-- 2. FIX EXISTING PERSONAL ORGS (set organization_type correctly)
-- ---------------------------------------------------------------------------

UPDATE organizations
SET organization_type = 'personal'
WHERE name LIKE 'personal_%' AND organization_type = 'team';


-- ---------------------------------------------------------------------------
-- 3. ORGANIZATIONS (team orgs for the test scenarios)
-- ---------------------------------------------------------------------------

-- Use your own user ID (first user in the system) as the owner for setup
-- You can change ownership later via the UI

-- FormaTech — Sophie's training organism (Pro plan)
INSERT INTO organizations (id, name, display_name, description, owner_user_id,
    organization_type, is_personal, max_groups, max_members, is_active,
    allowed_backends, default_backend,
    created_at, updated_at)
VALUES (
    'b0000000-0001-0000-0000-000000000001',
    'formatech', 'FormaTech', 'Organisme de formation IT - Toulouse (Qualiopi)',
    '05497508-239c-4d84-b949-1741aae5b166',
    'team', false, 30, 100, true,
    '[]', '',
    NOW(), NOW()
) ON CONFLICT (id) DO UPDATE SET
    display_name = EXCLUDED.display_name, description = EXCLUDED.description, updated_at = NOW();

-- ESITECH — Nadia's engineering school (School plan, with dedicated backend)
INSERT INTO organizations (id, name, display_name, description, owner_user_id,
    organization_type, is_personal, max_groups, max_members, is_active,
    allowed_backends, default_backend,
    created_at, updated_at)
VALUES (
    'b0000000-0001-0000-0000-000000000002',
    'esitech', 'ESITECH Bordeaux', 'Ecole d''ingenieurs - Departement Informatique',
    '05497508-239c-4d84-b949-1741aae5b166',
    'team', false, 50, 300, true,
    '[]', '',
    NOW(), NOW()
) ON CONFLICT (id) DO UPDATE SET
    display_name = EXCLUDED.display_name, description = EXCLUDED.description, updated_at = NOW();

-- DataSkills — another training company Marc works for
INSERT INTO organizations (id, name, display_name, description, owner_user_id,
    organization_type, is_personal, max_groups, max_members, is_active,
    allowed_backends, default_backend,
    created_at, updated_at)
VALUES (
    'b0000000-0001-0000-0000-000000000003',
    'dataskills', 'DataSkills Academy', 'Formation Data & DevOps - Paris',
    '05497508-239c-4d84-b949-1741aae5b166',
    'team', false, 20, 50, true,
    '[]', '',
    NOW(), NOW()
) ON CONFLICT (id) DO UPDATE SET
    display_name = EXCLUDED.display_name, description = EXCLUDED.description, updated_at = NOW();


-- ---------------------------------------------------------------------------
-- 4. ORGANIZATION SUBSCRIPTIONS
-- ---------------------------------------------------------------------------

-- FormaTech has a Pro plan
INSERT INTO organization_subscriptions (id, organization_id, subscription_plan_id,
    stripe_customer_id, status, current_period_start, current_period_end, quantity,
    created_at, updated_at)
VALUES (
    'c0000000-0001-0000-0000-000000000001',
    'b0000000-0001-0000-0000-000000000001',  -- FormaTech
    'a0000000-0001-0000-0000-000000000003',  -- Pro plan
    'cus_test_formatech', 'active', NOW(), NOW() + INTERVAL '1 year', 20,
    NOW(), NOW()
) ON CONFLICT (id) DO UPDATE SET status = 'active', updated_at = NOW();

-- ESITECH has a School plan
INSERT INTO organization_subscriptions (id, organization_id, subscription_plan_id,
    stripe_customer_id, status, current_period_start, current_period_end, quantity,
    created_at, updated_at)
VALUES (
    'c0000000-0001-0000-0000-000000000002',
    'b0000000-0001-0000-0000-000000000002',  -- ESITECH
    'a0000000-0001-0000-0000-000000000004',  -- School plan
    'cus_test_esitech', 'active', NOW(), NOW() + INTERVAL '1 year', 200,
    NOW(), NOW()
) ON CONFLICT (id) DO UPDATE SET status = 'active', updated_at = NOW();

-- DataSkills has a Pro plan
INSERT INTO organization_subscriptions (id, organization_id, subscription_plan_id,
    stripe_customer_id, status, current_period_start, current_period_end, quantity,
    created_at, updated_at)
VALUES (
    'c0000000-0001-0000-0000-000000000003',
    'b0000000-0001-0000-0000-000000000003',  -- DataSkills
    'a0000000-0001-0000-0000-000000000003',  -- Pro plan
    'cus_test_dataskills', 'active', NOW(), NOW() + INTERVAL '1 year', 15,
    NOW(), NOW()
) ON CONFLICT (id) DO UPDATE SET status = 'active', updated_at = NOW();


-- ---------------------------------------------------------------------------
-- 5. ORGANIZATION MEMBERS
--    Using existing user IDs from the system. Your main user (05497508...)
--    plays "Marc" (owner of personal + member of FormaTech, ESITECH, DataSkills)
--
--    Other existing users are assigned roles:
--    - 967f7f4e... = "Karim" (learner at FormaTech)
--    - 87fa3aaa... = "Léa" (student at ESITECH)
--    - 3e11cef2... = "Jean-Pierre" (vacataire at ESITECH + FormaTech)
--    - fa72e1f9... = "Sophie" (manager at FormaTech)
--    - ef95a311... = "Nadia" (manager at ESITECH)
-- ---------------------------------------------------------------------------

-- Marc (your user) is already owner of FormaTech, ESITECH, DataSkills (created above)
-- Add as owner member of all 3 orgs
INSERT INTO organization_members (id, organization_id, user_id, role, invited_by, joined_at, is_active, created_at, updated_at)
VALUES
    (gen_random_uuid(), 'b0000000-0001-0000-0000-000000000001', '05497508-239c-4d84-b949-1741aae5b166', 'owner', '05497508-239c-4d84-b949-1741aae5b166', NOW(), true, NOW(), NOW()),
    (gen_random_uuid(), 'b0000000-0001-0000-0000-000000000002', '05497508-239c-4d84-b949-1741aae5b166', 'owner', '05497508-239c-4d84-b949-1741aae5b166', NOW(), true, NOW(), NOW()),
    (gen_random_uuid(), 'b0000000-0001-0000-0000-000000000003', '05497508-239c-4d84-b949-1741aae5b166', 'owner', '05497508-239c-4d84-b949-1741aae5b166', NOW(), true, NOW(), NOW())
ON CONFLICT DO NOTHING;

-- Sophie (fa72e1f9...) — manager at FormaTech
INSERT INTO organization_members (id, organization_id, user_id, role, invited_by, joined_at, is_active, created_at, updated_at)
VALUES (gen_random_uuid(), 'b0000000-0001-0000-0000-000000000001', 'fa72e1f9-882e-424f-a349-6c75f82128b3', 'manager', '05497508-239c-4d84-b949-1741aae5b166', NOW(), true, NOW(), NOW())
ON CONFLICT DO NOTHING;

-- Nadia (ef95a311...) — manager at ESITECH
INSERT INTO organization_members (id, organization_id, user_id, role, invited_by, joined_at, is_active, created_at, updated_at)
VALUES (gen_random_uuid(), 'b0000000-0001-0000-0000-000000000002', 'ef95a311-696a-4b85-9b9d-c1979c7e611f', 'manager', '05497508-239c-4d84-b949-1741aae5b166', NOW(), true, NOW(), NOW())
ON CONFLICT DO NOTHING;

-- Jean-Pierre (3e11cef2...) — member of BOTH FormaTech and ESITECH (vacataire)
INSERT INTO organization_members (id, organization_id, user_id, role, invited_by, joined_at, is_active, created_at, updated_at)
VALUES
    (gen_random_uuid(), 'b0000000-0001-0000-0000-000000000001', '3e11cef2-0451-4d29-ba00-bddc0c84429d', 'member', '05497508-239c-4d84-b949-1741aae5b166', NOW(), true, NOW(), NOW()),
    (gen_random_uuid(), 'b0000000-0001-0000-0000-000000000002', '3e11cef2-0451-4d29-ba00-bddc0c84429d', 'member', '05497508-239c-4d84-b949-1741aae5b166', NOW(), true, NOW(), NOW())
ON CONFLICT DO NOTHING;

-- Karim (967f7f4e...) — learner at FormaTech only
INSERT INTO organization_members (id, organization_id, user_id, role, invited_by, joined_at, is_active, created_at, updated_at)
VALUES (gen_random_uuid(), 'b0000000-0001-0000-0000-000000000001', '967f7f4e-424a-408d-ad34-8c29f7c358e4', 'member', '05497508-239c-4d84-b949-1741aae5b166', NOW(), true, NOW(), NOW())
ON CONFLICT DO NOTHING;

-- Léa (87fa3aaa...) — student at ESITECH only
INSERT INTO organization_members (id, organization_id, user_id, role, invited_by, joined_at, is_active, created_at, updated_at)
VALUES (gen_random_uuid(), 'b0000000-0001-0000-0000-000000000002', '87fa3aaa-4179-4e85-bda5-00386cf97a63', 'member', '05497508-239c-4d84-b949-1741aae5b166', NOW(), true, NOW(), NOW())
ON CONFLICT DO NOTHING;


-- ---------------------------------------------------------------------------
-- 6. PERSONAL SUBSCRIPTIONS (for users with personal plans)
-- ---------------------------------------------------------------------------

-- Marc (your user) has a Trainer personal plan
INSERT INTO user_subscriptions (id, user_id, subscription_plan_id, subscription_type,
    status, current_period_start, current_period_end, created_at, updated_at)
VALUES (
    'd0000000-0001-0000-0000-000000000001',
    '05497508-239c-4d84-b949-1741aae5b166',
    'a0000000-0001-0000-0000-000000000002',  -- Trainer plan
    'personal', 'active', NOW(), NOW() + INTERVAL '1 month',
    NOW(), NOW()
) ON CONFLICT (id) DO UPDATE SET status = 'active', subscription_plan_id = EXCLUDED.subscription_plan_id, updated_at = NOW();

-- Jean-Pierre has a Free personal plan
INSERT INTO user_subscriptions (id, user_id, subscription_plan_id, subscription_type,
    status, current_period_start, current_period_end, created_at, updated_at)
VALUES (
    'd0000000-0001-0000-0000-000000000002',
    '3e11cef2-0451-4d29-ba00-bddc0c84429d',
    'a0000000-0001-0000-0000-000000000001',  -- Free plan
    'personal', 'active', NOW(), NOW() + INTERVAL '1 year',
    NOW(), NOW()
) ON CONFLICT (id) DO UPDATE SET status = 'active', subscription_plan_id = EXCLUDED.subscription_plan_id, updated_at = NOW();

-- Karim has NO personal subscription (only FormaTech org)
-- Léa has NO personal subscription (only ESITECH org)
-- Sophie has a Free personal plan
-- Nadia has a Free personal plan


-- ---------------------------------------------------------------------------
-- 7. CLASS GROUPS (to test group visibility per org)
-- ---------------------------------------------------------------------------

-- FormaTech groups
INSERT INTO class_groups (id, name, display_name, description, owner_user_id,
    organization_id, max_members, is_active, created_at, updated_at)
VALUES
    ('e0000000-0001-0000-0000-000000000001', 'docker-acme-mar2026', 'Docker - Acme Corp - Mars 2026',
     'Formation Docker pour Acme Corp', '05497508-239c-4d84-b949-1741aae5b166',
     'b0000000-0001-0000-0000-000000000001', 12, true, NOW(), NOW()),
    ('e0000000-0001-0000-0000-000000000002', 'k8s-bigco-apr2026', 'Kubernetes - BigCo - Avril 2026',
     'Formation Kubernetes avancee', '05497508-239c-4d84-b949-1741aae5b166',
     'b0000000-0001-0000-0000-000000000001', 10, true, NOW(), NOW())
ON CONFLICT (id) DO NOTHING;

-- ESITECH groups
INSERT INTO class_groups (id, name, display_name, description, owner_user_id,
    organization_id, max_members, is_active, created_at, updated_at)
VALUES
    ('e0000000-0001-0000-0000-000000000003', 'esitech-4a-td1', '4A DevOps - TD1',
     'Groupe TD1 - 4eme annee specialite DevOps', '05497508-239c-4d84-b949-1741aae5b166',
     'b0000000-0001-0000-0000-000000000002', 30, true, NOW(), NOW()),
    ('e0000000-0001-0000-0000-000000000004', 'esitech-4a-td2', '4A DevOps - TD2',
     'Groupe TD2 - 4eme annee specialite DevOps', '05497508-239c-4d84-b949-1741aae5b166',
     'b0000000-0001-0000-0000-000000000002', 30, true, NOW(), NOW()),
    ('e0000000-0001-0000-0000-000000000005', 'esitech-fc-docker', 'FC Docker - Session Avril',
     'Formation continue Docker (3 jours)', '05497508-239c-4d84-b949-1741aae5b166',
     'b0000000-0001-0000-0000-000000000002', 15, true, NOW(), NOW())
ON CONFLICT (id) DO NOTHING;

-- DataSkills group
INSERT INTO class_groups (id, name, display_name, description, owner_user_id,
    organization_id, max_members, is_active, created_at, updated_at)
VALUES
    ('e0000000-0001-0000-0000-000000000006', 'ds-ansible-apr2026', 'Ansible - Avril 2026',
     'Formation Ansible pour DataSkills', '05497508-239c-4d84-b949-1741aae5b166',
     'b0000000-0001-0000-0000-000000000003', 8, true, NOW(), NOW())
ON CONFLICT (id) DO NOTHING;


-- ---------------------------------------------------------------------------
-- 8. GROUP MEMBERS
-- ---------------------------------------------------------------------------

-- Karim in FormaTech Docker group
INSERT INTO group_members (id, group_id, user_id, role, invited_by, joined_at, is_active, created_at, updated_at)
VALUES (gen_random_uuid(), 'e0000000-0001-0000-0000-000000000001', '967f7f4e-424a-408d-ad34-8c29f7c358e4', 'member', '05497508-239c-4d84-b949-1741aae5b166', NOW(), true, NOW(), NOW())
ON CONFLICT DO NOTHING;

-- Léa in ESITECH TD1 group
INSERT INTO group_members (id, group_id, user_id, role, invited_by, joined_at, is_active, created_at, updated_at)
VALUES (gen_random_uuid(), 'e0000000-0001-0000-0000-000000000003', '87fa3aaa-4179-4e85-bda5-00386cf97a63', 'member', '05497508-239c-4d84-b949-1741aae5b166', NOW(), true, NOW(), NOW())
ON CONFLICT DO NOTHING;

-- Jean-Pierre as group owner in FormaTech Docker group + ESITECH TD1
INSERT INTO group_members (id, group_id, user_id, role, invited_by, joined_at, is_active, created_at, updated_at)
VALUES
    (gen_random_uuid(), 'e0000000-0001-0000-0000-000000000001', '3e11cef2-0451-4d29-ba00-bddc0c84429d', 'owner', '05497508-239c-4d84-b949-1741aae5b166', NOW(), true, NOW(), NOW()),
    (gen_random_uuid(), 'e0000000-0001-0000-0000-000000000003', '3e11cef2-0451-4d29-ba00-bddc0c84429d', 'owner', '05497508-239c-4d84-b949-1741aae5b166', NOW(), true, NOW(), NOW())
ON CONFLICT DO NOTHING;

-- Marc as owner of all groups (already org owner, adding as group owner for clarity)
INSERT INTO group_members (id, group_id, user_id, role, invited_by, joined_at, is_active, created_at, updated_at)
VALUES
    (gen_random_uuid(), 'e0000000-0001-0000-0000-000000000001', '05497508-239c-4d84-b949-1741aae5b166', 'owner', '05497508-239c-4d84-b949-1741aae5b166', NOW(), true, NOW(), NOW()),
    (gen_random_uuid(), 'e0000000-0001-0000-0000-000000000002', '05497508-239c-4d84-b949-1741aae5b166', 'owner', '05497508-239c-4d84-b949-1741aae5b166', NOW(), true, NOW(), NOW()),
    (gen_random_uuid(), 'e0000000-0001-0000-0000-000000000003', '05497508-239c-4d84-b949-1741aae5b166', 'owner', '05497508-239c-4d84-b949-1741aae5b166', NOW(), true, NOW(), NOW()),
    (gen_random_uuid(), 'e0000000-0001-0000-0000-000000000004', '05497508-239c-4d84-b949-1741aae5b166', 'owner', '05497508-239c-4d84-b949-1741aae5b166', NOW(), true, NOW(), NOW()),
    (gen_random_uuid(), 'e0000000-0001-0000-0000-000000000005', '05497508-239c-4d84-b949-1741aae5b166', 'owner', '05497508-239c-4d84-b949-1741aae5b166', NOW(), true, NOW(), NOW()),
    (gen_random_uuid(), 'e0000000-0001-0000-0000-000000000006', '05497508-239c-4d84-b949-1741aae5b166', 'owner', '05497508-239c-4d84-b949-1741aae5b166', NOW(), true, NOW(), NOW())
ON CONFLICT DO NOTHING;

COMMIT;

-- =============================================================================
-- VERIFICATION QUERIES
-- =============================================================================

-- Plans overview
SELECT name, priority, price_amount/100.0 as price_eur, max_concurrent_terminals, features
FROM subscription_plans ORDER BY priority;

-- Orgs with their plans
SELECT o.display_name, o.organization_type,
       sp.name as plan_name, os.status
FROM organizations o
LEFT JOIN organization_subscriptions os ON os.organization_id = o.id AND os.status = 'active'
LEFT JOIN subscription_plans sp ON sp.id = os.subscription_plan_id
ORDER BY o.organization_type, o.display_name;

-- Members per org
SELECT o.display_name, om.user_id, om.role
FROM organization_members om
JOIN organizations o ON o.id = om.organization_id
WHERE om.is_active = true
ORDER BY o.display_name, om.role;

-- Groups per org
SELECT o.display_name as org, cg.display_name as group_name
FROM class_groups cg
JOIN organizations o ON o.id = cg.organization_id
WHERE cg.is_active = true
ORDER BY o.display_name, cg.display_name;

-- Personal subscriptions
SELECT us.user_id, sp.name as plan_name, us.status
FROM user_subscriptions us
JOIN subscription_plans sp ON sp.id = us.subscription_plan_id
WHERE us.status = 'active'
ORDER BY us.user_id;
