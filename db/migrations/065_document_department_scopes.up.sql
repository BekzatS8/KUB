-- 065_document_department_scopes.up.sql
-- Изоляция документов по отделам (ТЗ 04.07.2026, п.2.1/2.2).
--
-- Раньше scope знал только 'deal' / 'hr' / 'legal'. Теперь каждый отдел хранит
-- свои документы (отчёты, шаблоны) в своём scope: продажи, визовый,
-- партнёрский, контроль качества, руководство. Документы по сделкам остаются
-- в scope 'deal'.
-- This migration is re-applied on every deploy, so every statement is idempotent.

ALTER TABLE documents DROP CONSTRAINT IF EXISTS documents_scope_chk;
ALTER TABLE documents ADD CONSTRAINT documents_scope_chk CHECK (
    scope IN ('deal', 'hr', 'legal', 'sales', 'visa', 'partner', 'quality_control', 'management')
);
