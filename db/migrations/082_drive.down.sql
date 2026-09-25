-- 082_drive.down.sql
-- ВНИМАНИЕ: удаляет только метаданные. Объекты с префиксом drive/ в S3
-- останутся — их нужно удалить отдельно, если хранилище выводится из работы.
DROP TABLE IF EXISTS drive_shares;
DROP TABLE IF EXISTS drive_nodes;
