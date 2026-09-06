package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Aneeshie/shared-in-memory-index/internal/cache"
	"github.com/Aneeshie/shared-in-memory-index/internal/domain"
	"github.com/Aneeshie/shared-in-memory-index/internal/dto"
	"github.com/Aneeshie/shared-in-memory-index/internal/repository"
)

type CacheService struct {
	repo  *repository.Repository
	cache *cache.Cache
}

func NewCacheService(repo *repository.Repository, cache *cache.Cache) *CacheService {
	return &CacheService{
		repo:  repo,
		cache: cache,
	}
}

func (s *CacheService) GetData(ctx context.Context, key string) (*dto.GetDataResponse, error) {
	entry, err := s.cache.Get(key)

	if err == nil {
		return &dto.GetDataResponse{
			Data: entry.Data,
		}, nil
	}

	cachedEntry, err := s.repo.GetCachedEntry(ctx, key)
	if err != nil {
		if errors.Is(err, repository.ErrNoRowRound) {
			return nil, fmt.Errorf("data not found")
		}

		return nil, fmt.Errorf("get data from database: %w", err)
	}

	s.cache.Put(key, cachedEntry.Data)

	return &dto.GetDataResponse{
		Data: cachedEntry.Data,
	}, nil
}

func (s *CacheService) PutData(ctx context.Context, key string, entry *dto.PutDataRequest) error {
	e := domain.CachedEntry{
		Key:       key,
		Data:      entry.Data,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := s.repo.CreateCachedEntry(ctx, &e); err != nil {
		return fmt.Errorf("failed to store cache entry: %w", err)
	}

	s.cache.Put(key, entry.Data)

	return nil
}
