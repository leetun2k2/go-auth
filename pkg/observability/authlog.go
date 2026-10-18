// Package observability provides structured auth event logging (spec 11).
package observability

import (
	"context"

	"github.com/leetun2k2/go-bedrock/contextx"
	"github.com/leetun2k2/go-bedrock/logx"
)

// Auth event stages.
const (
	StageOAuthStart      = "oauth_start"
	StageCallback        = "callback"
	StageExchange        = "exchange"
	StageRefresh         = "refresh"
	StageSessionValidate = "session_validate"
	StageLogout          = "logout"
)

// Auth event outcomes.
const (
	StatusSuccess = "success"
	StatusDenied  = "denied"
	StatusFailure = "failure"
)

// none fills client_id and provider when a stage has no value yet, so
// every event still carries the required field.
const none = "none"

// Event is a structured auth operation event. Only non-secret identifiers
// belong on it: never attach tokens, authorization codes, JWTs, emails,
// cookies, or raw provider payloads (spec 10, 11).
type Event struct {
	ClientID string
	Provider string
	Stage    string
	Status   string
}

// Log emits a structured auth event carrying correlation_id, client_id,
// provider, stage, and status. The correlation ID is read from the request
// context populated by the transport layer's correlation middleware.
func Log(ctx context.Context, logger *logx.Logger, event Event) {
	logger.Info(ctx, "auth_event",
		"correlation_id", orNone(contextx.GetCorrelationID(ctx)),
		"client_id", orNone(event.ClientID),
		"provider", orNone(event.Provider),
		"stage", event.Stage,
		"status", event.Status,
	)
}

func orNone(v string) string {
	if v == "" {
		return none
	}
	return v
}
