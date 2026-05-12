package service

import (
	"sort"
)

// RRFScore 计算单个文档的 RRF 分数
// rank: 文档在列表中的排名（从0开始）
// k: 平滑常数，通常取 60
func RRFScore(rank int, k float64) float64 {
	// rank 从 0 开始，加 1
	return 1.0 / (k + float64(rank+1))
}

// FusionResult RRF 融合结果结构
type FusionResult struct {
	Doc   interface{} // 通用文档结构，避免具体类型依赖
	ID    string      // 文档唯一标识
	Score float64     // RRF 总分
}

// ReciprocalRankFusion RRF 融合多路召回结果
// lists: 多个召回结果列表，每个列表是一个文档切片。
// getID: 从文档中提取唯一 ID 的函数。
// k: RRF 常数 (默认 60)。
func ReciprocalRankFusion(lists [][]interface{}, getID func(interface{}) string, k float64) []FusionResult {
	if k <= 0 {
		k = 60
	}

	scores := make(map[string]float64)
	docs := make(map[string]interface{})

	// 遍历每个列表
	for _, list := range lists {
		for rank, doc := range list {
			id := getID(doc)
			docs[id] = doc
			scores[id] += RRFScore(rank, k)
		}
	}

	// 转换为结果切片
	var results []FusionResult
	for id, score := range scores {
		results = append(results, FusionResult{
			Doc:   docs[id],
			ID:    id,
			Score: score,
		})
	}

	// 按分数降序排序
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	return results
}
