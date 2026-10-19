// Package catalog turns YouTube links into tracks a queue can hold.
package catalog

import (
	"errors"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"synqsong/internal/room"
)

// Links read: watch, embed, shorts and live pages, and youtu.be.
var (
	bareID = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)
	linkID = regexp.MustCompile(`(?:youtube\.com/(?:watch\?v=|embed/|shorts/|live/)|youtu\.be/)([A-Za-z0-9_-]{11})`)
)

var errNoVideo = errors.New("YouTube link: paste a youtube.com or youtu.be link, or an 11-character video id")

// FindVideoID takes the video id from a bare id, a youtube.com link, or a link anywhere in the text.
func FindVideoID(raw string) (room.VideoID, error) {
	s := strings.TrimSpace(raw)

	if bareID.MatchString(s) {
		return room.ParseVideoID(s)
	}

	if id, ok := idFromYouTubeURL(s); ok {
		return id, nil
	}

	m := linkID.FindStringSubmatch(s)
	if m == nil {
		return "", errNoVideo
	}

	return room.ParseVideoID(m[1])
}

var idPathPrefixes = []string{"embed", "shorts", "live", "v"}

// idFromYouTubeURL reads links the regex misses, like "watch?feature=share&v=ID" or "/v/ID".
func idFromYouTubeURL(s string) (room.VideoID, bool) {
	u, err := url.Parse(s)
	if err != nil || !isYouTubeHost(u.Hostname()) {
		return "", false
	}

	candidate := u.Query().Get("v")

	if segments := strings.Split(strings.Trim(u.Path, "/"), "/"); len(segments) >= 2 && slices.Contains(idPathPrefixes, segments[0]) {
		candidate = segments[1]
	}

	id, err := room.ParseVideoID(candidate)

	return id, err == nil
}

func isYouTubeHost(host string) bool {
	host = strings.ToLower(host)

	for _, domain := range []string{"youtube.com", "youtube-nocookie.com"} {
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return true
		}
	}

	return false
}

// Videos longer than maxLength cannot be queued.
const maxLength = 10 * time.Minute

var (
	ErrTooLong       = errors.New("videos over 10 minutes cannot be added")
	ErrLive          = errors.New("live streams cannot be added")
	ErrNotEmbeddable = errors.New("this video's channel does not allow playing it here, try another link")
)

// Video is what YouTube says about one video. A zero Duration means the length is unknown.
type Video struct {
	ID         room.VideoID
	Title      string
	Channel    string
	Thumbnail  string
	Duration   time.Duration
	Live       bool
	Embeddable bool
}

// Queueable applies the rules for adding a video to a queue.
func (v Video) Queueable() error {
	switch {
	case !v.Embeddable:
		return ErrNotEmbeddable
	case v.Live:
		return ErrLive
	case v.Duration > maxLength:
		return ErrTooLong
	}

	return nil
}

func (v Video) Track() room.Track {
	return room.Track{Video: v.ID, Title: v.Title, Channel: v.Channel, Thumbnail: v.Thumbnail, Duration: v.Duration}
}

var isoDuration = regexp.MustCompile(`^P(?:(\d+)D)?(?:T(?:(\d+)H)?(?:(\d+)M)?(?:(\d+)S)?)?$`)

// A part longer than this is counted as this, which keeps a huge count over the cap and never negative.
const longestCounted = 24 * time.Hour

// parseISODuration reads YouTube's "PT4M13S" and "P1DT2H". "P0D", which live streams report, is unknown.
func parseISODuration(iso string) (time.Duration, bool) {
	m := isoDuration.FindStringSubmatch(iso)
	if m == nil || iso == "P" || iso == "P0D" || strings.HasSuffix(iso, "T") {
		return 0, false
	}

	var total time.Duration

	for i, unit := range []time.Duration{24 * time.Hour, time.Hour, time.Minute, time.Second} {
		if m[i+1] == "" {
			continue
		}

		n, err := strconv.ParseInt(m[i+1], 10, 64)
		if err != nil || n > int64(longestCounted/unit) {
			return longestCounted, true
		}

		total += time.Duration(n) * unit
	}

	return total, true
}
