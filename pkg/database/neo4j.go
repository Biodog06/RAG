package database

import (
	"context"
	"fmt"
	"pai-smart-go/internal/config"
	"pai-smart-go/pkg/log"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// Neo4jDriver 全局 Neo4j 驱动实例
var Neo4jDriver neo4j.DriverWithContext

// InitNeo4j 初始化 Neo4j 连接
func InitNeo4j(cfg config.Neo4jConfig) error {
	var err error
	Neo4jDriver, err = neo4j.NewDriverWithContext(
		cfg.URI,
		neo4j.BasicAuth(cfg.Username, cfg.Password, ""),
	)
	if err != nil {
		return fmt.Errorf("failed to create neo4j driver: %w", err)
	}

	// 验证连接
	ctx := context.Background()
	err = Neo4jDriver.VerifyConnectivity(ctx)
	if err != nil {
		return fmt.Errorf("failed to verify neo4j connectivity: %w", err)
	}

	log.Info("Neo4j 连接初始化成功")
	return nil
}

// CloseNeo4j 关闭 Neo4j 连接
func CloseNeo4j() {
	if Neo4jDriver != nil {
		Neo4jDriver.Close(context.Background())
	}
}
