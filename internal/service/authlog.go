package service

import (
	"context"

	"github.com/leetun2k2/go-auth/pkg/observability"
)

// logAuthEvent records a structured auth event for the given stage so
// operators can see client, provider, and outcome without any
// authentication material (spec 10, 11).
func (h *Service) logAuthEvent(ctx context.Context, clientID, provider, stage, status string) {
	observability.Log(ctx, h.logger, observability.Event{
		ClientID: clientID,
		Provider: provider,
		Stage:    stage,
		Status:   status,
	})
}
