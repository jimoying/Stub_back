package redisclient

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

// Open 创建 Redis 客户端并验证服务可用。
func Open(ctx context.Context, rawURL string) (*redis.Client, error) {
	// 1. 解析标准 redis:// URL，避免在业务代码中拼接连接参数。
	options, err := redis.ParseURL(rawURL)
	if err != nil {
		return nil, fmt.Errorf("解析 Redis URL 失败: %w", err)
	}
	client := redis.NewClient(options)

	// 2. 启动时检查 Redis；Redis 是部署必需依赖，但不作为核心数据权威源。
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("连接 Redis 失败: %w", err)
	}
	return client, nil
}
