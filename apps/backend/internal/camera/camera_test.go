package camera

import "testing"

func TestCameraStreamingURL(t *testing.T) {
	cases := []struct {
		name string
		cam  Camera
		want string
	}{
		{
			name: "credentials embedded in RTSPURL, no separate username",
			cam:  Camera{RTSPURL: "rtsp://camUser:camPass@192.168.1.50:554/stream1"},
			want: "rtsp://camUser:camPass@192.168.1.50:554/stream1",
		}
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.cam.StreamingURL()
			if err != nil {
				t.Fatalf("StreamingURL() error = %v", err)
			}
			if got != tc.want {
				t.Errorf("StreamingURL() = %q, want %q", got, tc.want)
			}
		})
	}
}
