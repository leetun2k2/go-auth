package handler

import "context"

type HealthInput struct{}
type HealthOutput struct {
	StatusCode int    `json:"status_code"`
	Message    string `json:"message,omitempty"`
}

func (h *Handler) HealthHandler(c context.Context, input *HealthInput) (HealthOutput, error) {
	return HealthOutput{StatusCode: 200, Message: "ok"}, nil
}
