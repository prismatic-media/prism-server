package sqlite_test

import (
	"context"
	"testing"

	"github.com/ringmaster217/prism/internal/store/sqlite"
)

func TestGetSetting_NotFound(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	_, err := sqlite.GetSetting(ctx, db, "nonexistent_key")
	if err != sqlite.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestSetAndGetSetting(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := sqlite.SetSetting(ctx, db, "tmdb_api_key", "abc123"); err != nil {
		t.Fatalf("SetSetting: %v", err)
	}

	val, err := sqlite.GetSetting(ctx, db, "tmdb_api_key")
	if err != nil {
		t.Fatalf("GetSetting: %v", err)
	}
	if val != "abc123" {
		t.Errorf("got %q, want %q", val, "abc123")
	}
}

func TestSetSetting_Upsert(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := sqlite.SetSetting(ctx, db, "ffmpeg_hwaccel", "vaapi"); err != nil {
		t.Fatalf("SetSetting first: %v", err)
	}
	if err := sqlite.SetSetting(ctx, db, "ffmpeg_hwaccel", "nvenc"); err != nil {
		t.Fatalf("SetSetting second: %v", err)
	}

	val, err := sqlite.GetSetting(ctx, db, "ffmpeg_hwaccel")
	if err != nil {
		t.Fatalf("GetSetting: %v", err)
	}
	if val != "nvenc" {
		t.Errorf("got %q, want %q", val, "nvenc")
	}
}

func TestGetAllSettings_OnlyConfigurable(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := sqlite.BootstrapSettings(ctx, db); err != nil {
		t.Fatalf("BootstrapSettings: %v", err)
	}

	settings, err := sqlite.GetAllSettings(ctx, db, true)
	if err != nil {
		t.Fatalf("GetAllSettings: %v", err)
	}

	// jwt_secret and setup_complete must not appear
	if _, ok := settings["jwt_secret"]; ok {
		t.Error("jwt_secret should not appear in configurable settings")
	}
	if _, ok := settings["setup_complete"]; ok {
		t.Error("setup_complete should not appear in configurable settings")
	}

	// All configurable keys must be present
	for _, key := range []string{"thumbs_dir", "transcode_workers", "transcode_poll_interval", "storage_min_free_bytes", "auto_transcode_on_discovery", "tmdb_api_key", "cast_receiver_app_id"} {
		if _, ok := settings[key]; !ok {
			t.Errorf("missing expected key %q", key)
		}
	}
}

func TestBootstrapSettings_Idempotent(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	// Pre-set a value
	if err := sqlite.SetSetting(ctx, db, "ffmpeg_hwaccel", "vaapi"); err != nil {
		t.Fatalf("SetSetting: %v", err)
	}

	// Bootstrap twice
	if err := sqlite.BootstrapSettings(ctx, db); err != nil {
		t.Fatalf("BootstrapSettings first: %v", err)
	}
	if err := sqlite.BootstrapSettings(ctx, db); err != nil {
		t.Fatalf("BootstrapSettings second: %v", err)
	}

	// Pre-set value must be preserved
	val, err := sqlite.GetSetting(ctx, db, "ffmpeg_hwaccel")
	if err != nil {
		t.Fatalf("GetSetting: %v", err)
	}
	if val != "vaapi" {
		t.Errorf("bootstrap overwrote existing value: got %q, want %q", val, "vaapi")
	}
}

func TestBootstrapSettings_GeneratesJWTSecret(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := sqlite.BootstrapSettings(ctx, db); err != nil {
		t.Fatalf("BootstrapSettings: %v", err)
	}

	secret, err := sqlite.GetSetting(ctx, db, "jwt_secret")
	if err != nil {
		t.Fatalf("GetSetting jwt_secret: %v", err)
	}
	if len(secret) < 32 {
		t.Errorf("jwt_secret too short: %q", secret)
	}
}

func TestBootstrapSettings_JWTSecretStable(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := sqlite.BootstrapSettings(ctx, db); err != nil {
		t.Fatalf("first bootstrap: %v", err)
	}
	first, _ := sqlite.GetSetting(ctx, db, "jwt_secret")

	if err := sqlite.BootstrapSettings(ctx, db); err != nil {
		t.Fatalf("second bootstrap: %v", err)
	}
	second, _ := sqlite.GetSetting(ctx, db, "jwt_secret")

	if first != second {
		t.Error("jwt_secret changed between bootstrap calls")
	}
}
