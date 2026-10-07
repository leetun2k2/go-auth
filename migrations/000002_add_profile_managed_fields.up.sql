ALTER TABLE profiles
    ADD COLUMN display_name_managed BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN avatar_url_managed BOOLEAN NOT NULL DEFAULT false;
