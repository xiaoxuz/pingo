package cache

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisClient struct {
	client *redis.Client
}

func NewRedisClient(addr string, password string, db int) (*RedisClient, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis ping failed: %w", err)
	}

	return &RedisClient{client: rdb}, nil
}

func (r *RedisClient) Close() error {
	return r.client.Close()
}

func (r *RedisClient) Raw() *redis.Client {
	return r.client
}

// ============================================================
// 在线状态
// ============================================================

const onlineKeyPrefix = "pingo:online:"
const onlineTTL = 120 * time.Second

func (r *RedisClient) SetOnline(ctx context.Context, agentID string, online bool) error {
	key := onlineKeyPrefix + agentID
	if online {
		return r.client.Set(ctx, key, "1", onlineTTL).Err()
	}
	return r.client.Del(ctx, key).Err()
}

func (r *RedisClient) IsOnline(ctx context.Context, agentID string) (bool, error) {
	key := onlineKeyPrefix + agentID
	n, err := r.client.Exists(ctx, key).Result()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (r *RedisClient) BatchIsOnline(ctx context.Context, agentIDs []string) (map[string]bool, error) {
	result := make(map[string]bool, len(agentIDs))
	if len(agentIDs) == 0 {
		return result, nil
	}

	keys := make([]string, len(agentIDs))
	for i, id := range agentIDs {
		keys[i] = onlineKeyPrefix + id
	}

	vals, err := r.client.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, err
	}

	for i, v := range vals {
		if v != nil {
			result[agentIDs[i]] = true
		}
	}
	return result, nil
}

func (r *RedisClient) RefreshOnline(ctx context.Context, agentID string) error {
	key := onlineKeyPrefix + agentID
	return r.client.Expire(ctx, key, onlineTTL).Err()
}

// ============================================================
// 未读计数缓存
// ============================================================

const unreadKeyPrefix = "pingo:unread:"
const unreadTTL = 30 * time.Minute

func unreadKey(agentID string, conversationID string) string {
	return fmt.Sprintf("%s%s:%s", unreadKeyPrefix, agentID, conversationID)
}

func (r *RedisClient) GetUnreadCount(ctx context.Context, agentID string, conversationID string) (int, bool, error) {
	key := unreadKey(agentID, conversationID)
	val, err := r.client.Get(ctx, key).Int()
	if err == redis.Nil {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return val, true, nil
}

func (r *RedisClient) SetUnreadCount(ctx context.Context, agentID string, conversationID string, count int) error {
	key := unreadKey(agentID, conversationID)
	return r.client.Set(ctx, key, count, unreadTTL).Err()
}

func (r *RedisClient) IncrementUnread(ctx context.Context, agentID string, conversationID string) (int, error) {
	key := unreadKey(agentID, conversationID)
	v, err := r.client.Incr(ctx, key).Result()
	return int(v), err
}

func (r *RedisClient) ClearUnread(ctx context.Context, agentID string, conversationID string) error {
	key := unreadKey(agentID, conversationID)
	return r.client.Set(ctx, key, 0, unreadTTL).Err()
}

// ============================================================
// 发现缓存
// ============================================================

const discoverKeyPrefix = "pingo:discover:"
const discoverTTL = 5 * time.Minute

func discoverCacheKey(keyword string, tags string) string {
	return fmt.Sprintf("%s%s:%s", discoverKeyPrefix, keyword, tags)
}

func (r *RedisClient) GetDiscoverCache(ctx context.Context, keyword string, tags string) (string, bool, error) {
	key := discoverCacheKey(keyword, tags)
	val, err := r.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return val, true, nil
}

func (r *RedisClient) SetDiscoverCache(ctx context.Context, keyword string, tags string, data string) error {
	key := discoverCacheKey(keyword, tags)
	return r.client.Set(ctx, key, data, discoverTTL).Err()
}

func (r *RedisClient) InvalidateDiscoverCache(ctx context.Context) error {
	iter := r.client.Scan(ctx, 0, discoverKeyPrefix+"*", 100).Iterator()
	for iter.Next(ctx) {
		r.client.Del(ctx, iter.Val())
	}
	return iter.Err()
}

// ============================================================
// 限流
// ============================================================

const rateLimitPrefix = "pingo:rate:"

func (r *RedisClient) RateLimit(ctx context.Context, key string, limit int, window time.Duration) (bool, int, error) {
	rlKey := rateLimitPrefix + key
	now := time.Now().UnixNano()
	windowStart := now - window.Nanoseconds()

	pipe := r.client.Pipeline()

	pipe.ZRemRangeByScore(ctx, rlKey, "0", fmt.Sprintf("%d", windowStart))
	pipe.ZAdd(ctx, rlKey, redis.Z{Score: float64(now), Member: fmt.Sprintf("%d", now)})
	pipe.ZCard(ctx, rlKey)
	pipe.Expire(ctx, rlKey, window*2)

	results, err := pipe.Exec(ctx)
	if err != nil {
		return false, 0, err
	}

	count := results[2].(*redis.IntCmd).Val()
	allowed := count <= int64(limit)
	return allowed, int(count), nil
}
