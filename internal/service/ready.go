package service

import "context"

type ReadyInput struct{}

// ReadyOutput mirrors HealthOutput's shape: StatusCode drives the HTTP
// status at the transport layer, Status and Error are the JSON body.
type ReadyOutput struct {
	StatusCode int    `json:"-"`
	Status     string `json:"status"`
	Error      string `json:"error,omitempty"`
}

// Ready reports whether the service can safely serve auth traffic.
// It checks PostgreSQL connectivity only: a provider outage does not
// affect readiness because existing session validation keeps working
// (spec 11).
func (h *Service) Ready(c context.Context, input *ReadyInput) (ReadyOutput, error) {
	if err := h.repo.Ping(c); err != nil {
		return ReadyOutput{StatusCode: 503, Status: "not ready", Error: "database unavailable"}, nil
	}
	return ReadyOutput{StatusCode: 200, Status: "ready"}, nil
}
