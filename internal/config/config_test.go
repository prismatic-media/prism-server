package config

import (
	"testing"
)

func TestRuntimeSettingsFromMap(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]string
		expected RuntimeSettings
	}{
		{
			name:  "defaults",
			input: map[string]string{},
			expected: RuntimeSettings{
				ThumbsDir:        "",
				FFmpegHWAccel:    "none",
				TranscodeWorkers: 1,
			},
		},
		{
			name: "custom values",
			input: map[string]string{
				"jwt_secret":           "secret123",
				"thumbs_dir":           "/custom/thumbs",
				"ffmpeg_hwaccel":       "nvenc",
				"transcode_workers":    "4",
				"tmdb_api_key":         "apikey",
				"cast_receiver_app_id": "castid",
			},
			expected: RuntimeSettings{
				JWTSecret:         "secret123",
				ThumbsDir:         "/custom/thumbs",
				FFmpegHWAccel:     "nvenc",
				TranscodeWorkers:  4,
				TMDBApiKey:        "apikey",
				CastReceiverAppID: "castid",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RuntimeSettingsFromMap(tt.input)
			if got.JWTSecret != tt.expected.JWTSecret {
				t.Errorf("JWTSecret = %q, want %q", got.JWTSecret, tt.expected.JWTSecret)
			}
			if got.ThumbsDir != tt.expected.ThumbsDir {
				t.Errorf("ThumbsDir = %q, want %q", got.ThumbsDir, tt.expected.ThumbsDir)
			}
			if got.FFmpegHWAccel != tt.expected.FFmpegHWAccel {
				t.Errorf("FFmpegHWAccel = %q, want %q", got.FFmpegHWAccel, tt.expected.FFmpegHWAccel)
			}
			if got.TranscodeWorkers != tt.expected.TranscodeWorkers {
				t.Errorf("TranscodeWorkers = %d, want %d", got.TranscodeWorkers, tt.expected.TranscodeWorkers)
			}
			if got.TMDBApiKey != tt.expected.TMDBApiKey {
				t.Errorf("TMDBApiKey = %q, want %q", got.TMDBApiKey, tt.expected.TMDBApiKey)
			}
			if got.CastReceiverAppID != tt.expected.CastReceiverAppID {
				t.Errorf("CastReceiverAppID = %q, want %q", got.CastReceiverAppID, tt.expected.CastReceiverAppID)
			}
		})
	}
}
