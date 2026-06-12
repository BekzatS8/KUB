-- =============================================================================
-- KUB CRM — Диагностика Проблемы Б: branch_id IS NULL
-- =============================================================================
-- Только READ-запросы. Запускать на проде до любых изменений.
-- Каждый блок — независимый SELECT; можно запускать по одному.
-- Порядок: сначала пользователи, потом сущности, потом списки.
-- =============================================================================


-- ---------------------------------------------------------------------------
-- §1. ОБЩАЯ КАРТИНА ПО ПОЛЬЗОВАТЕЛЯМ
-- ---------------------------------------------------------------------------

-- 1a. Итого пользователей и доля с NULL branch_id
SELECT
    COUNT(*)                                               AS total_users,
    COUNT(*) FILTER (WHERE branch_id IS NULL)              AS null_branch,
    COUNT(*) FILTER (WHERE branch_id IS NOT NULL)          AS has_branch,
    ROUND(
        COUNT(*) FILTER (WHERE branch_id IS NULL) * 100.0
        / NULLIF(COUNT(*), 0), 1
    )                                                      AS null_pct
FROM users;


-- 1b. Разбивка NULL branch_id по ролям
--     Критичность:
--       sales(10), visa(60), quality_control(30) — branch-scope; NULL → ErrForbidden,
--         весь pipeline невидим.
--       partner(70) — own-scope для лидов/клиентов, forbidden для сделок;
--         branch_id не используется в scope-проверке, но resolveUserBranch вернёт
--         ErrForbidden если он вдруг попадёт в ветку branch.
--       management(40) — scope=All; NULL не мешает видимости данных.
--       admin(50)      — scope=All; NULL не мешает видимости данных.
--       hr(80)         — scope=Forbidden для leads/deals; NULL некритичен.
--       legal(90)      — scope=Forbidden для leads; scope=All для clients; NULL некритичен.
SELECT
    u.role_id,
    r.name                                                        AS role_name,
    COUNT(*)                                                      AS total_users,
    COUNT(*) FILTER (WHERE u.branch_id IS NULL)                   AS null_branch,
    COUNT(*) FILTER (WHERE u.branch_id IS NOT NULL)               AS has_branch,
    ROUND(
        COUNT(*) FILTER (WHERE u.branch_id IS NULL) * 100.0
        / NULLIF(COUNT(*), 0), 1
    )                                                             AS null_pct,
    CASE
        WHEN u.role_id IN (10, 30, 60) THEN 'КРИТИЧНО (branch-scope)'
        WHEN u.role_id IN (70)         THEN 'НЕКРИТИЧНО (own-scope)'
        WHEN u.role_id IN (40, 50)     THEN 'НЕ ВЛИЯЕТ (scope=All)'
        WHEN u.role_id IN (80, 90)     THEN 'НИЗКИЙ (scope=Forbidden/All)'
        ELSE                                'НЕИЗВЕСТНАЯ РОЛЬ'
    END                                                           AS impact
FROM users u
LEFT JOIN roles r ON r.id = u.role_id
GROUP BY u.role_id, r.name
ORDER BY null_branch DESC;


-- ---------------------------------------------------------------------------
-- §2. NULL branch_id ПО СУЩНОСТЯМ (snapshot масштаба)
-- ---------------------------------------------------------------------------

SELECT 'users'            AS entity,
       COUNT(*)           AS total,
       COUNT(*) FILTER (WHERE branch_id IS NULL)  AS null_branch,
       ROUND(COUNT(*) FILTER (WHERE branch_id IS NULL) * 100.0 / NULLIF(COUNT(*),0), 1) AS null_pct
FROM users
UNION ALL
SELECT 'leads',           COUNT(*),
       COUNT(*) FILTER (WHERE branch_id IS NULL),
       ROUND(COUNT(*) FILTER (WHERE branch_id IS NULL) * 100.0 / NULLIF(COUNT(*),0), 1)
FROM leads
UNION ALL
SELECT 'deals',           COUNT(*),
       COUNT(*) FILTER (WHERE branch_id IS NULL),
       ROUND(COUNT(*) FILTER (WHERE branch_id IS NULL) * 100.0 / NULLIF(COUNT(*),0), 1)
