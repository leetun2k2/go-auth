package usecase

import (
	"context"

	"github.com/leetun2k2/go-auth/internal/service"
)

var ErrInvalidAdminRequest = service.ErrInvalidAdminRequest

type UpsertClientInput = service.UpsertClientInput

func (u *Usecase) UpsertClient(ctx context.Context, input *UpsertClientInput) error {
	return u.service.UpsertClient(ctx, input)
}
