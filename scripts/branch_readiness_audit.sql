-- Branch readiness audit for production release.
-- Safe to run repeatedly. It does not mutate data.
--
-- Output columns:
-- issue_key | severity | issue_count | action
--
-- Release rule:
-- - Any CRITICAL row with issue_count > 0 must stop the release until fixed.
-- - WARN rows should be reviewed, but they do not automatically block deploy.

WITH checks AS (
    SELECT
        'scoped_active_users_without_branch' AS issue_key,
        'CRITICAL' AS severity,
        COUNT(*)::bigint AS issue_count,
        'Assign branch_id to every active sales/operations/control user before deploy.' AS action
    FROM users
    WHERE role_id IN (10, 20, 30)
      AND COALESCE(is_active, TRUE) = TRUE
      AND branch_id IS NULL

    UNION ALL
    SELECT
        'clients_without_branch',
        'CRITICAL',
        COUNT(*)::bigint,
        'Backfill from owner branch where safe, otherwise assign branch manually.'
    FROM clients
    WHERE branch_id IS NULL

    UNION ALL
    SELECT
        'leads_without_branch',
        'CRITICAL',
        COUNT(*)::bigint,
        'Backfill from owner branch where safe, otherwise assign branch manually.'
    FROM leads
    WHERE branch_id IS NULL

    UNION ALL
    SELECT
        'deals_without_branch',
        'CRITICAL',
        COUNT(*)::bigint,
        'Backfill from linked lead or owner branch where safe, otherwise assign branch manually.'
    FROM deals
    WHERE branch_id IS NULL

    UNION ALL
    SELECT
        'documents_without_branch',
        'CRITICAL',
        COUNT(*)::bigint,
        'Backfill from linked deal branch where safe, otherwise assign branch manually.'
    FROM documents
    WHERE branch_id IS NULL

    UNION ALL
    SELECT
        'tasks_without_branch',
        'CRITICAL',
        COUNT(*)::bigint,
        'Backfill from creator branch where safe, otherwise assign branch manually.'
    FROM tasks
    WHERE branch_id IS NULL

    UNION ALL
    SELECT
        'chats_without_branch',
        'CRITICAL',
        COUNT(*)::bigint,
        'Backfill from creator branch where safe, otherwise assign branch manually.'
    FROM chats
    WHERE branch_id IS NULL

    UNION ALL
    SELECT
        'lead_owner_branch_mismatch',
        'CRITICAL',
        COUNT(*)::bigint,
        'Review leads whose scoped owner branch differs from lead.branch_id.'
    FROM leads l
    JOIN users u ON u.id = l.owner_id
    WHERE u.role_id IN (10, 20, 30)
      AND u.branch_id IS NOT NULL
      AND l.branch_id IS NOT NULL
      AND l.branch_id <> u.branch_id

    UNION ALL
    SELECT
        'deal_lead_branch_mismatch',
        'CRITICAL',
        COUNT(*)::bigint,
        'Review deals whose linked lead branch differs from deal.branch_id.'
    FROM deals d
    JOIN leads l ON l.id = d.lead_id
    WHERE d.branch_id IS NOT NULL
      AND l.branch_id IS NOT NULL
      AND d.branch_id <> l.branch_id

    UNION ALL
    SELECT
        'document_deal_branch_mismatch',
        'CRITICAL',
        COUNT(*)::bigint,
        'Review documents whose linked deal branch differs from document.branch_id.'
    FROM documents doc
    JOIN deals d ON d.id = doc.deal_id
    WHERE doc.branch_id IS NOT NULL
      AND d.branch_id IS NOT NULL
      AND doc.branch_id <> d.branch_id

    UNION ALL
    SELECT
        'task_assignee_branch_mismatch',
        'CRITICAL',
        COUNT(*)::bigint,
        'Review tasks assigned to scoped users from a different branch.'
    FROM tasks t
    JOIN users u ON u.id = t.assignee_id
    WHERE u.role_id IN (10, 20, 30)
      AND u.branch_id IS NOT NULL
      AND t.branch_id IS NOT NULL
      AND t.branch_id <> u.branch_id

    UNION ALL
    SELECT
        'chat_member_cross_branch',
        'WARN',
        COUNT(*)::bigint,
        'Review chats with scoped members from a branch different from chat.branch_id.'
    FROM chats c
    JOIN chat_members cm ON cm.chat_id = c.id
    JOIN users u ON u.id = cm.user_id
    WHERE u.role_id IN (10, 20, 30)
      AND u.branch_id IS NOT NULL
      AND c.branch_id IS NOT NULL
      AND c.branch_id <> u.branch_id
)
SELECT issue_key, severity, issue_count, action
FROM checks
ORDER BY
    CASE severity WHEN 'CRITICAL' THEN 1 ELSE 2 END,
    issue_key;
