-- Dev-only seed helper for manual smoke checks.
-- Do NOT use in production.

-- promote first user to management for local smoke
UPDATE users SET role_id = 40 WHERE id = 1;

-- optional sample individual client
INSERT INTO clients (owner_id, client_type, display_name, primary_phone, primary_email, name, phone, email, created_at, updated_at)
SELECT 1, 'individual', 'Smoke Individual', '77001110001', 'smoke.individual@local.dev', 'Smoke Individual', '77001110001', 'smoke.individual@local.dev', NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM clients WHERE display_name = 'Smoke Individual');
