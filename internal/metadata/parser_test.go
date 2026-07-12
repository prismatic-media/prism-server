package metadata

import (
	"testing"
)

func TestParseTVEpisode_Basic(t *testing.T) {
	info, ok := ParseTVEpisode("Andor - s01e03 - Reckoning.mkv")
	if !ok {
		t.Fatal("expected match")
	}
	if info.ShowName != "Andor" {
		t.Errorf("ShowName: got %q, want %q", info.ShowName, "Andor")
	}
	if info.SeasonNumber != 1 {
		t.Errorf("SeasonNumber: got %d, want 1", info.SeasonNumber)
	}
	if info.EpisodeNumber != 3 {
		t.Errorf("EpisodeNumber: got %d, want 3", info.EpisodeNumber)
	}
	if info.EpisodeName != "Reckoning" {
		t.Errorf("EpisodeName: got %q, want %q", info.EpisodeName, "Reckoning")
	}
}

func TestParseTVEpisode_MultiWordShow(t *testing.T) {
	info, ok := ParseTVEpisode("Mission Impossible - s02e10 - The Setup.mkv")
	if !ok {
		t.Fatal("expected match")
	}
	if info.ShowName != "Mission Impossible" {
		t.Errorf("ShowName: got %q, want %q", info.ShowName, "Mission Impossible")
	}
	if info.SeasonNumber != 2 {
		t.Errorf("SeasonNumber: got %d, want 2", info.SeasonNumber)
	}
	if info.EpisodeNumber != 10 {
		t.Errorf("EpisodeNumber: got %d, want 10", info.EpisodeNumber)
	}
}

func TestParseTVEpisode_WithPath(t *testing.T) {
	info, ok := ParseTVEpisode("/media/tv/Andor - s01e03 - Reckoning.mkv")
	if !ok {
		t.Fatal("expected match")
	}
	if info.ShowName != "Andor" {
		t.Errorf("ShowName: got %q, want %q", info.ShowName, "Andor")
	}
}

func TestParseTVEpisode_NoMatch(t *testing.T) {
	_, ok := ParseTVEpisode("Inception (2010).mp4")
	if ok {
		t.Error("expected no match for movie filename")
	}
}

func TestParseTVEpisode_CaseInsensitive(t *testing.T) {
	info, ok := ParseTVEpisode("Andor - S01E03 - Reckoning.mkv")
	if !ok {
		t.Fatal("expected match")
	}
	if info.SeasonNumber != 1 || info.EpisodeNumber != 3 {
		t.Errorf("got S%dE%d, want S01E03", info.SeasonNumber, info.EpisodeNumber)
	}
}

func TestParseTVEpisode_BracketedWithTitle(t *testing.T) {
	info, ok := ParseTVEpisode("Arrow [01x15] Dodger.mp4")
	if !ok {
		t.Fatal("expected match")
	}
	if info.ShowName != "Arrow" {
		t.Errorf("ShowName: got %q, want %q", info.ShowName, "Arrow")
	}
	if info.SeasonNumber != 1 {
		t.Errorf("SeasonNumber: got %d, want 1", info.SeasonNumber)
	}
	if info.EpisodeNumber != 15 {
		t.Errorf("EpisodeNumber: got %d, want 15", info.EpisodeNumber)
	}
	if info.EpisodeName != "Dodger" {
		t.Errorf("EpisodeName: got %q, want %q", info.EpisodeName, "Dodger")
	}
}

func TestParseTVEpisode_BracketedWithoutTitle(t *testing.T) {
	info, ok := ParseTVEpisode("Arrow [01x15].mp4")
	if !ok {
		t.Fatal("expected match")
	}
	if info.ShowName != "Arrow" {
		t.Errorf("ShowName: got %q, want %q", info.ShowName, "Arrow")
	}
	if info.SeasonNumber != 1 {
		t.Errorf("SeasonNumber: got %d, want 1", info.SeasonNumber)
	}
	if info.EpisodeNumber != 15 {
		t.Errorf("EpisodeNumber: got %d, want 15", info.EpisodeNumber)
	}
	if info.EpisodeName != "Episode 15" {
		t.Errorf("EpisodeName: got %q, want %q", info.EpisodeName, "Episode 15")
	}
}

