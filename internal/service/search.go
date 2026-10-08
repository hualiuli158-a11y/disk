package service

import (
	"context"
	"errors"
	"strings"

	"disk/internal/model"
)

var ErrInvalidSearchQuery = errors.New("搜索参数不合法")

type SearchRepository interface {
	Search(
		ctx context.Context,
		userID uint64,
		query string,
		kind string,
		offset int,
		limit int,
		sortField string,
		sortOrder string,
	) ([]model.ContentItem, int64, error)
}

type SearchService struct {
	search SearchRepository
}

func NewSearchService(search SearchRepository) *SearchService {
	return &SearchService{search: search}
}

type SearchInput struct {
	UserID    uint64
	Query     string
	Kind      string
	Page      int
	PageSize  int
	SortField string
	SortOrder string
}

type SearchOutput struct {
	Items      []ContentItem
	Page       int
	PageSize   int
	Total      int64
	TotalPages int64
	Query      string
	Kind       string
	SortField  string
	SortOrder  string
}

func (s *SearchService) Search(ctx context.Context, input SearchInput) (*SearchOutput, error) {
	if input.UserID == 0 {
		return nil, ErrFolderUserNotFound
	}
	input.Query = strings.TrimSpace(input.Query)
	if input.Query == "" || len([]rune(input.Query)) > 255 {
		return nil, ErrInvalidSearchQuery
	}
	if input.Kind == "" {
		input.Kind = "all"
	}
	if input.Page == 0 {
		input.Page = 1
	}
	if input.PageSize == 0 {
		input.PageSize = DefaultContentPageSize
	}
	if input.SortField == "" {
		input.SortField = "name"
	}
	if input.SortOrder == "" {
		input.SortOrder = "asc"
	}
	validKind := input.Kind == "all" || input.Kind == "folder" || input.Kind == "file"
	validSort := input.SortField == "name" || input.SortField == "size" || input.SortField == "created_at" || input.SortField == "updated_at"
	validOrder := input.SortOrder == "asc" || input.SortOrder == "desc"
	maxInt := int(^uint(0) >> 1)
	if !validKind || !validSort || !validOrder || input.Page < 1 || input.PageSize < 1 || input.PageSize > MaxContentPageSize || input.Page > maxInt/input.PageSize {
		return nil, ErrInvalidSearchQuery
	}

	offset := (input.Page - 1) * input.PageSize
	results, total, err := s.search.Search(
		ctx, input.UserID, input.Query, input.Kind, offset, input.PageSize, input.SortField, input.SortOrder,
	)
	if err != nil {
		return nil, err
	}

	items := make([]ContentItem, 0, len(results))
	for _, item := range results {
		items = append(items, ContentItem{
			ID: item.ID, Kind: item.Kind, ParentID: item.ParentID, Name: item.Name,
			Size: item.Size, SHA256: item.SHA256, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt,
		})
	}
	totalPages := int64(0)
	if total > 0 {
		totalPages = (total + int64(input.PageSize) - 1) / int64(input.PageSize)
	}
	return &SearchOutput{
		Items: items, Page: input.Page, PageSize: input.PageSize, Total: total, TotalPages: totalPages,
		Query: input.Query, Kind: input.Kind, SortField: input.SortField, SortOrder: input.SortOrder,
	}, nil
}
