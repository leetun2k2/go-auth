ALTER TABLE profiles
    DROP COLUMN IF EXISTS avatar_url_managed,
    DROP COLUMN IF EXISTS display_name_managed;
