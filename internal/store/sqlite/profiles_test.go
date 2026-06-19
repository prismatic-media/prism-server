package sqlite_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/prismatic-media/prism-server/internal/models"
	"github.com/prismatic-media/prism-server/internal/store/sqlite"
)

func TestCreateTranscodeProfile(t *testing.T) {
	db := openTestDB(t)
	p := &models.TranscodeProfile{
		Name:          "2160p",
		Width:         3840,
		Height:        2160,
		VideoBitrateK: 15000,
		AudioBitrateK: 192,
		Codec:         "av1",
		IsActive:      true,
	}

	err := sqlite.CreateTranscodeProfile(context.Background(), db, p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if p.ID == uuid.Nil {
		t.Error("expected profile ID to be set")
	}

	got, err := sqlite.GetTranscodeProfile(context.Background(), db, p.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.Name != p.Name {
		t.Errorf("Name = %q, want %q", got.Name, p.Name)
	}
	if got.Codec != "av1" {
		t.Errorf("Codec = %q, want %q", got.Codec, "av1")
	}
	if !got.IsActive {
		t.Error("expected IsActive to be true")
	}
}

func TestListTranscodeProfiles(t *testing.T) {
	db := openTestDB(t)

	// In-memory migrations seed 9 default profiles (4 active H.264, 5 inactive AV1). Let's make sure they are listed.
	profiles, err := sqlite.ListTranscodeProfiles(context.Background(), db, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 9 default profiles seeded by migration
	if len(profiles) != 9 {
		t.Errorf("expected 9 seeded profiles, got %d", len(profiles))
	}

	// Let's create an inactive profile
	inactive := &models.TranscodeProfile{
		Name:          "4k_inactive",
		Width:         3840,
		Height:        2160,
		VideoBitrateK: 12000,
		AudioBitrateK: 192,
		Codec:         "hevc",
		IsActive:      false,
	}
	if err := sqlite.CreateTranscodeProfile(context.Background(), db, inactive); err != nil {
		t.Fatal(err)
	}

	all, err := sqlite.ListTranscodeProfiles(context.Background(), db, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 10 {
		t.Errorf("expected 10 profiles in total, got %d", len(all))
	}

	active, err := sqlite.ListTranscodeProfiles(context.Background(), db, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 4 {
		t.Errorf("expected 4 active profiles, got %d", len(active))
	}
}

func TestUpdateTranscodeProfile(t *testing.T) {
	db := openTestDB(t)

	// Fetch one of the seeded profiles
	profiles, err := sqlite.ListTranscodeProfiles(context.Background(), db, false)
	if err != nil {
		t.Fatal(err)
	}

	p := profiles[0]
	p.Name = "updated_name"
	p.IsActive = false

	if err := sqlite.UpdateTranscodeProfile(context.Background(), db, p); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := sqlite.GetTranscodeProfile(context.Background(), db, p.ID)
	if err != nil {
		t.Fatal(err)
	}

	if got.Name != "updated_name" {
		t.Errorf("Name = %q, want %q", got.Name, "updated_name")
	}
	if got.IsActive {
		t.Error("expected IsActive to be false")
	}
}

func TestDeleteTranscodeProfile(t *testing.T) {
	db := openTestDB(t)

	profiles, err := sqlite.ListTranscodeProfiles(context.Background(), db, false)
	if err != nil {
		t.Fatal(err)
	}

	p := profiles[0]
	if err := sqlite.DeleteTranscodeProfile(context.Background(), db, p.ID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = sqlite.GetTranscodeProfile(context.Background(), db, p.ID)
	if err == nil {
		t.Error("expected error getting deleted profile")
	}
}
