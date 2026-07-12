package metadata

import (
	"testing"
)

func TestJaccardSimilarity(t *testing.T) {
	tests := []struct {
		a, b     string
		expected float64
	}{
		{"Jurassic World", "Jurassic World", 1.0},
		{"Jurassic World", "Jurassic World Rebirth", 2.0 / 3.0}, // Intersect: {jurassic, world}. Union: {jurassic, world, rebirth}
		{"The Devil Wears Prada", "The Devil Wears Prada 2", 4.0 / 5.0},
		{"Inception", "Interstellar", 0.0},
		{"", "", 1.0},
		{"", "Inception", 0.0},
		{"Jurassic-World!", "jurassic world", 1.0}, // case insensitivity and punctuation removal
	}

	for _, tc := range tests {
		got := JaccardSimilarity(tc.a, tc.b)
		if got != tc.expected {
			t.Errorf("JaccardSimilarity(%q, %q) = %f, expected %f", tc.a, tc.b, got, tc.expected)
		}
	}
}

func TestRuntimeScore(t *testing.T) {
	tests := []struct {
		localDuration float64
		tmdbRuntime   int
		expected      float64
	}{
		{7200.0, 120, 1.0}, // exactly 120 mins (7200s)
		{7400.0, 120, 1.0}, // 7400s = 123.33 mins (diff = 3.33 mins <= 5 mins)
		{7500.0, 120, 1.0}, // 7500s = 125 mins (diff = 5 mins)
		{7800.0, 120, 0.8}, // 7800s = 130 mins (diff = 10 mins). 1.0 - (10-5)/25 = 0.8
		{9000.0, 120, 0.0}, // 9000s = 150 mins (diff = 30 mins)
		{9100.0, 120, 0.0}, // 9100s = 151.67 mins (diff = 31.67 mins > 30 mins)
		{0.0, 120, 0.0},
		{7200.0, 0, 0.0},
	}

	for _, tc := range tests {
		got := RuntimeScore(tc.localDuration, tc.tmdbRuntime)
		// Small float delta check
		if got < tc.expected-0.0001 || got > tc.expected+0.0001 {
			t.Errorf("RuntimeScore(%f, %d) = %f, expected %f", tc.localDuration, tc.tmdbRuntime, got, tc.expected)
		}
	}
}

func TestScoreCandidates(t *testing.T) {
	candidates := []TMDBResult{
		{ID: 1, Title: "Jurassic World", Popularity: 80.0},
		{ID: 2, Title: "Jurassic World Rebirth", Popularity: 50.0},
		{ID: 3, Title: "Jurassic World: Fallen Kingdom", Popularity: 60.0},
	}

	runtimes := map[int]int{
		1: 124, // Jurassic World
		2: 110, // Rebirth
		3: 128, // Fallen Kingdom
	}

	// Case 1: Local duration is 124 mins (7440 seconds). Jurassic World should rank first.
	scored := ScoreCandidates("Jurassic World", 7440.0, candidates, runtimes)
	if len(scored) != 3 {
		t.Fatalf("expected 3 scored candidates, got %d", len(scored))
	}
	if scored[0].Candidate.ID != 1 {
		t.Errorf("expected top candidate to be ID 1 (Jurassic World), got ID %d", scored[0].Candidate.ID)
	}

	// Case 2: No duration available. Should redistribute weight to 65% Title, 35% Popularity.
	scoredNoDuration := ScoreCandidates("Jurassic World", 0.0, candidates, runtimes)
	if scoredNoDuration[0].Candidate.ID != 1 {
		t.Errorf("expected top candidate to be ID 1, got ID %d", scoredNoDuration[0].Candidate.ID)
	}
}
