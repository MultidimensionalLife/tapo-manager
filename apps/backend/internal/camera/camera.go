// Package camera defines the Tapo camera model shared across the backend.
package camera

import "net/url"

// Camera describes a single Tapo (or other RTSP-capable) camera.
type Camera struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	RTSPURL  string `json:"rtspUrl"`
	Username string `json:"username,omitempty"`
	Password string `json:"-"`
}

// StreamingURL returns the RTSP URL ffmpeg should actually connect with.
// RTSPURL may already carry embedded credentials (rtsp://user:pass@host/...,
// as used by the base cameras.json convention); if Username is set
// separately instead, it's added here rather than left unused, overriding
// any credentials already embedded in RTSPURL.
func (c Camera) StreamingURL() (string, error) {
	if c.Username == "" {
		return c.RTSPURL, nil
	}
	u, err := url.Parse(c.RTSPURL)
	if err != nil {
		return "", err
	}
	u.User = url.UserPassword(c.Username, c.Password)
	return u.String(), nil
}
