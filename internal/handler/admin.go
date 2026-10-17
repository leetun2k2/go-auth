package handler

import (
	"context"

	"github.com/leetun2k2/go-auth/internal/usecase"
)

var ErrInvalidAdminRequest = usecase.ErrInvalidAdminRequest

type UpsertClientInput = usecase.UpsertClientInput
type UpsertClientProviderInput = usecase.UpsertClientProviderInput

func (h *Handler) UpsertClient(ctx context.Context, input *UpsertClientInput) error {
	return h.usecase.UpsertClient(ctx, input)
}

func (h *Handler) UpsertClientProvider(ctx context.Context, input *UpsertClientProviderInput) error {
	return h.usecase.UpsertClientProvider(ctx, input)
}
