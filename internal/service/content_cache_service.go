// Package service 包含了语义缓存的业务逻辑。
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"pai-smart-go/pkg/embedding"
	"pai-smart-go/pkg/log"
	"time"

	"github.com/go-redis/redis/v8"
)

// ContentCacheService 定义了语义缓存的接口。
// 使用向量余弦相似度判断两个查询是否语义等价。
type ContentCacheService interface {
	// GetCachedResponse 查找语义相似的缓存响应。
	// 如果找到相似度 > threshold 的缓存，返回缓存的 LLM 响应；否则返回空字符串。
	GetCachedResponse(ctx context.Context, queryVector []float32) (string, bool)
	// CacheResponse 将查询向量与 LLM 响应存入缓存。
	CacheResponse(ctx context.Context, queryVector []float32, response string) error
}

// semanticCacheEntry 语义缓存条目
type semanticCacheEntry struct {
	Vector    []float32 `json:"vector"`
	Response  string    `json:"response"`
	Timestamp int64     `json:"timestamp"`
}

type contentCacheService struct {
	redisClient     *redis.Client
	embeddingClient embedding.Client
	threshold       float64       // 相似度阈值，默认 0.95
	ttl             time.Duration // 缓存过期时间
}

// NewContentCacheService 创建一个新的语义缓存服务实例。
func NewContentCacheService(opts ...ContentCacheOption) ContentCacheService {
	svc := &contentCacheService{
		threshold: 0.95,
		ttl:       24 * time.Hour,
	}
	for _, opt := range opts {
		opt(svc)
	}
	return svc
}

// ContentCacheOption 语义缓存配置选项
type ContentCacheOption func(*contentCacheService)

// WithRedisClient 设置 Redis 客户端
func WithRedisClient(client *redis.Client) ContentCacheOption {
	return func(s *contentCacheService) {
		s.redisClient = client
	}
}

// WithEmbeddingClient 设置 Embedding 客户端
func WithEmbeddingClient(client embedding.Client) ContentCacheOption {
	return func(s *contentCacheService) {
		s.embeddingClient = client
	}
}

// WithThreshold 设置相似度阈值
func WithThreshold(threshold float64) ContentCacheOption {
	return func(s *contentCacheService) {
		s.threshold = threshold
	}
}

const (
	semCacheKeyPrefix = "sem_cache:"
	semCacheIndexKey  = "sem_cache:index" // 用于存储所有缓存条目 key 的 Set
)

// GetCachedResponse 查找语义相似的缓存响应。
// 线性扫描所有缓存向量，计算余弦相似度。
func (s *contentCacheService) GetCachedResponse(ctx context.Context, queryVector []float32) (string, bool) {
	if s.redisClient == nil || len(queryVector) == 0 {
		return "", false
	}

	// 获取所有缓存条目的 key
	keys, err := s.redisClient.SMembers(ctx, semCacheIndexKey).Result()
	if err != nil {
		log.Warnf("[SemanticCache] 获取缓存索引失败: %v", err)
		return "", false
	}

	if len(keys) == 0 {
		return "", false
	}

	log.Infof("[SemanticCache] 开始扫描 %d 个缓存条目", len(keys))

	var bestSimilarity float64
	var bestResponse string

	for _, key := range keys {
		data, err := s.redisClient.Get(ctx, key).Result()
		if err != nil {
			// key 可能已过期，从索引中移除
			s.redisClient.SRem(ctx, semCacheIndexKey, key)
			continue
		}

		var entry semanticCacheEntry
		if err := json.Unmarshal([]byte(data), &entry); err != nil {
			continue
		}

		// 计算余弦相似度
		similarity := cosineSimilarity(queryVector, entry.Vector)
		if similarity > bestSimilarity {
			bestSimilarity = similarity
			bestResponse = entry.Response
		}
	}

	if bestSimilarity >= s.threshold {
		log.Infof("[SemanticCache] 缓存命中! 相似度: %.4f (阈值: %.4f)", bestSimilarity, s.threshold)
		return bestResponse, true
	}

	log.Infof("[SemanticCache] 未命中缓存, 最高相似度: %.4f (阈值: %.4f)", bestSimilarity, s.threshold)
	return "", false
}

// CacheResponse 将查询向量与 LLM 响应存入缓存。
func (s *contentCacheService) CacheResponse(ctx context.Context, queryVector []float32, response string) error {
	if s.redisClient == nil || len(queryVector) == 0 || response == "" {
		return nil
	}

	entry := semanticCacheEntry{
		Vector:    queryVector,
		Response:  response,
		Timestamp: time.Now().UnixMilli(),
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal cache entry: %w", err)
	}

	// 使用时间戳作为唯一 key
	key := fmt.Sprintf("%s%d", semCacheKeyPrefix, time.Now().UnixNano())

	// 存储缓存条目（带 TTL）
	if err := s.redisClient.Set(ctx, key, string(data), s.ttl).Err(); err != nil {
		return fmt.Errorf("failed to set cache entry: %w", err)
	}

	// 将 key 加入索引 Set
	if err := s.redisClient.SAdd(ctx, semCacheIndexKey, key).Err(); err != nil {
		log.Warnf("[SemanticCache] 添加缓存索引失败: %v", err)
	}

	log.Infof("[SemanticCache] 缓存写入成功, key: %s", key)
	return nil
}

// cosineSimilarity 计算两个向量的余弦相似度。
func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dotProduct, normA, normB float64
	for i := range a {
		dotProduct += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}

	denominator := math.Sqrt(normA) * math.Sqrt(normB)
	if denominator == 0 {
		return 0
	}

	return dotProduct / denominator
}
