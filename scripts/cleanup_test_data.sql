-- cleanup_test_data.sql (ТЗ 04.07.2026, п.10.c)
--
-- ⚠️  РУЧНОЙ скрипт очистки ТЕСТОВЫХ данных перед боевым запуском.
--     НЕ является миграцией и НЕ запускается автоматически.
--
-- Запуск (после проверки списка ниже!):
--   psql "$DATABASE_URL" -f scripts/cleanup_test_data.sql
--
-- Сначала посмотрите, что будет удалено:
--   SELECT id, name, display_name FROM clients
--   WHERE name ILIKE '%smoke%' OR display_name ILIKE '%smoke%'
--      OR name ILIKE 'test%' OR display_name ILIKE 'test%'
--      OR name ILIKE '%testov%' OR name ILIKE 'юр боб%' OR name ILIKE 'юр ком%';

BEGIN;

-- 1. Тестовые клиенты (SmsSmoke*, Smoke*, test testov, юр боб/ком и т.п.)
CREATE TEMP TABLE _test_clients AS
SELECT id FROM clients
WHERE name ILIKE '%smoke%' OR display_name ILIKE '%smoke%'
   OR name ILIKE 'test%'   OR display_name ILIKE 'test%'
   OR name ILIKE '%testov%'
   OR email ILIKE '%@example.com'
   OR name ILIKE 'юр боб%' OR name ILIKE 'юр ком%'
   OR name ILIKE '%testik%' OR display_name ILIKE '%testik%';

-- 2. Сделки и документы этих клиентов
DELETE FROM documents WHERE client_id IN (SELECT id FROM _test_clients)
   OR deal_id IN (SELECT id FROM deals WHERE client_id IN (SELECT id FROM _test_clients));
DELETE FROM deals WHERE client_id IN (SELECT id FROM _test_clients);

-- 3. Тестовые лиды (smoke/test в названии или описании)
DELETE FROM leads
WHERE title ILIKE '%smoke%' OR title ILIKE '%test%'
   OR description ILIKE '%smoke%';

-- 4. Сами клиенты с профилями (каскад по FK)
DELETE FROM client_individual_profiles WHERE client_id IN (SELECT id FROM _test_clients);
DELETE FROM client_legal_profiles     WHERE client_id IN (SELECT id FROM _test_clients);
DELETE FROM clients WHERE id IN (SELECT id FROM _test_clients);

-- 5. Тестовые документы-черновики без клиента и сделки (fff, dok, 0 из демо)
DELETE FROM documents
WHERE client_id IS NULL AND deal_id IS NULL
  AND status = 'draft'
  AND (doc_type IN ('fff', 'dok', '0') OR title IN ('fff', 'dok', '0'));

-- 6. Тестовые пользователи (test@gmail.com, sms.smoke...)
--    Пользователей НЕ удаляем автоматически — заблокируйте вручную через
--    интерфейс (Пользователи → Заблокированные), чтобы не потерять связи.

COMMIT;

-- После очистки проверьте счётчики:
--   SELECT COUNT(*) FROM clients; SELECT COUNT(*) FROM deals; SELECT COUNT(*) FROM leads;