FROM deals
UNION ALL
SELECT 'clients',         COUNT(*),
       COUNT(*) FILTER (WHERE branch_id IS NULL),
       ROUND(COUNT(*) FILTER (WHERE branch_id IS NULL) * 100.0 / NULLIF(COUNT(*),0), 1)
FROM clients
UNION ALL
SELECT 'tasks',           COUNT(*),
       COUNT(*) FILTER (WHERE branch_id IS NULL),
       ROUND(COUNT(*) FILTER (WHERE branch_id IS NULL) * 100.0 / NULLIF(COUNT(*),0), 1)
FROM tasks
UNION ALL
SELECT 'documents',       COUNT(*),
       COUNT(*) FILTER (WHERE branch_id IS NULL),
       ROUND(COUNT(*) FILTER (WHERE branch_id IS NULL) * 100.0 / NULLIF(COUNT(*),0), 1)
FROM documents
UNION ALL
SELECT 'chats',           COUNT(*),
       COUNT(*) FILTER (WHERE branch_id IS NULL),
       ROUND(COUNT(*) FILTER (WHERE branch_id IS NULL) * 100.0 / NULLIF(COUNT(*),0), 1)
FROM chats
UNION ALL
SELECT 'telephony_calls', COUNT(*),
       COUNT(*) FILTER (WHERE branch_id IS NULL),
       ROUND(COUNT(*) FILTER (WHERE branch_id IS NULL) * 100.0 / NULLIF(COUNT(*),0), 1)
FROM telephony_calls
ORDER BY null_branch DESC;


-- ---------------------------------------------------------------------------
-- §3. ВОССТАНОВИМОСТЬ: лиды с NULL branch_id
-- ---------------------------------------------------------------------------
-- Источник восстановления: owner → users.branch_id
-- Если owner сам без branch_id — запись неисправима автоматически.

SELECT
    COUNT(*)                                                             AS leads_null_branch,
    COUNT(*) FILTER (WHERE u.branch_id IS NOT NULL)                      AS recoverable_from_owner,
    COUNT(*) FILTER (WHERE u.branch_id IS NULL OR u.id IS NULL)          AS unrecoverable
FROM leads l
LEFT JOIN users u ON u.id = l.owner_id
WHERE l.branch_id IS NULL;

-- Сколько уникальных владельцев у восстановимых лидов
SELECT
    l.owner_id,
    u.email          AS owner_email,
    u.branch_id      AS owner_branch_id,
    COUNT(l.id)      AS recoverable_leads_count
FROM leads l
JOIN users u ON u.id = l.owner_id
WHERE l.branch_id IS NULL
  AND u.branch_id IS NOT NULL
GROUP BY l.owner_id, u.email, u.branch_id
ORDER BY recoverable_leads_count DESC;


-- ---------------------------------------------------------------------------
-- §4. ВОССТАНОВИМОСТЬ: сделки с NULL branch_id
-- ---------------------------------------------------------------------------
-- Сделки наследуют: lead.branch_id → owner.branch_id → NULL

SELECT
    COUNT(*)                                                                         AS deals_null_branch,
    COUNT(*) FILTER (WHERE COALESCE(l.branch_id, u.branch_id) IS NOT NULL)           AS recoverable,
    COUNT(*) FILTER (WHERE COALESCE(l.branch_id, u.branch_id) IS NULL)               AS unrecoverable
FROM deals d
LEFT JOIN leads  l ON l.id = d.lead_id
LEFT JOIN users  u ON u.id = d.owner_id
WHERE d.branch_id IS NULL;


-- ---------------------------------------------------------------------------
-- §5. ВОССТАНОВИМОСТЬ: клиенты с NULL branch_id
-- ---------------------------------------------------------------------------
-- Клиенты наследуют: owner → users.branch_id

SELECT
    COUNT(*)                                                             AS clients_null_branch,
    COUNT(*) FILTER (WHERE u.branch_id IS NOT NULL)                      AS recoverable_from_owner,
    COUNT(*) FILTER (WHERE u.branch_id IS NULL OR u.id IS NULL)          AS unrecoverable
FROM clients c
LEFT JOIN users u ON u.id = c.owner_id
WHERE c.branch_id IS NULL;


-- ---------------------------------------------------------------------------
-- §6. ВОССТАНОВИМОСТЬ: задачи с NULL branch_id
-- ---------------------------------------------------------------------------
-- Задачи наследуют: creator_id → users.branch_id

