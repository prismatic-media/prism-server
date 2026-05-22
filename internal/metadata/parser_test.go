package metadata

import (
	"testing"
)

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
