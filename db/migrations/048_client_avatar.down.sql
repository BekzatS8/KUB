-- 048 down: Remove avatar fields from clients
ALTER TABLE clients DROP COLUMN IF EXISTS avatar_url;
ALTER TABLE clients DROP COLUMN IF EXISTS avatar_path;
ALTER TABLE clients DROP COLUMN IF EXISTS avatar_crop_x;
ALTER TABLE clients DROP COLUMN IF EXISTS avatar_crop_y;
ALTER TABLE clients DROP COLUMN IF EXISTS avatar_crop_scale;
ALTER TABLE clients DROP COLUMN IF EXISTS avatar_crop_size;