func TestParseTVEpisode_SxEWithoutTitle(t *testing.T) {
	info, ok := ParseTVEpisode("Andor - s01e03.mkv")
	if !ok {
		t.Fatal("expected match")
	}
	if info.ShowName != "Andor" {
		t.Errorf("ShowName: got %q, want %q", info.ShowName, "Andor")
	}
	if info.SeasonNumber != 1 {
		t.Errorf("SeasonNumber: got %d, want 1", info.SeasonNumber)
	}
	if info.EpisodeNumber != 3 {
		t.Errorf("EpisodeNumber: got %d, want 3", info.EpisodeNumber)
	}
	if info.EpisodeName != "Episode 3" {
		t.Errorf("EpisodeName: got %q, want %q", info.EpisodeName, "Episode 3")
	}
}

func TestParseTVEpisode_MultiDigitSeason(t *testing.T) {
	info, ok := ParseTVEpisode("ShowName - s102e304 - EpisodeTitle.mkv")
	if !ok {
		t.Fatal("expected match")
	}
	if info.ShowName != "ShowName" {
		t.Errorf("ShowName: got %q, want %q", info.ShowName, "ShowName")
	}
	if info.SeasonNumber != 102 {
		t.Errorf("SeasonNumber: got %d, want 102", info.SeasonNumber)
	}
	if info.EpisodeNumber != 304 {
		t.Errorf("EpisodeNumber: got %d, want 304", info.EpisodeNumber)
	}
	if info.EpisodeName != "EpisodeTitle" {
		t.Errorf("EpisodeName: got %q, want %q", info.EpisodeName, "EpisodeTitle")
	}
}

func TestParseTVEpisode_MultiEpisodeSxEWithTitle(t *testing.T) {
	info, ok := ParseTVEpisode("Friends - s09e23-e24 - The One In Barbados.mp4")
	if !ok {
		t.Fatal("expected match")
	}
	if info.ShowName != "Friends" {
		t.Errorf("ShowName: got %q, want %q", info.ShowName, "Friends")
	}
	if info.SeasonNumber != 9 {
		t.Errorf("SeasonNumber: got %d, want 9", info.SeasonNumber)
	}
	if info.EpisodeNumber != 23 {
		t.Errorf("EpisodeNumber: got %d, want 23", info.EpisodeNumber)
	}
	if info.EpisodeName != "The One In Barbados" {
		t.Errorf("EpisodeName: got %q, want %q", info.EpisodeName, "The One In Barbados")
	}
}

func TestParseTVEpisode_MultiEpisodeSxEWithoutTitle(t *testing.T) {
	info, ok := ParseTVEpisode("Friends - s09e23-e24.mp4")
	if !ok {
		t.Fatal("expected match")
	}
	if info.ShowName != "Friends" {
		t.Errorf("ShowName: got %q, want %q", info.ShowName, "Friends")
	}
	if info.SeasonNumber != 9 {
		t.Errorf("SeasonNumber: got %d, want 9", info.SeasonNumber)
	}
	if info.EpisodeNumber != 23 {
		t.Errorf("EpisodeNumber: got %d, want 23", info.EpisodeNumber)
	}
	if info.EpisodeName != "Episode 23" {
		t.Errorf("EpisodeName: got %q, want %q", info.EpisodeName, "Episode 23")
	}
}

func TestParseTVEpisode_MultiEpisodeBracketedWithTitle(t *testing.T) {
	info, ok := ParseTVEpisode("Arrow [01x15-16] Dodger.mp4")
	if !ok {
		t.Fatal("expected match")
	}
	if info.ShowName != "Arrow" {
		t.Errorf("ShowName: got %q, want %q", info.ShowName, "Arrow")
	}
	if info.SeasonNumber != 1 {
		t.Errorf("SeasonNumber: got %d, want 1", info.SeasonNumber)
	}
	if info.EpisodeNumber != 15 {
		t.Errorf("EpisodeNumber: got %d, want 15", info.EpisodeNumber)
	}
	if info.EpisodeName != "Dodger" {
		t.Errorf("EpisodeName: got %q, want %q", info.EpisodeName, "Dodger")
	}
}

func TestParseTVEpisode_MultiEpisodeBracketedWithoutTitle(t *testing.T) {
	info, ok := ParseTVEpisode("Arrow [01x15-16].mp4")
	if !ok {
		t.Fatal("expected match")
	}
	if info.ShowName != "Arrow" {
		t.Errorf("ShowName: got %q, want %q", info.ShowName, "Arrow")
	}
	if info.SeasonNumber != 1 {
		t.Errorf("SeasonNumber: got %d, want 1", info.SeasonNumber)
	}
	if info.EpisodeNumber != 15 {
		t.Errorf("EpisodeNumber: got %d, want 15", info.EpisodeNumber)
	}
	if info.EpisodeName != "Episode 15" {
		t.Errorf("EpisodeName: got %q, want %q", info.EpisodeName, "Episode 15")
	}
}

