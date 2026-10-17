package usecase

import (
	"context"

	"github.com/leetun2k2/go-auth/internal/service"
)

var ErrInvalidAdminRequest = service.ErrInvalidAdminRequest

type UpsertClientInput = service.UpsertClientInput
type UpsertClientProviderInput = service.UpsertClientProviderInput

func (u *Usecase) UpsertClient(ctx context.Context, input *UpsertClientInput) error {
	return u.service.UpsertClient(ctx, input)
}

func (u *Usecase) UpsertClientProvider(ctx context.Context, input *UpsertClientProviderInput) error {
	return u.service.UpsertClientProvider(ctx, input)
}
