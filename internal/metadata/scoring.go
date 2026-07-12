package metadata

import (
	"math"
	"regexp"
	"sort"
	"strings"
)

var wordRegex = regexp.MustCompile(`[a-zA-Z0-9]+`)

// JaccardSimilarity computes the word-level Jaccard similarity between two strings.
// It is case-insensitive and ignores punctuation.
func JaccardSimilarity(a, b string) float64 {
	aWords := tokenize(a)
	bWords := tokenize(b)

	if len(aWords) == 0 && len(bWords) == 0 {
		return 1.0
	}
	if len(aWords) == 0 || len(bWords) == 0 {
		return 0.0
	}

	intersection := 0
	unionMap := make(map[string]bool)

	for w := range aWords {
		unionMap[w] = true
		if bWords[w] {
			intersection++
		}
	}
	for w := range bWords {
		unionMap[w] = true
	}

	return float64(intersection) / float64(len(unionMap))
}

func tokenize(s string) map[string]bool {
	words := make(map[string]bool)
	matches := wordRegex.FindAllString(strings.ToLower(s), -1)
	for _, m := range matches {
		words[m] = true
	}
	return words
}

// RuntimeScore computes a score based on the difference between the local duration
// (in seconds) and the TMDB movie runtime (in minutes).
// It returns 1.0 if within ±5 minutes, decaying linearly to 0.0 at ±30 minutes.
func RuntimeScore(localDurationSecs float64, tmdbRuntimeMins int) float64 {
	if localDurationSecs <= 0 || tmdbRuntimeMins <= 0 {
		return 0.0
	}

	tmdbSecs := float64(tmdbRuntimeMins * 60)
	diffMins := math.Abs(localDurationSecs-tmdbSecs) / 60.0

	if diffMins <= 5.0 {
		return 1.0
	}
	if diffMins >= 30.0 {
		return 0.0
	}

	// Linear decay from 1.0 (at 5 mins) to 0.0 (at 30 mins)
	return 1.0 - (diffMins-5.0)/25.0
}

// ScoredCandidate represents a TMDB search candidate with individual signal scores.
type ScoredCandidate struct {
	Candidate       TMDBResult
	TitleScore      float64
	PopularityScore float64
	RuntimeScore    float64
	CompositeScore  float64
}

// ScoreCandidates evaluates all candidates and calculates their composite score.
// runtimes contains TMDB ID -> runtime in minutes.
// localDurationSecs is the media duration from ffprobe. If <= 0, runtime scoring is skipped.
func ScoreCandidates(query string, localDurationSecs float64, candidates []TMDBResult, runtimes map[int]int) []ScoredCandidate {
	if len(candidates) == 0 {
		return nil
	}

	// Find maximum popularity to normalize popularity scores.
	maxPopularity := 0.0
	for _, c := range candidates {
		if c.Popularity > maxPopularity {
			maxPopularity = c.Popularity
		}
	}

	scored := make([]ScoredCandidate, len(candidates))
	for i, c := range candidates {
		titleScore := JaccardSimilarity(query, c.Title)

		popScore := 0.0
		if maxPopularity > 0 {
			popScore = c.Popularity / maxPopularity
		} else {
			// If max popularity is 0, give everyone a neutral score.
			popScore = 1.0
		}

		rtScore := 0.0
		hasRuntime := false
		if localDurationSecs > 0 && runtimes != nil {
			if rt, ok := runtimes[c.ID]; ok && rt > 0 {
				rtScore = RuntimeScore(localDurationSecs, rt)
				hasRuntime = true
			}
		}

		var composite float64
		if hasRuntime {
			// Weights: Title (50%), Popularity (20%), Runtime (30%)
			composite = (0.50 * titleScore) + (0.20 * popScore) + (0.30 * rtScore)
		} else {
			// Weight redistribution: Title (65%), Popularity (35%)
			composite = (0.65 * titleScore) + (0.35 * popScore)
		}

		scored[i] = ScoredCandidate{
			Candidate:       c,
			TitleScore:      titleScore,
			PopularityScore: popScore,
			RuntimeScore:    rtScore,
			CompositeScore:  composite,
		}
	}

	// Sort by composite score descending. If tied, sort by popularity descending.
	sort.Slice(scored, func(i, j int) bool {
		if scored[i].CompositeScore == scored[j].CompositeScore {
			return scored[i].Candidate.Popularity > scored[j].Candidate.Popularity
		}
		return scored[i].CompositeScore > scored[j].CompositeScore
	})

	return scored
}