func TestParseTitle_ParenYear(t *testing.T) {
	title, year := ParseTitle("Inception (2010).mp4")
	if title != "Inception" {
		t.Errorf("title: got %q, want %q", title, "Inception")
	}
	if year != 2010 {
		t.Errorf("year: got %d, want 2010", year)
	}
}

func TestParseTitle_DotYear(t *testing.T) {
	title, year := ParseTitle("The.Dark.Knight.2008.1080p.BluRay.mkv")
	if title != "The Dark Knight" {
		t.Errorf("title: got %q, want %q", title, "The Dark Knight")
	}
	if year != 2008 {
		t.Errorf("year: got %d, want 2008", year)
	}
}

func TestParseTitle_SpaceYear(t *testing.T) {
	title, year := ParseTitle("The Matrix 1999 BluRay.mkv")
	if title != "The Matrix" {
		t.Errorf("title: got %q, want %q", title, "The Matrix")
	}
	if year != 1999 {
		t.Errorf("year: got %d, want 1999", year)
	}
}

func TestParseTitle_NoYear(t *testing.T) {
	title, year := ParseTitle("Some Show S01E01.mkv")
	if title != "Some Show S01E01" {
		t.Errorf("title: got %q, want %q", title, "Some Show S01E01")
	}
	if year != 0 {
		t.Errorf("year: got %d, want 0", year)
	}
}

func TestParseTitle_NoExtension(t *testing.T) {
	title, year := ParseTitle("Interstellar (2014)")
	if title != "Interstellar" {
		t.Errorf("title: got %q, want %q", title, "Interstellar")
	}
	if year != 2014 {
		t.Errorf("year: got %d, want 2014", year)
	}
}

func TestParseTitle_StripDirectory(t *testing.T) {
	title, year := ParseTitle("/media/movies/The.Godfather.1972.mkv")
	if title != "The Godfather" {
		t.Errorf("title: got %q, want %q", title, "The Godfather")
	}
	if year != 1972 {
		t.Errorf("year: got %d, want 1972", year)
	}
}

func TestParseTitle_Underscores(t *testing.T) {
	title, year := ParseTitle("Fight_Club_1999.mkv")
	if title != "Fight Club" {
		t.Errorf("title: got %q, want %q", title, "Fight Club")
	}
	if year != 1999 {
		t.Errorf("year: got %d, want 1999", year)
	}
}

func TestParseTVEpisode_WithYearInName(t *testing.T) {
	info, ok := ParseTVEpisode("Bluey (2018) - s01e01 - Magic Xylophone.mp4")
	if !ok {
		t.Fatal("expected match")
	}
	if info.ShowName != "Bluey" {
		t.Errorf("ShowName: got %q, want %q", info.ShowName, "Bluey")
	}
	if info.Year != 2018 {
		t.Errorf("Year: got %d, want 2018", info.Year)
	}
}

func TestShowNameFromPath(t *testing.T) {
	tests := []struct {
		filePath    string
		libraryRoot string
		wantName    string
		wantYear    int
	}{
		{
			filePath:    "/media/tv/Bluey (2018)/Season 1/Bluey (2018) - s01e01.mp4",
			libraryRoot: "/media/tv",
			wantName:    "Bluey",
			wantYear:    2018,
		},
		{
			filePath:    "/media/tv/Friends/Friends - s01e01.mp4",
			libraryRoot: "/media/tv",
			wantName:    "Friends",
			wantYear:    0,
		},
		{
			filePath:    "/media/tv/Bluey - s01e01.mp4",
			libraryRoot: "/media/tv",
			wantName:    "",
			wantYear:    0,
		},
	}

	for _, tc := range tests {
		name, year := ShowNameFromPath(tc.filePath, tc.libraryRoot)
		if name != tc.wantName {
			t.Errorf("ShowNameFromPath(%q, %q) name: got %q, want %q", tc.filePath, tc.libraryRoot, name, tc.wantName)
		}
		if year != tc.wantYear {
			t.Errorf("ShowNameFromPath(%q, %q) year: got %d, want %d", tc.filePath, tc.libraryRoot, year, tc.wantYear)
		}
	}
}