SELECT
    COUNT(*)                                                             AS tasks_null_branch,
    COUNT(*) FILTER (WHERE u.branch_id IS NOT NULL)                      AS recoverable_from_creator,
    COUNT(*) FILTER (WHERE u.branch_id IS NULL OR u.id IS NULL)          AS unrecoverable
FROM tasks t
LEFT JOIN users u ON u.id = t.creator_id
WHERE t.branch_id IS NULL;


-- ---------------------------------------------------------------------------
-- §7. ВОССТАНОВИМОСТЬ: документы с NULL branch_id
-- ---------------------------------------------------------------------------
-- Документы наследуют: deal.branch_id → NULL

SELECT
    COUNT(*)                                                             AS docs_null_branch,
    COUNT(*) FILTER (WHERE d.branch_id IS NOT NULL)                      AS recoverable_from_deal,
    COUNT(*) FILTER (WHERE d.branch_id IS NULL OR d.id IS NULL)          AS unrecoverable
FROM documents dc
LEFT JOIN deals d ON d.id = dc.deal_id
WHERE dc.branch_id IS NULL;


-- ---------------------------------------------------------------------------
-- §8. ДОСТУПНЫЕ ФИЛИАЛЫ (кандидат на дефолтный)
-- ---------------------------------------------------------------------------
-- Используйте id наименьшего активного филиала как DEFAULT_BRANCH_ID
-- для записей, которые нельзя восстановить автоматически.

SELECT
    id,
    code,
    name,
    is_active,
    created_at
FROM branches
ORDER BY id;


-- ---------------------------------------------------------------------------
-- §9. ПОИМЁННЫЙ СПИСОК ПОЛЬЗОВАТЕЛЕЙ С NULL branch_id
-- ---------------------------------------------------------------------------
-- Выводит всех пользователей без филиала для ручного разбора.
-- Особое внимание: role_id IN (10, 30, 60) — критичные роли.

SELECT
    u.id,
    u.email,
    u.role_id,
    r.name                                                   AS role_name,
    TRIM(CONCAT(u.last_name, ' ', u.first_name, ' ', u.middle_name)) AS full_name,
    u.is_active,
    u.is_verified,
    u.created_at,
    CASE
        WHEN u.role_id IN (10, 30, 60) THEN 'КРИТИЧНО — pipeline невидим'
        WHEN u.role_id IN (70)         THEN 'Некритично — own-scope'
        WHEN u.role_id IN (40, 50)     THEN 'Не влияет — scope=All'
        WHEN u.role_id IN (80, 90)     THEN 'Низкий приоритет'
        ELSE                                'Неизвестная роль'
    END                                                      AS impact_note
FROM users u
LEFT JOIN roles r ON r.id = u.role_id
WHERE u.branch_id IS NULL
ORDER BY
    CASE WHEN u.role_id IN (10, 30, 60) THEN 0
         WHEN u.role_id IN (70)         THEN 1
         WHEN u.role_id IN (40, 50)     THEN 2
         ELSE 3
    END,
    u.id;


-- ---------------------------------------------------------------------------
-- §10. КОНТРОЛЬ: пользователи с branch_id, которого нет в branches
-- ---------------------------------------------------------------------------
-- Рано выявить "мёртвые" FK (маловероятно, но быстро исключает).

SELECT u.id, u.email, u.branch_id
FROM users u
WHERE u.branch_id IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM branches b WHERE b.id = u.branch_id);


-- ---------------------------------------------------------------------------
-- §11. СТАТИСТИКА САМО-ЗАРЕГИСТРИРОВАННЫХ ПОЛЬЗОВАТЕЛЕЙ
-- ---------------------------------------------------------------------------
-- Register() (user_handler.go:869) всегда создаёт BranchID=nil, role=sales(10).
-- Эти записи — основной источник NULL branch_id для критичной роли.
-- Ориентировочно: пользователи role=10 без branch_id, в хронологии.

SELECT
    DATE_TRUNC('month', created_at)  AS month,
    COUNT(*)                         AS registered_sales_no_branch
FROM users
WHERE role_id = 10
  AND branch_id IS NULL
GROUP BY 1
ORDER BY 1;
