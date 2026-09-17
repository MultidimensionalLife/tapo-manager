// Package stream bridges RTSP camera feeds to browser-playable video by
// shelling out to ffmpeg. Browsers cannot play RTSP directly, so each
// camera's feed is continuously remuxed to fragmented MP4 and broadcast
// live over a WebSocket to any connected viewers via MediaSource.
package stream

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"sync"
	"time"

	"tapo-manager/backend/internal/camera"
)

// Manager owns one ffmpeg process per camera and keeps it alive.
type Manager struct {
	ffmpegPath string

	mu      sync.Mutex
	baseCtx context.Context
	cancels map[string]context.CancelFunc
	hubs    map[string]*hub
}

// NewManager creates a Manager that invokes ffmpeg at ffmpegPath (found on
// PATH if just "ffmpeg").
func NewManager(ffmpegPath string) *Manager {
	return &Manager{
		ffmpegPath: ffmpegPath,
		cancels:    make(map[string]context.CancelFunc),
		hubs:       make(map[string]*hub),
	}
}

// Start begins (re)streaming every camera's RTSP feed in the background. It
// returns once all camera goroutines have been launched.
func (m *Manager) Start(ctx context.Context, cameras []camera.Camera) error {
	m.mu.Lock()
	m.baseCtx = ctx
	m.mu.Unlock()

	for _, c := range cameras {
		if err := m.startCamera(ctx, c); err != nil {
			return err
		}
	}
	return nil
}

// AddCamera starts streaming a camera added after Start, under the same
// lifecycle (cancelled on Stop/shutdown) as the initial set.
func (m *Manager) AddCamera(c camera.Camera) error {
	m.mu.Lock()
	ctx := m.baseCtx
	m.mu.Unlock()
	if ctx == nil {
		return fmt.Errorf("camera %s: manager not started", c.ID)
	}
	return m.startCamera(ctx, c)
}

// RemoveCamera stops a running camera's ffmpeg process and disconnects any
// viewers. It's a no-op if the camera isn't currently streaming.
func (m *Manager) RemoveCamera(id string) error {
	m.mu.Lock()
	cancel, ok := m.cancels[id]
	if ok {
		delete(m.cancels, id)
	}
	h, hubOK := m.hubs[id]
	if hubOK {
		delete(m.hubs, id)
	}
	m.mu.Unlock()

	if ok {
		cancel()
	}
	if hubOK {
		h.reset()
	}
	return nil
}

// Subscribe registers a WebSocket viewer for camera id's live stream. It
// returns a channel of fMP4 chunks, the cached init segment to send first
// (nil if the stream hasn't produced one yet), and false if no such camera
// is currently configured.
func (m *Manager) Subscribe(id string) (ch chan []byte, initSeg []byte, unsubscribe func(), ok bool) {
	m.mu.Lock()
	h, ok := m.hubs[id]
	m.mu.Unlock()
	if !ok {
		return nil, nil, nil, false
	}
	ch, initSeg = h.subscribe()
	return ch, initSeg, func() { h.unsubscribe(ch) }, true
}

func (m *Manager) startCamera(ctx context.Context, c camera.Camera) error {
	h := newHub()

	camCtx, cancel := context.WithCancel(ctx)
	m.mu.Lock()
	m.cancels[c.ID] = cancel
	m.hubs[c.ID] = h
	m.mu.Unlock()

	go m.runWithRestart(camCtx, c, h)
	return nil
}

// runWithRestart keeps ffmpeg running for a camera, restarting it with
// exponential backoff if it exits (e.g. the camera drops off the network).
func (m *Manager) runWithRestart(ctx context.Context, c camera.Camera, h *hub) {
	backoff := time.Second
	const maxBackoff = 30 * time.Second

	for {
		if ctx.Err() != nil {
			return
		}

		start := time.Now()
		if err := m.runFFmpeg(ctx, c, h); err != nil && ctx.Err() == nil {
			log.Printf("camera %s: ffmpeg exited: %v", c.ID, err)
		}
		// Existing viewers built their MediaSource around this run's init
		// segment; a restart produces a new one, so they need to reconnect.
		h.reset()

		if ctx.Err() != nil {
			return
		}

		// A long, healthy run resets the backoff before we retry.
		if time.Since(start) > maxBackoff {
			backoff = time.Second
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}

		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

func (m *Manager) runFFmpeg(ctx context.Context, c camera.Camera, h *hub) error {
	streamingURL, err := c.StreamingURL()
	if err != nil {
		return fmt.Errorf("building stream URL: %w", err)
	}

	args := []string{
		"-rtsp_transport", "tcp",
		// Stalled cameras (dropped Wi-Fi, hung firmware) otherwise leave ffmpeg
		// blocked on a read forever, so runWithRestart's exit-triggered backoff
		// never fires. This bounds a stalled connection to 15s before ffmpeg
		// exits and gets reconnected. The rtsp demuxer exposes its own
		// "-timeout" AVOption rather than the generic "-rw_timeout" used by
		// plain tcp/http protocols.
		"-timeout", "15000000",
		// Cheap RTSP cameras often emit irregular/absent timestamps and the
		// occasional corrupt packet; without genpts/discardcorrupt these can
		// stall the muxer instead of just skipping the bad frame.
		"-fflags", "+genpts+discardcorrupt",
		"-avoid_negative_ts", "make_zero",
		// Default probing can take up to several seconds per (re)connect;
		// these are plenty for a single H264(+AAC) IP camera stream and cut
		// that down to ~1s, which is most of the "slow to load" latency.
		"-probesize", "1000000",
		"-analyzeduration", "1000000",
		"-i", streamingURL,
		"-c:v", "copy",
		"-c:a", "aac",
		"-f", "mp4",
		// Fragmented, streaming-safe MP4: an empty moov up front (no seeking
		// needed to finalize it, since we're writing to a pipe) followed by
		// a moof+mdat fragment at each keyframe (copy mode can't manufacture
		// keyframes on demand, so fragment boundaries follow the camera's
		// own GOP structure, same as the hls_time-based segmentation
		// before). default_base_moof avoids absolute byte offsets that are
		// meaningless once this is streamed rather than written to a real
		// file.
		"-movflags", "frag_keyframe+empty_moov+default_base_moof",
		"pipe:1",
	}

	cmd := exec.CommandContext(ctx, m.ffmpegPath, args...)
	cmd.Stderr = log.Writer()

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("opening ffmpeg stdout: %w", err)
	}

	log.Printf("camera %s: starting ffmpeg", c.ID)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting ffmpeg: %w", err)
	}

	pipeErr := pipeToHub(h, stdout)
	waitErr := cmd.Wait()
	if waitErr != nil {
		return waitErr
	}
	return pipeErr
}

// Stop cancels every running ffmpeg process.
func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, cancel := range m.cancels {
		cancel()
	}
}
