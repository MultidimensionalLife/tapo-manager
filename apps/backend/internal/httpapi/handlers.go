// Package httpapi exposes the HTTP surface the frontend talks to: the
// camera list and the live WebSocket feed each camera's video player
// attaches to via MediaSource.
package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/gorilla/websocket"

	"tapo-manager/backend/internal/camera"
)

// CameraDTO is the API-facing shape of a camera. It intentionally omits the
// RTSP URL and credentials, exposing only the WebSocket path the frontend
// should attach a MediaSource-backed <video> element to.
type CameraDTO struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	StreamURL string `json:"streamUrl"`
}

var upgrader = websocket.Upgrader{
	// A LAN camera tool with no auth of its own; matches the permissive
	// CORS policy already used for the rest of the API.
	CheckOrigin: func(r *http.Request) bool { return true },
}

// addCameraRequest is the body accepted by POST /api/cameras.
type addCameraRequest struct {
	Name     string `json:"name"`
	RTSPURL  string `json:"rtspUrl"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

// Streamer starts and stops the ffmpeg pipeline for cameras added or
// removed at runtime, and hands out live viewers subscriptions to it.
type Streamer interface {
	AddCamera(c camera.Camera) error
	RemoveCamera(id string) error
	Subscribe(id string) (ch chan []byte, initSeg []byte, unsubscribe func(), ok bool)
}

// NewMux builds the backend's HTTP routes. cameras is the live camera
// registry (queried on every request, not just at boot) and streamer starts
// the ffmpeg pipeline for cameras added at runtime and serves live viewers.
func NewMux(cameras *camera.Store, streamer Streamer) *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	mux.HandleFunc("GET /api/cameras", func(w http.ResponseWriter, r *http.Request) {
		list := cameras.List()
		dtos := make([]CameraDTO, 0, len(list))
		for _, c := range list {
			dtos = append(dtos, toDTO(c))
		}
		writeJSON(w, dtos)
	})

	mux.HandleFunc("POST /api/cameras", func(w http.ResponseWriter, r *http.Request) {
		var req addCameraRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		req.Name = strings.TrimSpace(req.Name)
		req.RTSPURL = strings.TrimSpace(req.RTSPURL)

		if req.Name == "" {
			http.Error(w, "name is required", http.StatusBadRequest)
			return
		}
		u, err := url.Parse(req.RTSPURL)
		if err != nil || u.Scheme != "rtsp" || u.Host == "" {
			http.Error(w, "rtspUrl must be a valid rtsp:// URL", http.StatusBadRequest)
			return
		}

		added, err := cameras.Add(camera.Camera{
			Name:     req.Name,
			RTSPURL:  req.RTSPURL,
			Username: req.Username,
			Password: req.Password,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}

		if err := streamer.AddCamera(added); err != nil {
			http.Error(w, "camera saved but failed to start stream: "+err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusCreated)
		writeJSON(w, toDTO(added))
	})

	mux.HandleFunc("DELETE /api/cameras/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")

		if err := cameras.Remove(id); err != nil {
			switch {
			case errors.Is(err, camera.ErrNotFound):
				http.Error(w, err.Error(), http.StatusNotFound)
			case errors.Is(err, camera.ErrBaseCamera):
				http.Error(w, err.Error(), http.StatusForbidden)
			default:
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}

		if err := streamer.RemoveCamera(id); err != nil {
			http.Error(w, "camera removed but failed to stop stream: "+err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("GET /ws/cameras/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		ch, initSeg, unsubscribe, ok := streamer.Subscribe(id)
		if !ok {
			http.Error(w, "camera not found", http.StatusNotFound)
			return
		}
		defer unsubscribe()

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		if len(initSeg) > 0 {
			if err := conn.WriteMessage(websocket.BinaryMessage, initSeg); err != nil {
				return
			}
		}

		// The client never sends messages; this goroutine's only job is to
		// notice when it disconnects (or sends the WS close frame) so the
		// write loop below unblocks instead of leaking the subscription.
		go func() {
			for {
				if _, _, err := conn.ReadMessage(); err != nil {
					_ = conn.Close()
					return
				}
			}
		}()

		for chunk := range ch {
			if err := conn.WriteMessage(websocket.BinaryMessage, chunk); err != nil {
				return
			}
		}
	})

	return mux
}

func toDTO(c camera.Camera) CameraDTO {
	return CameraDTO{
		ID:        c.ID,
		Name:      c.Name,
		StreamURL: "/ws/cameras/" + c.ID,
	}
}

// WithCORS allows the frontend dev server (a different origin/port) to call
// the API. Fine for a LAN camera tool; tighten the origin for anything
// exposed beyond localhost.
func WithCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
