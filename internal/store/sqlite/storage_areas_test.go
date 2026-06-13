package sqlite_test

import (
	"context"
	"testing"

	"github.com/prismatic-media/prism-server/internal/models"
	"github.com/prismatic-media/prism-server/internal/store/sqlite"
)

func TestBootstrapStorageAreas_NoDefaultsCreated(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := sqlite.BootstrapSettings(ctx, db); err != nil {
		t.Fatalf("BootstrapSettings: %v", err)
	}

	areas, err := sqlite.ListStorageAreas(ctx, db)
	if err != nil {
		t.Fatalf("ListStorageAreas: %v", err)
	}
	if len(areas) != 0 {
		t.Fatalf("expected 0 default storage areas, got %d", len(areas))
	}
}

func TestCreateAndUpdateStorageArea(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := sqlite.BootstrapSettings(ctx, db); err != nil {
		t.Fatalf("BootstrapSettings: %v", err)
	}

	area := &models.StorageArea{Kind: models.StorageAreaKindSegments, Path: "/mnt/segments2", Enabled: true}
	if err := sqlite.CreateStorageArea(ctx, db, area); err != nil {
		t.Fatalf("CreateStorageArea: %v", err)
	}

	if err := sqlite.UpdateStorageArea(ctx, db, area.ID, "/mnt/segments2-renamed", false); err != nil {
		t.Fatalf("UpdateStorageArea: %v", err)
	}

	got, err := sqlite.GetStorageAreaByID(ctx, db, area.ID)
	if err != nil {
		t.Fatalf("GetStorageAreaByID: %v", err)
	}
	if got.Path != "/mnt/segments2-renamed" {
		t.Fatalf("path=%q want %q", got.Path, "/mnt/segments2-renamed")
	}
	if got.Enabled {
		t.Fatalf("enabled=%v want false", got.Enabled)
	}
}
