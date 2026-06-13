package sqlite_test

import (
	"context"
	"testing"

	"github.com/prismatic-media/prism-server/internal/models"
	"github.com/prismatic-media/prism-server/internal/store/sqlite"
)

func TestListTVShows_SortingIgnoreThe(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	lib := newLib("/shows", models.MediaTypeTVShow)
	if err := sqlite.CreateLibrary(ctx, db, lib); err != nil {
		t.Fatal(err)
	}

	// Insert shows in non-alphabetical order
	// Expected alphabetical sort order ignoring "The ":
	// 1. The Mandalorian (Mandalorian - M)
	// 2. The Office (Office - O)
	// 3. Severance (S)
	// 4. The Witcher (Witcher - W)
	shows := []*models.TVShow{
		{LibraryID: lib.ID, Name: "The Witcher"},
		{LibraryID: lib.ID, Name: "The Mandalorian"},
		{LibraryID: lib.ID, Name: "Severance"},
		{LibraryID: lib.ID, Name: "The Office"},
	}

	for _, show := range shows {
		if err := sqlite.UpsertTVShow(ctx, db, show); err != nil {
			t.Fatal(err)
		}
	}

	got, err := sqlite.ListTVShows(ctx, db, lib.ID)
	if err != nil {
		t.Fatal(err)
	}

	if len(got) != 4 {
		t.Fatalf("expected 4 shows, got %d", len(got))
	}

	expectedOrder := []string{"The Mandalorian", "The Office", "Severance", "The Witcher"}
	for i, name := range expectedOrder {
		if got[i].Name != name {
			t.Errorf("expected show %d to be %q, got %q", i, name, got[i].Name)
		}
	}
}
