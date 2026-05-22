package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	Port      int    `mapstructure:"port"`
	DBPath    string `mapstructure:"db_path"`
	JWTSecret string `mapstructure:"jwt_secret"`

	// Media storage
	MediaDir    string `mapstructure:"media_dir"`
	SegmentsDir string `mapstructure:"segments_dir"`
	ThumbsDir   string `mapstructure:"thumbs_dir"`

	// Transcoder
	FFmpegPath       string `mapstructure:"ffmpeg_path"`
	FFprobePath      string `mapstructure:"ffprobe_path"`
	TranscodeWorkers int    `mapstructure:"transcode_workers"`

	// External APIs
	TMDBApiKey string `mapstructure:"tmdb_api_key"`
}

func Load() (*Config, error) {
	v := viper.New()

	v.SetDefault("port", 8080)
	v.SetDefault("db_path", "galactic.db")
	v.SetDefault("media_dir", "/media")
	v.SetDefault("segments_dir", "/data/segments")
	v.SetDefault("thumbs_dir", "/data/thumbs")
	v.SetDefault("ffmpeg_path", "ffmpeg")
	v.SetDefault("ffprobe_path", "ffprobe")
	v.SetDefault("transcode_workers", 2)

	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	v.AddConfigPath("$HOME/.galactic")
	v.AddConfigPath("/etc/galactic")

	v.SetEnvPrefix("GALACTIC")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Viper's AutomaticEnv does not populate keys during Unmarshal without
	// explicit BindEnv calls. Bind every key we want to be overridable via env.
	for _, key := range []string{
		"port", "db_path", "jwt_secret",
		"media_dir", "segments_dir", "thumbs_dir",
		"ffmpeg_path", "ffprobe_path", "transcode_workers",
		"tmdb_api_key",
	} {
		_ = v.BindEnv(key)
	}

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("reading config: %w", err)
		}
	}

	cfg := &Config{}
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("unmarshaling config: %w", err)
	}

	if cfg.JWTSecret == "" {
		return nil, fmt.Errorf("jwt_secret is required (set GALACTIC_JWT_SECRET)")
	}

	return cfg, nil
}
