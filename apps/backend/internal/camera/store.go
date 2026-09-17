package camera

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"unicode"
)

// ErrNotFound is returned by Remove when no camera has the given ID.
var ErrNotFound = errors.New("camera not found")

// ErrBaseCamera is returned by Remove when the camera is defined in the
// base (often read-only) config file rather than added through the API.
var ErrBaseCamera = errors.New("camera is defined in the base config and cannot be deleted via the API")

// Persister writes the set of runtime-added cameras to durable storage (a
// config file separate from the base one, which may be read-only). It's a
// function type so Store doesn't need to depend on the config package,
// which already depends on camera.
type Persister func(added []Camera) error

// Store holds the live, in-memory set of configured cameras and keeps
// runtime additions persisted as they change. Safe for concurrent use.
//
// base is the camera list loaded from the (possibly read-only) config file
// at boot and is never rewritten. added is everything created through the
// API afterwards; only added is ever passed to persist, so writes never
// touch the base config file.
type Store struct {
	mu      sync.RWMutex
	base    []Camera
	added   []Camera
	persist Persister
}

// NewStore creates a Store seeded with the cameras loaded at boot.
func NewStore(base []Camera, persist Persister) *Store {
	b := make([]Camera, len(base))
	copy(b, base)
	return &Store{base: b, persist: persist}
}

// SeedRuntime adds previously-persisted runtime cameras (read from the
// runtime config file at boot) without re-persisting them.
func (s *Store) SeedRuntime(added []Camera) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.added = append([]Camera(nil), added...)
}

// List returns a snapshot of the current cameras (base + runtime-added).
func (s *Store) List() []Camera {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Camera, 0, len(s.base)+len(s.added))
	out = append(out, s.base...)
	out = append(out, s.added...)
	return out
}

// Add assigns the camera a unique ID derived from its name (if it doesn't
// already have one), appends it, persists the updated runtime list, and
// returns the stored camera. Persistence failure rolls back the in-memory
// addition.
func (s *Store) Add(c Camera) (Camera, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if c.ID == "" {
		c.ID = s.uniqueIDLocked(c.Name)
	} else if s.hasIDLocked(c.ID) {
		return Camera{}, fmt.Errorf("camera id %q already exists", c.ID)
	}

	updatedAdded := append(append([]Camera(nil), s.added...), c)
	if s.persist != nil {
		if err := s.persist(updatedAdded); err != nil {
			return Camera{}, fmt.Errorf("saving camera config: %w", err)
		}
	}
	s.added = updatedAdded
	return c, nil
}

// Remove deletes a runtime-added camera and persists the updated list.
// Cameras from the base config can't be removed this way and yield
// ErrBaseCamera; an unknown ID yields ErrNotFound. Persistence failure rolls
// back the in-memory removal.
func (s *Store) Remove(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	idx := -1
	for i, c := range s.added {
		if c.ID == id {
			idx = i
			break
		}
	}
	if idx == -1 {
		for _, c := range s.base {
			if c.ID == id {
				return ErrBaseCamera
			}
		}
		return ErrNotFound
	}

	updatedAdded := append(append([]Camera(nil), s.added[:idx]...), s.added[idx+1:]...)
	if s.persist != nil {
		if err := s.persist(updatedAdded); err != nil {
			return fmt.Errorf("saving camera config: %w", err)
		}
	}
	s.added = updatedAdded
	return nil
}

func (s *Store) hasIDLocked(id string) bool {
	for _, existing := range s.base {
		if existing.ID == id {
			return true
		}
	}
	for _, existing := range s.added {
		if existing.ID == id {
			return true
		}
	}
	return false
}

// uniqueIDLocked slugifies name and, if that collides with an existing
// camera, appends "-2", "-3", ... until it's unique.
func (s *Store) uniqueIDLocked(name string) string {
	base := slugify(name)
	if base == "" {
		base = "camera"
	}
	id := base
	for n := 2; s.hasIDLocked(id); n++ {
		id = fmt.Sprintf("%s-%d", base, n)
	}
	return id
}

func slugify(name string) string {
	var b strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(name) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			prevDash = false
		case !prevDash && b.Len() > 0:
			b.WriteRune('-')
			prevDash = true
		}
	}
	return strings.TrimRight(b.String(), "-")
}
