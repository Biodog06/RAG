package evaluation

import (
	"math"
	"sort"
)

// SearchResult 代表单条检索出的文档信息
type SearchResult struct {
	DocID string
	Score float64 // 检索得分
}

// GroundTruth 代表标准答案（Query 关联的正向文档及其相关度分值）
type GroundTruth struct {
	RelevantDocIDs map[string]int // DocID -> Relevance Grade (0-4, 0 表示不相关)
}

// CalculatePrecisionAtK 计算 Precision@K
// 公式: (Top K 中的相关文档数) / K
func CalculatePrecisionAtK(results []SearchResult, gt GroundTruth, k int) float64 {
	if k <= 0 || len(results) == 0 {
		return 0
	}
	n := min(k, len(results))
	relevantCount := 0
	for i := 0; i < n; i++ {
		if grade, ok := gt.RelevantDocIDs[results[i].DocID]; ok && grade > 0 {
			relevantCount++
		}
	}
	return float64(relevantCount) / float64(k)
}

// CalculateMRR 计算 Mean Reciprocal Rank (对于单个 Query)
// 如果结果集中第一个相关文档的排名是 rank，则得分为 1/rank。如果没搜到相关文档，得分为 0。
func CalculateMRR(results []SearchResult, gt GroundTruth) float64 {
	for i, res := range results {
		if grade, ok := gt.RelevantDocIDs[res.DocID]; ok && grade > 0 {
			return 1.0 / float64(i+1)
		}
	}
	return 0
}

// CalculateNDCG 计算 Normalized Discounted Cumulative Gain
// 公式: DCG / IDCG
func CalculateNDCG(results []SearchResult, gt GroundTruth, k int) float64 {
	if k <= 0 || len(results) == 0 {
		return 0
	}
	n := min(k, len(results))

	// 1. 计算实际的 DCG
	dcg := 0.0
	for i := 0; i < n; i++ {
		rel := 0.0
		if grade, ok := gt.RelevantDocIDs[results[i].DocID]; ok {
			rel = float64(grade)
		}
		// DCG 公式: rel_i / log2(i + 1 + 1) -> 索引从 0 开始所以是 i+2
		dcg += (math.Pow(2, rel) - 1.0) / math.Log2(float64(i+2))
	}

	// 2. 计算理想状态下的 IDCG (将所有相关文档按相关度降序排列)
	var allRelevantGrades []int
	for _, grade := range gt.RelevantDocIDs {
		if grade > 0 {
			allRelevantGrades = append(allRelevantGrades, grade)
		}
	}
	sort.Slice(allRelevantGrades, func(i, j int) bool {
		return allRelevantGrades[i] > allRelevantGrades[j]
	})

	idcg := 0.0
	idcgCount := min(k, len(allRelevantGrades))
	for i := 0; i < idcgCount; i++ {
		rel := float64(allRelevantGrades[i])
		idcg += (math.Pow(2, rel) - 1.0) / math.Log2(float64(i+2))
	}

	if idcg == 0 {
		return 0
	}
	return dcg / idcg
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
