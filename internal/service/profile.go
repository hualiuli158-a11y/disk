package service

import (
	"context"
	"errors"
	"time"

	"disk/internal/model"
	"disk/internal/repository"
)

var ErrProfileNotFound = errors.New("用户不存在")

type ProfileUserFinder interface {
	FindByID(context.Context, uint64) (*model.User, error)
}
type ProfileService struct {
	users ProfileUserFinder
}

type ProfileOutput struct {
	ID         uint64
	Username   string
	CreateTime time.Time
	UsedQuota  int64
}

func NewProfileService(users ProfileUserFinder) *ProfileService {
	return &ProfileService{users: users}
}

func (s *ProfileService) Profile(ctx context.Context, userID uint64) (*ProfileOutput, error) {
	user, err := s.users.FindByID(ctx, userID)
	if errors.Is(err, repository.ErrUserNotFound) {
		return nil, ErrProfileNotFound
	}
	if err != nil {
		return nil, err
	}
	return &ProfileOutput{
		ID:         user.ID,
		Username:   user.Username,
		CreateTime: user.CreateTime,
		UsedQuota:  user.UsedQuota,
	}, nil
}
