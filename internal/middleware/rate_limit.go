package middleware

import (
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// RateLimiter 实现了简单的基于用户 ID 的限流。
type RateLimiter struct {
	limiters map[string]*rate.Limiter
	mu       sync.RWMutex
	r        rate.Limit
	b        int
}

// NewRateLimiter 创建一个限流器实例。
// r: 每秒允许的请求数；b: 令牌桶大小。
func NewRateLimiter(r rate.Limit, b int) *RateLimiter {
	return &RateLimiter{
		limiters: make(map[string]*rate.Limiter),
		r:        r,
		b:        b,
	}
}

func (rl *RateLimiter) getLimiter(userID string) *rate.Limiter {
	rl.mu.RLock()
	limiter, exists := rl.limiters[userID]
	rl.mu.RUnlock()

	if exists {
		return limiter
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()
	
	// 再次检查防止多写
	limiter, exists = rl.limiters[userID]
	if exists {
		return limiter
	}

	limiter = rate.NewLimiter(rl.r, rl.b)
	rl.limiters[userID] = limiter
	return limiter
}

// Handler 返回一个基于用户 ID 或 IP 的限流中间件。
func (rl *RateLimiter) Handler() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 优先获取 userID，未登录则尝试按 IP 限流
		var key string
		if _, exists := c.Get("user"); exists {
			key = c.GetString("user_id") 
			if key == "" {
				// 获取 Claims 里的 ID 或 Username
				key = c.GetString("username")
			}
		}
		
		if key == "" {
			key = c.ClientIP()
		}

		limiter := rl.getLimiter(key)
		if !limiter.Allow() {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": "请求过于频繁，请稍后再试",
			})
			return
		}

		c.Next()
	}
}
