package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"uptime-lite/internal/models"
)

const statusTTL = 5 * time.Minute

// Client wraps a Redis connection with domain-specific helpers.
type Client struct {
	rdb *redis.Client
}

// New connects to Redis and returns a ready Client.
func New(redisURL string) (*Client, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("redis.ParseURL: %w", err)
	}
	rdb := redis.NewClient(opts)
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		return nil, fmt.Errorf("redis ping: %w", err)
	}
	return &Client{rdb: rdb}, nil
}

// SetStatus persists the latest probe result for a target.
func (c *Client) SetStatus(ctx context.Context, s models.TargetStatus) error {
	data, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("marshal status: %w", err)
	}
	key := statusKey(s.TargetID)
	return c.rdb.Set(ctx, key, data, statusTTL).Err()
}

// GetStatus retrieves the cached status for a target.
// Returns (nil, nil) when no entry exists.
func (c *Client) GetStatus(ctx context.Context, targetID int64) (*models.TargetStatus, error) {
	data, err := c.rdb.Get(ctx, statusKey(targetID)).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("redis get: %w", err)
	}
	var s models.TargetStatus
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("unmarshal status: %w", err)
	}
	return &s, nil
}

// Close shuts down the underlying Redis connection.
func (c *Client) Close() error {
	return c.rdb.Close()
}

func statusKey(targetID int64) string {
	return fmt.Sprintf("status:%d", targetID)
}
