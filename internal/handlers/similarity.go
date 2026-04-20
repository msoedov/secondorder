package handlers

import (
	"encoding/json"
	"math"
	"net/http"
	"strings"
)

const (
	// SimilarityThreshold is the minimum normalized Levenshtein similarity (0–1)
	// required to include an issue in similarity results.
	SimilarityThreshold = 0.70
)

// SimilarIssueResult is a single result returned by the similarity endpoint.
type SimilarIssueResult struct {
	Key             string  `json:"key"`
	Title           string  `json:"title"`
	SimilarityScore float64 `json:"similarity_score"`
}

// SimilarIssues handles POST /api/v1/issues/similarity.
// Body: {"title": "proposed title"}
// Returns a JSON array of {key, title, similarity_score} for open/in-progress
// issues whose title similarity exceeds SimilarityThreshold, sorted desc.
func (a *API) SimilarIssues(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Title string `json:"title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Title) == "" {
		jsonError(w, "title required", http.StatusBadRequest)
		return
	}

	candidates, err := a.db.ListActiveIssueTitles()
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var results []SimilarIssueResult
	for _, c := range candidates {
		score := titleSimilarity(body.Title, c.Title)
		if score >= SimilarityThreshold {
			results = append(results, SimilarIssueResult{
				Key:             c.Key,
				Title:           c.Title,
				SimilarityScore: math.Round(score*1000) / 1000,
			})
		}
	}

	// Sort by similarity_score descending (insertion sort is fine for small slices).
	for i := 1; i < len(results); i++ {
		for j := i; j > 0 && results[j].SimilarityScore > results[j-1].SimilarityScore; j-- {
			results[j], results[j-1] = results[j-1], results[j]
		}
	}

	if results == nil {
		results = []SimilarIssueResult{}
	}
	jsonOK(w, results)
}

// titleSimilarity returns the normalized Levenshtein similarity between two strings,
// case-folded. Result is in [0.0, 1.0]: 1.0 = identical, 0.0 = completely different.
func titleSimilarity(a, b string) float64 {
	a = strings.ToLower(strings.TrimSpace(a))
	b = strings.ToLower(strings.TrimSpace(b))
	if a == b {
		return 1.0
	}
	maxLen := len([]rune(a))
	if lb := len([]rune(b)); lb > maxLen {
		maxLen = lb
	}
	if maxLen == 0 {
		return 1.0
	}
	d := levenshtein(a, b)
	return 1.0 - float64(d)/float64(maxLen)
}

// levenshtein computes the edit distance between two strings.
func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	la, lb := len(ra), len(rb)

	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}

	prev := make([]int, lb+1)
	curr := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}

	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			del := prev[j] + 1
			ins := curr[j-1] + 1
			sub := prev[j-1] + cost
			curr[j] = min3(del, ins, sub)
		}
		prev, curr = curr, prev
	}
	return prev[lb]
}

func min3(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}
