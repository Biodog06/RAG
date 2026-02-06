package service

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"pai-smart-go/internal/model"
	"pai-smart-go/pkg/database"
	"pai-smart-go/pkg/log"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
)

// ContentCacheService 定义了缓存服务的接口
type ContentCacheService interface {
	Get(ctx context.Context, query string, history []model.ChatMessage) (string, bool)
	Set(ctx context.Context, query string, history []model.ChatMessage, answer string) error
}

type redisCacheService struct {
	rdb *redis.Client
}

func NewContentCacheService() ContentCacheService {
	return &redisCacheService{
		rdb: database.RDB,
	}
}

const cacheTTL = 24 * time.Hour

// Get 尝试从缓存获取答案
// 目前实现为精确匹配（Plan B），未来可升级为向量语义匹配
func (s *redisCacheService) Get(ctx context.Context, query string, history []model.ChatMessage) (string, bool) {
	key := s.generateKey(query, history)
	val, err := s.rdb.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", false
	}
	if err != nil {
		log.Errorf("[CacheService] Get cache error: %v", err)
		return "", false
	}
	log.Infof("[CacheService] Cache HIT for query: %s", query)
	return val, true
}

func (s *redisCacheService) Set(ctx context.Context, query string, history []model.ChatMessage, answer string) error {
	key := s.generateKey(query, history)
	err := s.rdb.Set(ctx, key, answer, cacheTTL).Err()
	if err != nil {
		log.Errorf("[CacheService] Set cache error: %v", err)
	}
	return err
}

func (s *redisCacheService) generateKey(query string, history []model.ChatMessage) string {
	// 1. Normalize Query
	normalizedQuery := strings.TrimSpace(strings.ToLower(query))

	// 2. Context Hash (使用最后一条历史消息及其角色来代表上下文)
	// 如果没有历史，就是空。如果有，取最后一条。
	// 这是一种简化的上下文匹配，避免必须完全一样的历史才能命中。
	contextHash := ""
	if len(history) > 0 {
		lastMsg := history[len(history)-1]
		contextHash = hashString(fmt.Sprintf("%s:%s", lastMsg.Role, lastMsg.Content))
	}

	// 3. Combine
	combined := fmt.Sprintf("cache:answer:%s:%s", hashString(normalizedQuery), contextHash)
	return combined
}

func hashString(text string) string {
	hash := md5.Sum([]byte(text))
	return hex.EncodeToString(hash[:])
}
