BEGIN;

ALTER TABLE users
    DROP COLUMN IF EXISTS updated_at,
    DROP COLUMN IF EXISTS avatar_crop_size,
    DROP COLUMN IF EXISTS avatar_crop_scale,
    DROP COLUMN IF EXISTS avatar_crop_y,
    DROP COLUMN IF EXISTS avatar_crop_x,
    DROP COLUMN IF EXISTS avatar_original_path,
    DROP COLUMN IF EXISTS avatar_path,
    DROP COLUMN IF EXISTS avatar_url,
    DROP COLUMN IF EXISTS extra_info,
    DROP COLUMN IF EXISTS address;

COMMIT;
