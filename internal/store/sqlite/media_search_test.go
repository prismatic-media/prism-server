package sqlite_test

import (
	"context"
	"testing"

	"github.com/prismatic-media/prism-server/internal/models"
	"github.com/prismatic-media/prism-server/internal/store/sqlite"
)

func TestSearchMedia(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	// 1. Create a Movie library and a TV Show library
	movieLib := newLib("/movies", models.MediaTypeMovie)
	if err := sqlite.CreateLibrary(ctx, db, movieLib); err != nil {
		t.Fatal(err)
	}

	showLib := newLib("/shows", models.MediaTypeTVShow)
	if err := sqlite.CreateLibrary(ctx, db, showLib); err != nil {
		t.Fatal(err)
	}

	// 2. Insert Movies
	m1 := newMovieItem(movieLib.ID, "Interstellar", "/movies/interstellar.mkv")
	if err := sqlite.UpsertMediaItem(ctx, db, m1); err != nil {
		t.Fatal(err)
	}
	// Enrich with metadata: TMDB, Year, Overview, PosterPath, Director, Cast
	cast1 := []models.CastMember{
		{Name: "Matthew McConaughey", Character: "Cooper"},
		{Name: "Anne Hathaway", Character: "Brand"},
	}
	if err := sqlite.UpdateMediaMetadata(ctx, db, m1.ID, 157336, 2014, "A team of explorers travel through a wormhole in space.", "/posters/interstellar.jpg", "Christopher Nolan", cast1, "", nil); err != nil {
		t.Fatal(err)
	}

	m2 := newMovieItem(movieLib.ID, "The Matrix", "/movies/matrix.mkv")
	if err := sqlite.UpsertMediaItem(ctx, db, m2); err != nil {
		t.Fatal(err)
	}
	cast2 := []models.CastMember{
		{Name: "Keanu Reeves", Character: "Neo"},
		{Name: "Laurence Fishburne", Character: "Morpheus"},
	}
	if err := sqlite.UpdateMediaMetadata(ctx, db, m2.ID, 603, 1999, "A computer hacker learns from mysterious rebels.", "/posters/matrix.jpg", "Lana Wachowski", cast2, "", nil); err != nil {
		t.Fatal(err)
	}

	// 3. Insert TV Shows
	s1 := &models.TVShow{
		LibraryID: showLib.ID,
		Name:      "Breaking Bad",
	}
	if err := sqlite.UpsertTVShow(ctx, db, s1); err != nil {
		t.Fatal(err)
	}
	cast3 := []models.CastMember{
		{Name: "Bryan Cranston", Character: "Walter White"},
		{Name: "Aaron Paul", Character: "Jesse Pinkman"},
	}
	if err := sqlite.UpdateTVShowMetadata(ctx, db, s1.ID, 1396, 2008, "A high school chemistry teacher starts producing meth.", "/posters/breaking_bad.jpg", "Vince Gilligan", cast3, "", nil); err != nil {
		t.Fatal(err)
	}

	s2 := &models.TVShow{
		LibraryID: showLib.ID,
		Name:      "Stranger Things",
	}
	if err := sqlite.UpsertTVShow(ctx, db, s2); err != nil {
		t.Fatal(err)
	}
	cast4 := []models.CastMember{
		{Name: "Millie Bobby Brown", Character: "Eleven"},
		{Name: "Winona Ryder", Character: "Joyce Byers"},
	}
	if err := sqlite.UpdateTVShowMetadata(ctx, db, s2.ID, 66732, 2016, "A young boy vanishes, a mother must confront terrifying forces.", "/posters/stranger_things.jpg", "The Duffer Brothers", cast4, "", nil); err != nil {
		t.Fatal(err)
	}

	// 4. Test Search Queries
	tests := []struct {
		name      string
		query     string
		wantCount int
		wantFirst string
		wantType  string
	}{
		{
			name:      "Search by title (Movie)",
			query:     "Interstellar",
			wantCount: 1,
			wantFirst: "Interstellar",
			wantType:  "movie",
		},
		{
			name:      "Search by name (TV Show)",
			query:     "Stranger",
			wantCount: 1,
			wantFirst: "Stranger Things",
			wantType:  "tvshow",
		},
		{
			name:      "Search by director (Movie)",
			query:     "Christopher Nolan",
			wantCount: 1,
			wantFirst: "Interstellar",
			wantType:  "movie",
		},
		{
			name:      "Search by director (TV Show)",
			query:     "Vince Gilligan",
			wantCount: 1,
			wantFirst: "Breaking Bad",
			wantType:  "tvshow",
		},
		{
			name:      "Search by cast member (Movie)",
			query:     "Keanu",
			wantCount: 1,
			wantFirst: "The Matrix",
			wantType:  "movie",
		},
		{
			name:      "Search by cast member (TV Show)",
			query:     "Aaron Paul",
			wantCount: 1,
			wantFirst: "Breaking Bad",
			wantType:  "tvshow",
		},
		{
			name:      "Search matching multiple items",
			query:     "space", // Interstellar overview contains "space", Stranger Things does not
			wantCount: 1,
			wantFirst: "Interstellar",
			wantType:  "movie",
		},
		{
			name:      "Search case-insensitive",
			query:     "matrix",
			wantCount: 1,
			wantFirst: "The Matrix",
			wantType:  "movie",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := sqlite.SearchMedia(ctx, db, tt.query)
			if err != nil {
				t.Fatalf("SearchMedia failed: %v", err)
			}
			if len(results) != tt.wantCount {
				t.Fatalf("got %d results, want %d", len(results), tt.wantCount)
			}
			if tt.wantCount > 0 {
				if results[0].Title != tt.wantFirst {
					t.Errorf("first result title = %q, want %q", results[0].Title, tt.wantFirst)
				}
				if results[0].MediaType != tt.wantType {
					t.Errorf("first result type = %q, want %q", results[0].MediaType, tt.wantType)
				}
			}
		})
	}
}
