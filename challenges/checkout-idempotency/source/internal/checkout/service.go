package checkout

import (
	"context"
	"strings"
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Checkout(ctx context.Context, userID, key string, request Request) (Order, error) {
	if strings.TrimSpace(userID) == "" || strings.TrimSpace(key) == "" {
		return Order{}, ErrInvalidRequest
	}
	if err := request.validate(); err != nil {
		return Order{}, err
	}
	total, err := totalCents(request.Items)
	if err != nil {
		return Order{}, err
	}
	saved, err := s.repo.Create(ctx, NewOrder{
		UserID:      userID,
		Key:         key,
		Fingerprint: fingerprint(request),
		Items:       request.Items,
		TotalCents:  total,
	})
	if err != nil {
		return Order{}, err
	}
	return saved.Order, nil
}
