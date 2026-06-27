package config

import (
	"flag"
	"os"
	"strconv"
)

// Config holds startup-only configuration — values known before the database
// is opened. All runtime settings live in the database settings table.
type Config struct {
	DBPath string
	Port   int
}

// RuntimeSettings holds settings loaded from the database at startup. These
// are read once and wired into subsystems; changes take effect after a restart.
type RuntimeSettings struct {
	JWTSecret         string
	ThumbsDir         string
	FFmpegHWAccel     string
	TranscodeWorkers  int
	TMDBApiKey        string
	CastReceiverAppID string
}

// Load parses startup configuration from flags and environment variables.
// Flag values take precedence over environment variables.
//
//	--db   / PRISM_DB    path to the SQLite database file (default: prism.db)
//	--port / PRISM_PORT  HTTP listen port (default: 8080)
func Load() *Config {
	cfg := &Config{
		DBPath: envOrDefault("PRISM_DB", "prism.db"),
		Port:   envIntOrDefault("PRISM_PORT", 8080),
	}

	flag.StringVar(&cfg.DBPath, "db", cfg.DBPath, "path to SQLite database file (env: PRISM_DB)")
	flag.IntVar(&cfg.Port, "port", cfg.Port, "HTTP listen port (env: PRISM_PORT)")
	flag.Parse()

	return cfg
}

// RuntimeSettingsFromMap converts the raw string map from the settings store
// into a typed RuntimeSettings struct.
func RuntimeSettingsFromMap(m map[string]string) RuntimeSettings {
	return RuntimeSettings{
		JWTSecret:         m["jwt_secret"],
		ThumbsDir:         m["thumbs_dir"],
		FFmpegHWAccel:     stringOrDefault(m["ffmpeg_hwaccel"], "none"),
		TranscodeWorkers:  intOrDefault(m["transcode_workers"], 1),
		TMDBApiKey:        m["tmdb_api_key"],
		CastReceiverAppID: m["cast_receiver_app_id"],
	}
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envIntOrDefault(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func stringOrDefault(s, def string) string {
	if s != "" {
		return s
	}
	return def
}

func intOrDefault(s string, def int) int {
	if n, err := strconv.Atoi(s); err == nil && n > 0 {
		return n
	}
	return def
}
