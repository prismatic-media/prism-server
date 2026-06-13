package sqlite

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
)

// Known user-configurable setting keys. jwt_secret and setup_complete are
// managed internally and are intentionally excluded from this list.
var configurableSettingKeys = map[string]struct{}{
	"thumbs_dir":                  {},
	"ffmpeg_hwaccel":              {},
	"transcode_workers":           {},
	"transcode_poll_interval":     {},
	"storage_min_free_bytes":      {},
	"auto_transcode_on_discovery": {},
	"tmdb_api_key":                {},
	"cast_receiver_app_id":        {},
}

// IsConfigurableKey reports whether key is a user-configurable setting.
func IsConfigurableKey(key string) bool {
	_, ok := configurableSettingKeys[key]
	return ok
}

// settingDefaults returns the default values for all settings that the
// bootstrap function will insert if not already present.
func settingDefaults() map[string]string {
	return map[string]string{
		"thumbs_dir":                  "",
		"ffmpeg_hwaccel":              "none",
		"transcode_workers":           "2",
		"transcode_poll_interval":     "15",
		"storage_min_free_bytes":      "21474836480",
		"auto_transcode_on_discovery": "false",
		"tmdb_api_key":                "",
		"cast_receiver_app_id":        "",
		"setup_complete":              "false",
	}
}

// GetSetting retrieves a setting value by key. Returns ErrNotFound if the key
// does not exist.
func GetSetting(ctx context.Context, db *sql.DB, key string) (string, error) {
	var value string
	err := db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("getting setting %q: %w", key, err)
	}
	return value, nil
}

// SetSetting inserts or updates a setting key-value pair.
func SetSetting(ctx context.Context, db *sql.DB, key, value string) error {
	_, err := db.ExecContext(ctx,
		`INSERT INTO settings (key, value) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key, value,
	)
	if err != nil {
		return fmt.Errorf("setting %q: %w", key, err)
	}
	return nil
}

// GetAllSettings returns all settings as a map. If onlyConfigurable is true,
// only user-configurable keys are returned (jwt_secret and setup_complete are
// excluded).
func GetAllSettings(ctx context.Context, db *sql.DB, onlyConfigurable bool) (map[string]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT key, value FROM settings`)
	if err != nil {
		return nil, fmt.Errorf("querying settings: %w", err)
	}
	defer func() { _ = rows.Close() }()

	result := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, fmt.Errorf("scanning setting: %w", err)
		}
		if onlyConfigurable && !IsConfigurableKey(k) {
			continue
		}
		result[k] = v
	}
	return result, rows.Err()
}

// BootstrapSettings ensures all default settings exist in the database and
// that jwt_secret is populated. It is safe to call on every startup — existing
// values are never overwritten.
func BootstrapSettings(ctx context.Context, db *sql.DB) error {
	defaults := settingDefaults()
	for key, value := range defaults {
		if err := insertSettingIfMissing(ctx, db, key, value); err != nil {
			return fmt.Errorf("bootstrapping setting %q: %w", key, err)
		}
	}

	// Ensure jwt_secret exists and is non-empty.
	secret, err := GetSetting(ctx, db, "jwt_secret")
	if err == ErrNotFound || secret == "" {
		generated, genErr := generateSecret(32)
		if genErr != nil {
			return fmt.Errorf("generating jwt_secret: %w", genErr)
		}
		if err := SetSetting(ctx, db, "jwt_secret", generated); err != nil {
			return fmt.Errorf("persisting jwt_secret: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("checking jwt_secret: %w", err)
	}

	if err := BootstrapStorageAreas(ctx, db); err != nil {
		return fmt.Errorf("bootstrapping storage areas: %w", err)
	}

	return nil
}

// insertSettingIfMissing inserts a key-value pair only if the key does not
// already exist in the settings table.
func insertSettingIfMissing(ctx context.Context, db *sql.DB, key, value string) error {
	_, err := db.ExecContext(ctx,
		`INSERT OR IGNORE INTO settings (key, value) VALUES (?, ?)`,
		key, value,
	)
	return err
}

// generateSecret returns a hex-encoded random secret of n bytes.
func generateSecret(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
