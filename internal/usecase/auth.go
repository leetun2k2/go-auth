package usecase

import (
	"context"

	"github.com/google/uuid"
	"github.com/leetun2k2/go-auth/internal/service"
)

var (
	ErrUnsupportedProvider    = service.ErrUnsupportedProvider
	ErrUnknownClient          = service.ErrUnknownClient
	ErrInvalidRedirectURI     = service.ErrInvalidRedirectURI
	ErrInvalidLoginRequest    = service.ErrInvalidLoginRequest
	ErrInvalidCallbackRequest = service.ErrInvalidCallbackRequest
	ErrInvalidClient          = service.ErrInvalidClient
	ErrInvalidExchangeRequest = service.ErrInvalidExchangeRequest
	ErrInvalidGrant           = service.ErrInvalidGrant
	ErrUnauthenticated        = service.ErrUnauthenticated
	ErrInvalidProfileRequest  = service.ErrInvalidProfileRequest
)

type HealthInput = service.HealthInput
type HealthOutput = service.HealthOutput
type ReadyInput = service.ReadyInput
type ReadyOutput = service.ReadyOutput
type LoginInput = service.LoginInput
type LoginOutput = service.LoginOutput
type CallbackInput = service.CallbackInput
type CallbackOutput = service.CallbackOutput
type ExchangeInput = service.ExchangeInput
type ExchangeOutput = service.ExchangeOutput
type LogoutInput = service.LogoutInput
type MeInput = service.MeInput
type MeOutput = service.MeOutput
type UpdateProfileInput = service.UpdateProfileInput
type UpdateProfileOutput = service.UpdateProfileOutput

func (u *Usecase) Health(ctx context.Context, input *HealthInput) (HealthOutput, error) {
	return u.service.Health(ctx, input)
}

func (u *Usecase) Ready(ctx context.Context, input *ReadyInput) (ReadyOutput, error) {
	return u.service.Ready(ctx, input)
}

func (u *Usecase) Login(ctx context.Context, input *LoginInput) (LoginOutput, error) {
	return u.service.Login(ctx, input)
}

func (u *Usecase) Callback(ctx context.Context, input *CallbackInput) (CallbackOutput, error) {
	return u.service.Callback(ctx, input)
}

func (u *Usecase) Exchange(ctx context.Context, input *ExchangeInput) (ExchangeOutput, error) {
	return u.service.Exchange(ctx, input)
}

func (u *Usecase) Me(ctx context.Context, input *MeInput) (MeOutput, error) {
	return u.service.Me(ctx, input)
}

func (u *Usecase) UpdateProfile(ctx context.Context, input *UpdateProfileInput) (UpdateProfileOutput, error) {
	return u.service.UpdateProfile(ctx, input)
}

func (u *Usecase) Logout(ctx context.Context, input *LogoutInput) error {
	return u.service.Logout(ctx, input)
}

func (u *Usecase) AuthenticateSessionUser(ctx context.Context, authHeader, clientID, clientSecret, provider string) (uuid.UUID, uuid.UUID, string, error) {
	return u.service.AuthenticateSessionUser(ctx, authHeader, clientID, clientSecret, provider)
}
