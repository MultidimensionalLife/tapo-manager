package stream

import "sync"

// hub fans out a single fragmented-MP4 ffmpeg stream to any number of
// WebSocket viewers. New subscribers first get the cached initialization
// segment (the leading ftyp+moov boxes, produced once by ffmpeg's
// empty_moov muxer), then every fragment (moof+mdat pair) broadcast from
// that point on.
type hub struct {
	mu      sync.Mutex
	initSeg []byte
	subs    map[chan []byte]struct{}
}

func newHub() *hub {
	return &hub{subs: make(map[chan []byte]struct{})}
}

// subscribe registers a new viewer. The returned initSeg is nil if ffmpeg
// hasn't produced one yet (e.g. a viewer connects in the brief window before
// the stream starts); the subscriber's channel will still receive it as the
// first broadcast once available.
func (h *hub) subscribe() (ch chan []byte, initSeg []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	// Buffered so a slow-to-drain viewer doesn't stall the broadcaster on
	// every single chunk; broadcast still drops it if the buffer fills.
	ch = make(chan []byte, 64)
	h.subs[ch] = struct{}{}
	return ch, h.initSeg
}

func (h *hub) unsubscribe(ch chan []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.subs[ch]; ok {
		delete(h.subs, ch)
		close(ch)
	}
}

// broadcast fans a chunk out to every current subscriber, copying it since
// the caller reuses its read buffer. A subscriber whose buffer is full is
// dropped rather than allowed to block every other viewer.
func (h *hub) broadcast(chunk []byte) {
	b := append([]byte(nil), chunk...)
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- b:
		default:
			delete(h.subs, ch)
			close(ch)
		}
	}
}

func (h *hub) setInitSegment(b []byte) {
	h.mu.Lock()
	h.initSeg = append([]byte(nil), b...)
	h.mu.Unlock()
}

// reset disconnects every subscriber and clears the cached init segment.
// Called when ffmpeg exits: a restart produces a fresh init segment that
// existing MSE SourceBuffers (built around the old one) can't just append,
// so viewers need to reconnect and rebuild from scratch.
func (h *hub) reset() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		delete(h.subs, ch)
		close(ch)
	}
	h.initSeg = nil
}
