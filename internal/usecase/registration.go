package usecase

import (
	"github.com/leetun2k2/go-auth/internal/service"
)

type Usecase struct {
	service *service.Service
}

func New(service *service.Service) *Usecase {
	return &Usecase{
		service: service,
	}
}
