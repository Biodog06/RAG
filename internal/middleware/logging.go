// Package middleware 存放 Gin 框架的中间件。
package middleware

import (
	"bytes"
	"io"
	"regexp"
	"time"

	"github.com/gin-gonic/gin"
	"pai-smart-go/pkg/log"
)

// bodyLogWriter 用于捕获响应体
type bodyLogWriter struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

// Write 实现了 io.Writer 接口，将响应写入 gin.ResponseWriter 和一个内部的 buffer
func (w bodyLogWriter) Write(b []byte) (int, error) {
	w.body.Write(b)
	return w.ResponseWriter.Write(b)
}

// RequestLogger 是一个 Gin 中间件，用于记录详细的请求和响应日志。
func RequestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 记录请求开始时间
		startTime := time.Now()

		// 读取并重新缓存请求体
		var requestBody []byte
		if c.Request.Body != nil {
			requestBody, _ = io.ReadAll(c.Request.Body)
		}
		// 将读取的请求体重新设置回 c.Request.Body，以便后续处理函数可以正常读取
		c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody))

		// 使用自定义的 ResponseWriter 捕获响应
		blw := &bodyLogWriter{body: bytes.NewBufferString(""), ResponseWriter: c.Writer}
		c.Writer = blw

		// 处理请求
		c.Next()

		// 计算延迟
		latency := time.Since(startTime)
		statusCode := c.Writer.Status()
		clientIP := c.ClientIP()
		method := c.Request.Method
		path := c.Request.URL.Path

		// 对日志内容进行敏感信息脱敏
		sanitizedReq := sanitizeLog(string(requestBody))
		sanitizedResp := sanitizeLog(blw.body.String())

		// 记录完整的请求和响应信息
		log.Infow("HTTP Request Log",
			"statusCode", statusCode,
			"latency", latency.String(),
			"clientIP", clientIP,
			"method", method,
			"path", path,
			"requestBody", sanitizedReq,
			"responseBody", sanitizedResp,
		)
	}
}

// sanitizeLog 对日志中的敏感字段进行脱敏（如 token, password, secret）。
func sanitizeLog(msg string) string {
	if msg == "" {
		return ""
	}
	// 正则匹配敏感字段，替换其值为 ***
	re := regexp.MustCompile(`("?token"?|"?password"?|"?secret"?)[:=]\s*"?([^"&\s,{}]+)"?`)
	return re.ReplaceAllString(msg, `$1:***`)
}

