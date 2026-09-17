// Package config loads server and camera configuration for the backend.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"tapo-manager/backend/internal/camera"
)

// Config holds everything the server needs to boot.
type Config struct {
	Addr string
	// RuntimeCamerasPath is where cameras added through the API are
	// persisted. Unlike the base camera file (often mounted read-only, since
	// it holds durable/managed credentials), this path must be writable, so
	// it defaults to living inside DataDir, which is expected to be a
	// persistent volume.
	RuntimeCamerasPath string
	FFmpegPath         string
	Cameras            []camera.Camera
}

// cameraEntry is the on-disk shape of a single camera in the config file.
// Password lives only here so it never round-trips through the domain
// camera.Camera type (which is also used for API responses).
type cameraEntry struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	RTSPURL  string `json:"rtspUrl"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

type fileConfig struct {
	Cameras []cameraEntry `json:"cameras"`
}

// Load reads server settings from the environment and the camera list from
// the JSON file at camerasPath. See cameras.example.json for the format.
func Load(camerasPath string) (Config, error) {
	dataDir := envOr("DATA_DIR", "data")
	cfg := Config{
		Addr:               envOr("ADDR", ":8080"),
		RuntimeCamerasPath: envOr("CAMERAS_RUNTIME_CONFIG", filepath.Join(dataDir, "cameras.runtime.json")),
		FFmpegPath:         envOr("FFMPEG_PATH", "ffmpeg"),
	}

	cameras, err := LoadCameras(camerasPath)
	if err != nil {
		return cfg, err
	}
	cfg.Cameras = cameras

	return cfg, nil
}

// LoadCameras reads a camera list from a JSON file in the cameras.json
// format. A missing file yields an empty (not error) list, so both the base
// config and an as-yet-unwritten runtime config load cleanly.
func LoadCameras(path string) ([]camera.Camera, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading camera config %s: %w", path, err)
	}

	var fc fileConfig
	if err := json.Unmarshal(raw, &fc); err != nil {
		return nil, fmt.Errorf("parsing camera config %s: %w", path, err)
	}

	cameras := make([]camera.Camera, 0, len(fc.Cameras))
	for _, c := range fc.Cameras {
		if c.ID == "" || c.RTSPURL == "" {
			return nil, fmt.Errorf("camera config %s: each camera needs an id and rtspUrl", path)
		}
		cameras = append(cameras, camera.Camera{
			ID:       c.ID,
			Name:     c.Name,
			RTSPURL:  c.RTSPURL,
			Username: c.Username,
			Password: c.Password,
		})
	}

	return cameras, nil
}

// Save writes the given cameras back to camerasPath in the on-disk format,
// preserving credentials so the file remains a complete config on restart.
func Save(camerasPath string, cameras []camera.Camera) error {
	fc := fileConfig{Cameras: make([]cameraEntry, 0, len(cameras))}
	for _, c := range cameras {
		fc.Cameras = append(fc.Cameras, cameraEntry{
			ID:       c.ID,
			Name:     c.Name,
			RTSPURL:  c.RTSPURL,
			Username: c.Username,
			Password: c.Password,
		})
	}

	raw, err := json.MarshalIndent(fc, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding camera config: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(camerasPath), 0o755); err != nil {
		return fmt.Errorf("creating directory for camera config %s: %w", camerasPath, err)
	}
	// 0600: this file holds RTSP credentials.
	if err := os.WriteFile(camerasPath, raw, 0o600); err != nil {
		return fmt.Errorf("writing camera config %s: %w", camerasPath, err)
	}
	return nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
