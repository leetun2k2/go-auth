package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/leetun2k2/go-auth/internal/repository"
)

// ErrInvalidProfileRequest indicates that the profile update request payload
// is malformed, has all fields omitted, or has invalid field values.
var ErrInvalidProfileRequest = errors.New("invalid profile request")

// UpdateProfileInput carries the caller's authorization header and optional profile fields.
type UpdateProfileInput struct {
	AuthorizationHeader string
	DisplayName         *string
	AvatarURL           *string
}

// UpdateProfileOutput returns the persisted shared profile for the updated user.
type UpdateProfileOutput struct {
	UserID      string
	Email       string
	DisplayName string
	AvatarURL   string
}

// UpdateProfile validates the session and mutable profile fields, persists the changes,
// marks each updated field as user-managed, and returns the updated profile.
func (h *Service) UpdateProfile(ctx context.Context, input *UpdateProfileInput) (UpdateProfileOutput, error) {
	session, err := h.authenticateSession(ctx, input.AuthorizationHeader)
	if err != nil {
		return UpdateProfileOutput{}, err
	}

	if input.DisplayName == nil && input.AvatarURL == nil {
		return UpdateProfileOutput{}, ErrInvalidProfileRequest
	}

	var params repository.UpdateProfileParams

	if input.DisplayName != nil {
		trimmedName := strings.TrimSpace(*input.DisplayName)
		if utf8.RuneCountInString(trimmedName) > 255 {
			return UpdateProfileOutput{}, ErrInvalidProfileRequest
		}
		params.DisplayName = &trimmedName
	}

	if input.AvatarURL != nil {
		trimmedURL := strings.TrimSpace(*input.AvatarURL)
		if trimmedURL != "" && !isValidHTTPSURL(trimmedURL) {
			return UpdateProfileOutput{}, ErrInvalidProfileRequest
		}
		params.AvatarURL = &trimmedURL
	}

	user, err := h.repo.UpdateProfile(ctx, session.User.ID, params)
	if err != nil {
		return UpdateProfileOutput{}, fmt.Errorf("update profile: %w", err)
	}

	return UpdateProfileOutput{
		UserID:      user.ID.String(),
		Email:       user.CanonicalEmail,
		DisplayName: user.DisplayName,
		AvatarURL:   user.AvatarURL,
	}, nil
}

func isValidHTTPSURL(rawURL string) bool {
	u, err := url.ParseRequestURI(rawURL)
	if err != nil {
		return false
	}
	if strings.ToLower(u.Scheme) != "https" {
		return false
	}
	if u.Host == "" {
		return false
	}
	return true
}
