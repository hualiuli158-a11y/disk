package service

import (
	"context"
	"errors"

	"disk/internal/model"
	"disk/internal/repository"
)

var ErrQuotaUserNotFound = errors.New("用户不存在")

type QuotaUserFinder interface {
	FindByID(ctx context.Context, userID uint64) (*model.User, error)
}

type QuotaService struct {
	users QuotaUserFinder
}

func NewQuotaService(users QuotaUserFinder) *QuotaService {
	return &QuotaService{users: users}
}

type QuotaOutput struct {
	Used      int64
	Limit     int64
	Remaining int64
}

func (s *QuotaService) GetQuota(ctx context.Context, userID uint64) (*QuotaOutput, error) {
	if userID == 0 {
		return nil, ErrQuotaUserNotFound
	}
	user, err := s.users.FindByID(ctx, userID)
	switch {
	case errors.Is(err, repository.ErrUserNotFound):
		return nil, ErrQuotaUserNotFound
	case err != nil:
		return nil, err
	}

	remaining := user.QuotaLimit - user.UsedQuota
	if remaining < 0 {
		remaining = 0
	}
	return &QuotaOutput{Used: user.UsedQuota, Limit: user.QuotaLimit, Remaining: remaining}, nil
}
