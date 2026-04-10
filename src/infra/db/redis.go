package db

import (
	"context"
	"fmt"
	"log"
	"lynx_agent_api/src/config"

	"github.com/redis/go-redis/v9"
)

var RedisClient *redis.Client

func InitRedis() error {

	config.LoadConfig()

	cfg := config.AppConfig

	RedisClient = redis.NewClient(&redis.Options{
		Addr:     cfg.Database.Redis.Addr,
		Username: cfg.Database.Redis.UserName,
		Password: cfg.Database.Redis.Password,
	})

	ctx := context.Background()

	if err := RedisClient.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("failed to connect to Redis: %w", err)
	}

	err := RedisClient.SetNX(ctx, "lxc:id", 99, 0).Err()
	if err != nil {
		return fmt.Errorf("failed to set initial LXC ID: %w", err)
	}

	log.Printf("✓ Redis connected successfully (addr: %s)\n", cfg.Database.Redis.Addr)
	return nil
}
