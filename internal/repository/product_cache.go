package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/Nuriklan/digital-commerce/internal/domain"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

type CachedProductRepository struct {
	repo ProductRepository
	rdb  *redis.Client
	ttl  time.Duration
}

func NewCachedProductRepository(repo ProductRepository, rdb *redis.Client, ttl time.Duration) *CachedProductRepository {
	return &CachedProductRepository{
		repo: repo,
		rdb:  rdb,
		ttl:  ttl,
	}
}

func (c *CachedProductRepository) cacheKey(id uuid.UUID) string {
	return fmt.Sprintf("product:%s", id.String())
}

func (c *CachedProductRepository) Save(ctx context.Context, p domain.Product) error {
	if err := c.repo.Save(ctx, p); err != nil {
		return err
	}

	// Invalidate cache: remove keys
	_ = c.rdb.Del(ctx, c.cacheKey(p.ID)).Err()
	return nil
}

func (c *CachedProductRepository) GetByID(ctx context.Context, id uuid.UUID) (domain.Product, error) {
	key := c.cacheKey(id)

	// Check cache
	val, err := c.rdb.Get(ctx, key).Result()
	if err == nil {
		var product domain.Product
		if err := json.Unmarshal([]byte(val), &product); err == nil {
			return product, nil
		}
	} else if !errors.Is(err, redis.Nil) {
		log.Printf("redis get error: %v", err)
	}

	// Cache Miss: reading from database
	product, err := c.repo.GetByID(ctx, id)
	if err != nil {
		return domain.Product{}, err
	}

	// Save in cache with TTL
	data, err := json.Marshal(product)
	if err == nil {
		if err := c.rdb.Set(ctx, key, data, c.ttl).Err(); err != nil {
			log.Printf("redis set error: %v", err)
		}
	}

	return product, nil
}

func (c *CachedProductRepository) List(ctx context.Context, limit, offset int) ([]domain.Product, error) {
	return c.repo.List(ctx, limit, offset)
}
