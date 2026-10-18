package handler

import (
	"context"

	"github.com/leetun2k2/go-auth/internal/usecase"
)

var (
	ErrUnsupportedProvider    = usecase.ErrUnsupportedProvider
	ErrUnknownClient          = usecase.ErrUnknownClient
	ErrInvalidRedirectURI     = usecase.ErrInvalidRedirectURI
	ErrInvalidLoginRequest    = usecase.ErrInvalidLoginRequest
	ErrInvalidCallbackRequest = usecase.ErrInvalidCallbackRequest
	ErrInvalidClient          = usecase.ErrInvalidClient
	ErrInvalidExchangeRequest = usecase.ErrInvalidExchangeRequest
	ErrInvalidGrant           = usecase.ErrInvalidGrant
	ErrUnauthenticated        = usecase.ErrUnauthenticated
	ErrInvalidProfileRequest  = usecase.ErrInvalidProfileRequest
)

type HealthInput = usecase.HealthInput
type HealthOutput = usecase.HealthOutput
type ReadyInput = usecase.ReadyInput
type ReadyOutput = usecase.ReadyOutput
type LoginInput = usecase.LoginInput
type LoginOutput = usecase.LoginOutput
type CallbackInput = usecase.CallbackInput
type CallbackOutput = usecase.CallbackOutput
type ExchangeInput = usecase.ExchangeInput
type ExchangeOutput = usecase.ExchangeOutput
type RefreshInput = usecase.RefreshInput
type RefreshOutput = usecase.RefreshOutput
type LogoutInput = usecase.LogoutInput
type LogoutAllInput = usecase.LogoutAllInput
type MeInput = usecase.MeInput
type MeOutput = usecase.MeOutput
type UpdateProfileInput = usecase.UpdateProfileInput
type UpdateProfileOutput = usecase.UpdateProfileOutput

func (h *Handler) Health(ctx context.Context, input *HealthInput) (HealthOutput, error) {
	return h.usecase.Health(ctx, input)
}

func (h *Handler) Ready(ctx context.Context, input *ReadyInput) (ReadyOutput, error) {
	return h.usecase.Ready(ctx, input)
}

func (h *Handler) Login(ctx context.Context, input *LoginInput) (LoginOutput, error) {
	return h.usecase.Login(ctx, input)
}

func (h *Handler) Callback(ctx context.Context, input *CallbackInput) (CallbackOutput, error) {
	return h.usecase.Callback(ctx, input)
}

func (h *Handler) Exchange(ctx context.Context, input *ExchangeInput) (ExchangeOutput, error) {
	return h.usecase.Exchange(ctx, input)
}

func (h *Handler) Refresh(ctx context.Context, input *RefreshInput) (RefreshOutput, error) {
	return h.usecase.Refresh(ctx, input)
}

func (h *Handler) Me(ctx context.Context, input *MeInput) (MeOutput, error) {
	return h.usecase.Me(ctx, input)
}

func (h *Handler) UpdateProfile(ctx context.Context, input *UpdateProfileInput) (UpdateProfileOutput, error) {
	return h.usecase.UpdateProfile(ctx, input)
}

func (h *Handler) Logout(ctx context.Context, input *LogoutInput) error {
	return h.usecase.Logout(ctx, input)
}

func (h *Handler) LogoutAll(ctx context.Context, input *LogoutAllInput) error {
	return h.usecase.LogoutAll(ctx, input)
}
