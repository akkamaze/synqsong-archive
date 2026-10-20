package catalog

import (
	"errors"
	"testing"
	"time"

	"synqsong/internal/room"
)

func TestFindVideoIDReadsYouTubeLinks(t *testing.T) {
	cases := map[string]room.VideoID{
		"dQw4w9WgXcQ":     "dQw4w9WgXcQ",
		"  dQw4w9WgXcQ  ": "dQw4w9WgXcQ",
		"https://www.youtube.com/watch?v=dQw4w9WgXcQ":               "dQw4w9WgXcQ",
		"https://youtube.com/watch?v=dQw4w9WgXcQ&t=42":              "dQw4w9WgXcQ",
		"https://m.youtube.com/watch?v=dQw4w9WgXcQ":                 "dQw4w9WgXcQ",
		"https://music.youtube.com/watch?v=dQw4w9WgXcQ&si=x":        "dQw4w9WgXcQ",
		"https://youtu.be/dQw4w9WgXcQ?si=abc":                       "dQw4w9WgXcQ",
		"https://www.youtube.com/embed/dQw4w9WgXcQ":                 "dQw4w9WgXcQ",
		"https://www.youtube.com/shorts/dQw4w9WgXcQ":                "dQw4w9WgXcQ",
		"https://www.youtube.com/live/dQw4w9WgXcQ":                  "dQw4w9WgXcQ",
		"ดูนี่ https://youtu.be/dQw4w9WgXcQ เพราะมาก":               "dQw4w9WgXcQ",
		"https://www.youtube.com/watch?feature=share&v=dQw4w9WgXcQ": "dQw4w9WgXcQ",
		"https://www.youtube.com/watch?app=desktop&v=dQw4w9WgXcQ":   "dQw4w9WgXcQ",
		"https://WWW.YouTube.COM/watch?v=dQw4w9WgXcQ":               "dQw4w9WgXcQ",
		"https://www.youtube-nocookie.com/embed/dQw4w9WgXcQ":        "dQw4w9WgXcQ",
		"https://youtube-nocookie.com/embed/dQw4w9WgXcQ?rel=0":      "dQw4w9WgXcQ",
		"https://www.youtube.com/v/dQw4w9WgXcQ":                     "dQw4w9WgXcQ",
		"https://www.youtube.com/shorts/dQw4w9WgXcQ/":               "dQw4w9WgXcQ",
	}

	for raw, want := range cases {
		if got, err := FindVideoID(raw); err != nil || got != want {
			t.Errorf("FindVideoID(%q) = %q, %v; want %q", raw, got, err, want)
		}
	}

	for _, bad := range []string{"", "hello", "https://vimeo.com/123456", "https://www.youtube.com/watch?v=short", "dQw4w9WgXc",
		"https://www.youtube.com/watch?feature=share&v=short", "https://notyoutube.com/v/dQw4w9WgXcQ", "https://www.youtube.com/channel/dQw4w9WgXcQ",
	} {
		if got, err := FindVideoID(bad); err == nil {
			t.Errorf("FindVideoID(%q) = %q, want an error", bad, got)
		}
	}
}

func TestParseISODuration(t *testing.T) {
	cases := map[string]time.Duration{
		"PT4M13S":  4*time.Minute + 13*time.Second,
		"PT1H":     time.Hour,
		"PT45S":    45 * time.Second,
		"PT1H2M3S": time.Hour + 2*time.Minute + 3*time.Second,
		"P1D":      24 * time.Hour,
		"P1DT2H":   26 * time.Hour,
		"P0DT3M":   3 * time.Minute,
	}

	for iso, want := range cases {
		if got, ok := parseISODuration(iso); !ok || got != want {
			t.Errorf("parseISODuration(%q) = %v, %v; want %v", iso, got, ok, want)
		}
	}

	for _, unknown := range []string{"", "P0D", "4M13S", "P", "PT", "P1DT", "PTXM", "P1H"} {
		if got, ok := parseISODuration(unknown); ok {
			t.Errorf("parseISODuration(%q) = %v, want unknown", unknown, got)
		}
	}
}

func TestDurationsBeyondCountingAreTooLong(t *testing.T) {
	for _, iso := range []string{"P1DT2H", "P2D", "PT25H", "PT9999999999H", "P99999999999999999999D", "PT99999999999999999999H", "PT99999999999999999999M"} {
		got, ok := parseISODuration(iso)

		video := Video{ID: "dQw4w9WgXcQ", Embeddable: true, Duration: got}
		if !ok || got <= 0 || !errors.Is(video.Queueable(), ErrTooLong) {
			t.Errorf("parseISODuration(%q) = %v, %v; want a positive length over 10 minutes", iso, got, ok)
		}
	}
}

func TestQueueableRules(t *testing.T) {
	ok := Video{ID: "dQw4w9WgXcQ", Title: "Song", Embeddable: true, Duration: 4 * time.Minute}

	if err := ok.Queueable(); err != nil {
		t.Fatalf("a 4-minute embeddable video: %v, want queueable", err)
	}

	unknownLength := ok
	unknownLength.Duration = 0
	if err := unknownLength.Queueable(); err != nil {
		t.Fatalf("unknown length: %v, want queueable", err)
	}

	exactlyTen := ok
	exactlyTen.Duration = 10 * time.Minute
	if err := exactlyTen.Queueable(); err != nil {
		t.Fatalf("exactly 10 minutes: %v, want queueable", err)
	}

	refused := map[string]struct {
		video Video
		want  error
	}{
		"too long":       {Video{ID: ok.ID, Embeddable: true, Duration: 10*time.Minute + time.Second}, ErrTooLong},
		"live":           {Video{ID: ok.ID, Embeddable: true, Live: true}, ErrLive},
		"not embeddable": {Video{ID: ok.ID, Duration: time.Minute}, ErrNotEmbeddable},
	}

	for name, c := range refused {
		if err := c.video.Queueable(); !errors.Is(err, c.want) {
			t.Errorf("%s: %v, want %v", name, err, c.want)
		}
	}
}
