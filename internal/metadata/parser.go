// Package metadata provides TMDB API integration and filename parsing for
// media enrichment.
package metadata

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var (
	// reParenYear matches "Some Title (2001)" or "Title(2001)".
	reParenYear = regexp.MustCompile(`^(.*?)\s*\((\d{4})\)`)

	// reDotYear matches "Some.Title.2001.1080p" or "Some Title 2001 BluRay"
	// where the year is in the range 1888–2099.
	reDotYear = regexp.MustCompile(`^(.*?)[\.\s](1[89]\d{2}|20\d{2})(?:[\.\s]|$)`)
)

// ParseTitle extracts a human-readable title and optional release year from a
// media filename. The directory component and extension are stripped before
// parsing. Returns year=0 if no year can be detected.
func ParseTitle(filename string) (title string, year int) {
	base := filepath.Base(filename)
	name := strings.TrimSuffix(base, filepath.Ext(base))

	// Normalise underscores to dots so separators are uniform for matching.
	norm := strings.ReplaceAll(name, "_", ".")

	// "Title (Year)" pattern takes priority.
	if m := reParenYear.FindStringSubmatch(norm); m != nil {
		y, _ := strconv.Atoi(m[2])
		return cleanTitle(m[1]), y
	}

	// "Title.Year." or "Title Year " pattern.
	if m := reDotYear.FindStringSubmatch(norm); m != nil {
		y, _ := strconv.Atoi(m[2])
		return cleanTitle(m[1]), y
	}

	return cleanTitle(name), 0
}

// cleanTitle replaces dots and underscores with spaces and trims whitespace.
func cleanTitle(s string) string {
	r := strings.NewReplacer(".", " ", "_", " ")
	return strings.TrimSpace(r.Replace(s))
}
