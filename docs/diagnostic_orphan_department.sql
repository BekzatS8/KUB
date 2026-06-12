-- =============================================================================
-- Диагностический SQL для оценки масштаба осиротевших записей
-- (department_id IS NULL после backfill).
--
-- ВАЖНО: Запускать ТОЛЬКО на проде ПЕРЕД деплоем миграции 039.
-- Читает данные, ничего не меняет.
-- =============================================================================

-- 1. Лиды: сколько записей не имеют funnel_id (не смогут получить dept из воронки)
SELECT
    COUNT(*)                                     AS total_leads,
    COUNT(*) FILTER (WHERE funnel_id IS NULL)    AS leads_no_funnel,
    COUNT(*) FILTER (WHERE funnel_id IS NOT NULL) AS leads_with_funnel,
    ROUND(
        100.0 * COUNT(*) FILTER (WHERE funnel_id IS NULL) / NULLIF(COUNT(*), 0), 1
    )                                             AS pct_no_funnel
FROM leads
WHERE is_archived = FALSE;

-- 2. Сделки: аналогично
SELECT
    COUNT(*)                                     AS total_deals,
    COUNT(*) FILTER (WHERE funnel_id IS NULL)    AS deals_no_funnel,
    COUNT(*) FILTER (WHERE funnel_id IS NOT NULL) AS deals_with_funnel,
    ROUND(
        100.0 * COUNT(*) FILTER (WHERE funnel_id IS NULL) / NULLIF(COUNT(*), 0), 1
    )                                             AS pct_no_funnel
FROM deals
WHERE is_archived = FALSE;

-- 3. Пользователи: сколько не имеют department_id (потребуют backfill из роли)
SELECT
    COUNT(*)                                              AS total_users,
    COUNT(*) FILTER (WHERE department_id IS NULL)        AS users_no_dept,
    COUNT(*) FILTER (WHERE department_id IS NOT NULL)    AS users_with_dept
FROM users
WHERE COALESCE(is_active, TRUE) = TRUE;

-- 4. Лиды: оценка результата backfill (сколько получат dept из воронки, сколько из юзера, сколько осиротеют)
SELECT
    COUNT(*) FILTER (WHERE f.department_id IS NOT NULL)                              AS will_get_dept_from_funnel,
    COUNT(*) FILTER (WHERE f.department_id IS NULL AND u.department_id IS NOT NULL) AS will_get_dept_from_owner,
    COUNT(*) FILTER (WHERE f.department_id IS NULL AND u.department_id IS NULL)     AS will_remain_null
FROM leads l
LEFT JOIN funnels f ON f.id = l.funnel_id
LEFT JOIN users u ON u.id = l.owner_id
WHERE l.is_archived = FALSE;

-- 5. Сделки: аналогично
SELECT
    COUNT(*) FILTER (WHERE f.department_id IS NOT NULL)                              AS will_get_dept_from_funnel,
    COUNT(*) FILTER (WHERE f.department_id IS NULL AND u.department_id IS NOT NULL) AS will_get_dept_from_owner,
    COUNT(*) FILTER (WHERE f.department_id IS NULL AND u.department_id IS NULL)     AS will_remain_null
FROM deals d
LEFT JOIN funnels f ON f.id = d.funnel_id
LEFT JOIN users u ON u.id = d.owner_id
WHERE d.is_archived = FALSE;

-- 6. Breakdown по отделам (для лидов — после гипотетического backfill)
SELECT
    COALESCE(d.code, 'NULL (orphaned)') AS department_code,
    COUNT(*)                            AS lead_count
FROM leads l
LEFT JOIN funnels f ON f.id = l.funnel_id
LEFT JOIN users u ON u.id = l.owner_id
LEFT JOIN departments d ON d.id = COALESCE(f.department_id, u.department_id)
WHERE l.is_archived = FALSE
GROUP BY d.code
ORDER BY lead_count DESC;
