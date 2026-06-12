-- Canonical Russian display names for all 8 active roles.
-- Idempotent: UPDATE WHERE code = '...' is safe to run multiple times.
UPDATE roles SET name = 'Менеджер по продажам (МОП)' WHERE code = 'sales';
UPDATE roles SET name = 'Отдел контроля качества'     WHERE code = 'quality_control';
UPDATE roles SET name = 'Руководство'                 WHERE code = 'management';
UPDATE roles SET name = 'Администратор'               WHERE code = 'admin';
UPDATE roles SET name = 'Визовый отдел'               WHERE code = 'visa';
UPDATE roles SET name = 'Менеджер по партнёрам'       WHERE code = 'partner';
UPDATE roles SET name = 'Отдел кадров'                WHERE code = 'hr';
UPDATE roles SET name = 'Юрист'                       WHERE code = 'legal';
