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

	// reTVEpisodePatterns lists the TV episode regex patterns to try in sequence:
	reTVEpisodePatterns = []*regexp.Regexp{
		// 1. "Show Name - s09e23-e24 - Episode Title" or "Show Name - s01e02 - Episode Title"
		regexp.MustCompile(`(?i)^(.+?)\s+-\s+s(\d+)e(\d+)(?:-e?\d+)*\s+-\s+(.+)$`),
		// 2. "Show Name [01x15-16] Episode Title" or "Show Name [01x02] Episode Title"
		regexp.MustCompile(`(?i)^(.+?)\s+\[(\d+)x(\d+)(?:-\d+)*\]\s+(.+)$`),
		// 3. "Show Name - s09e23-e24" or "Show Name - s01e02"
		regexp.MustCompile(`(?i)^(.+?)\s+-\s+s(\d+)e(\d+)(?:-e?\d+)*$`),
		// 4. "Show Name [01x15-16]" or "Show Name [01x02]"
		regexp.MustCompile(`(?i)^(.+?)\s+\[(\d+)x(\d+)(?:-\d+)*\]$`),
	}
)

// TVEpisodeInfo holds the structured fields parsed from a TV episode filename.
type TVEpisodeInfo struct {
	ShowName      string
	Year          int
	SeasonNumber  int
	EpisodeNumber int
	EpisodeName   string
}

// ShowNameFromPath walks up from the file path to find the first directory under the library root,
// then extracts the show name and optional year from that folder name.
// If the file is directly under the library root, it returns an empty show name.
func ShowNameFromPath(filePath, libraryRoot string) (name string, year int) {
	filePath = filepath.Clean(filePath)
	libraryRoot = filepath.Clean(libraryRoot)

	rel, err := filepath.Rel(libraryRoot, filePath)
	if err != nil || strings.HasPrefix(rel, "..") || rel == "." {
		return "", 0
	}

	parts := strings.Split(rel, string(filepath.Separator))
	if len(parts) <= 1 {
		// File is directly in the library root
		return "", 0
	}

	// The first component is the show-level folder name (e.g. "Bluey (2018)")
	showFolderName := parts[0]

	// Parse show name and year from the folder name using ParseTitle logic
	return ParseTitle(showFolderName)
}

// ParseTVEpisode attempts to parse a TV episode filename. The directory
// component and extension are stripped before matching. Returns (info, true)
// on success.
func ParseTVEpisode(filename string) (*TVEpisodeInfo, bool) {
	base := filepath.Base(filename)
	name := strings.TrimSuffix(base, filepath.Ext(base))

	for _, pattern := range reTVEpisodePatterns {
		m := pattern.FindStringSubmatch(name)
		if m != nil {
			season, _ := strconv.Atoi(m[2])
			episode, _ := strconv.Atoi(m[3])
			episodeName := ""
			if len(m) > 4 {
				episodeName = strings.TrimSpace(m[4])
			}
			if episodeName == "" {
				episodeName = "Episode " + strconv.Itoa(episode)
			}
			showName, year := ParseTitle(m[1])
			return &TVEpisodeInfo{
				ShowName:      showName,
				Year:          year,
				SeasonNumber:  season,
				EpisodeNumber: episode,
				EpisodeName:   episodeName,
			}, true
		}
	}

	return nil, false
}

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
