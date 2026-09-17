// Command server runs the tapo-manager backend: it reads RTSP camera feeds,
// remuxes them to fragmented MP4 via ffmpeg, and serves both the camera
// list and each camera's live video over a WebSocket to the React frontend.
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"tapo-manager/backend/internal/camera"
	"tapo-manager/backend/internal/config"
	"tapo-manager/backend/internal/httpapi"
	"tapo-manager/backend/internal/stream"
)

func main() {
	camerasPath := "cameras.json"
	if v := os.Getenv("CAMERAS_CONFIG"); v != "" {
		camerasPath = v
	}

	cfg, err := config.Load(camerasPath)
	if err != nil {
		log.Fatalf("loading config: %v", err)
	}

	// Cameras added through the API live in a separate, writable file: the
	// base config (cfg.Cameras) is commonly mounted read-only since it holds
	// durably-managed credentials.
	runtimeCameras, err := config.LoadCameras(cfg.RuntimeCamerasPath)
	if err != nil {
		log.Fatalf("loading runtime camera config: %v", err)
	}
	baseIDs := make(map[string]bool, len(cfg.Cameras))
	for _, c := range cfg.Cameras {
		baseIDs[c.ID] = true
	}
	allCameras := append([]camera.Camera(nil), cfg.Cameras...)
	for _, c := range runtimeCameras {
		if baseIDs[c.ID] {
			log.Printf("runtime camera config %s: id %q also exists in %s, ignoring the runtime entry", cfg.RuntimeCamerasPath, c.ID, camerasPath)
			continue
		}
		allCameras = append(allCameras, c)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	mgr := stream.NewManager(cfg.FFmpegPath)
	if err := mgr.Start(ctx, allCameras); err != nil {
		log.Fatalf("starting streams: %v", err)
	}
	defer mgr.Stop()

	store := camera.NewStore(cfg.Cameras, func(added []camera.Camera) error {
		return config.Save(cfg.RuntimeCamerasPath, added)
	})
	store.SeedRuntime(runtimeCameras)

	mux := httpapi.NewMux(store, mgr)
	srv := &http.Server{
		Addr:    cfg.Addr,
		Handler: httpapi.WithCORS(mux),
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("server shutdown: %v", err)
		}
	}()

	log.Printf("tapo-manager backend listening on %s (%d camera(s) configured)", cfg.Addr, len(allCameras))
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}
