package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// UpdateProfileParams carries the requested changes for user-managed profile fields.
type UpdateProfileParams struct {
	DisplayName *string
	AvatarURL   *string
}

// UpdateProfile updates the mutable shared profile fields and marks updated
// fields as user-managed so subsequent OAuth logins do not overwrite them.
func (r *Repository) UpdateProfile(ctx context.Context, userID uuid.UUID, params UpdateProfileParams) (*User, error) {
	hasDisplayName := params.DisplayName != nil
	var newDisplayName string
	if hasDisplayName {
		newDisplayName = *params.DisplayName
	}

	hasAvatarURL := params.AvatarURL != nil
	var newAvatarURL string
	if hasAvatarURL {
		newAvatarURL = *params.AvatarURL
	}

	const query = `
WITH updated_profile AS (
    INSERT INTO profiles (user_id, display_name, display_name_managed, avatar_url, avatar_url_managed, updated_at)
    VALUES (
        $1,
        CASE WHEN $2::boolean THEN $3 ELSE '' END,
        $2::boolean,
        CASE WHEN $4::boolean THEN $5 ELSE '' END,
        $4::boolean,
        now()
    )
    ON CONFLICT (user_id) DO UPDATE
    SET display_name = CASE WHEN $2::boolean THEN EXCLUDED.display_name ELSE profiles.display_name END,
        display_name_managed = CASE WHEN $2::boolean THEN true ELSE profiles.display_name_managed END,
        avatar_url = CASE WHEN $4::boolean THEN EXCLUDED.avatar_url ELSE profiles.avatar_url END,
        avatar_url_managed = CASE WHEN $4::boolean THEN true ELSE profiles.avatar_url_managed END,
        updated_at = now()
    RETURNING user_id, display_name, avatar_url
)
SELECT u.id, COALESCE(u.canonical_email, ''), COALESCE(p.display_name, ''), COALESCE(p.avatar_url, '')
FROM users u
JOIN updated_profile p ON p.user_id = u.id
WHERE u.id = $1`

	var user User
	err := r.db.QueryRow(ctx, query, userID, hasDisplayName, newDisplayName, hasAvatarURL, newAvatarURL).
		Scan(&user.ID, &user.CanonicalEmail, &user.DisplayName, &user.AvatarURL)
	if err != nil {
		return nil, fmt.Errorf("update profile: %w", err)
	}

	return &user, nil
}
