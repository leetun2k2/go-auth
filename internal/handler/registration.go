package handler

import (
	"github.com/leetun2k2/go-auth/internal/usecase"
)

type Handler struct {
	usecase *usecase.Usecase
}

func New(usecase *usecase.Usecase) *Handler {
	return &Handler{
		usecase: usecase,
	}
}
