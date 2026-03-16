package service

import (
	"context"
	"fmt"
	"pai-smart-go/pkg/database"
	"pai-smart-go/pkg/llm"
	"pai-smart-go/pkg/log"
	"strings"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

type GraphSearchService interface {
	Search(ctx context.Context, query string) (string, error)
}

type graphSearchService struct {
	llmClient llm.Client
}

func NewGraphSearchService(llmClient llm.Client) GraphSearchService {
	return &graphSearchService{
		llmClient: llmClient,
	}
}

func (s *graphSearchService) Search(ctx context.Context, query string) (string, error) {
	if database.Neo4jDriver == nil {
		return "", fmt.Errorf("neo4j driver not initialized")
	}

	// 1. 从 Query 中提取可能的实体核心词 (使用简单的分词或 LLM)
	entities, err := s.extractEntities(ctx, query)
	if err != nil {
		log.Warnf("[GraphSearch] 提取实体失败: %v", err)
		return "", nil
	}

	if len(entities) == 0 {
		return "", nil
	}

	// 2. 在 Neo4j 中执行 1-hop 检索
	results, err := s.queryGraph(ctx, entities)
	if err != nil {
		return "", err
	}

	if len(results) == 0 {
		return "", nil
	}

	// 3. 格式化为摘要
	return s.formatGraphKnowledge(results), nil
}

func (s *graphSearchService) extractEntities(ctx context.Context, query string) ([]string, error) {
	prompt := fmt.Sprintf(`从以下用户问题中提取最核心的实体名词（如产品名、人名、项目名等）。
只返回实体列表，用逗号分隔。

问题："%s"`, query)

	messages := []llm.Message{
		{Role: "user", Content: prompt},
	}

	resp, err := s.llmClient.GenerateOneShot(ctx, messages)
	if err != nil {
		return nil, err
	}

	parts := strings.Split(resp, ",")
	var entities []string
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			entities = append(entities, trimmed)
		}
	}
	return entities, nil
}

type GraphResult struct {
	Subject   string
	Predicate string
	Object    string
}

func (s *graphSearchService) queryGraph(ctx context.Context, entities []string) ([]GraphResult, error) {
	session := database.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	results, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		cypher := `
		MATCH (n:Entity)-[r:RELATION]->(m:Entity)
		WHERE ANY(e IN $entities WHERE n.name CONTAINS e OR m.name CONTAINS e)
		RETURN n.name AS sub, r.type AS pred, m.name AS obj
		LIMIT 20
		`
		res, err := tx.Run(ctx, cypher, map[string]interface{}{"entities": entities})
		if err != nil {
			return nil, err
		}

		var list []GraphResult
		for res.Next(ctx) {
			record := res.Record()
			sub, _ := record.Get("sub")
			pred, _ := record.Get("pred")
			obj, _ := record.Get("obj")
			list = append(list, GraphResult{
				Subject:   sub.(string),
				Predicate: pred.(string),
				Object:    obj.(string),
			})
		}
		return list, nil
	})

	if err != nil {
		return nil, err
	}
	return results.([]GraphResult), nil
}

func (s *graphSearchService) formatGraphKnowledge(results []GraphResult) string {
	var sb strings.Builder
	sb.WriteString("发现以下关联知识（图谱）：\n")
	for _, r := range results {
		sb.WriteString(fmt.Sprintf("- %s --(%s)--> %s\n", r.Subject, r.Predicate, r.Object))
	}
	return sb.String()
}
