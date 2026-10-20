package catalog

import (
	"sync"
	"time"

	"synqsong/internal/room"
)

// A kept video is reused for this long.
const (
	videoCacheLife = 6 * time.Hour
	videoCacheSize = 5000
)

// videoCache keeps Data API answers by video id. They are public, so everyone shares one cache.
type videoCache struct {
	mu     sync.Mutex
	size   int
	videos map[room.VideoID]keptVideo
}

type keptVideo struct {
	video Video
	at    time.Time
}

func newVideoCache(size int) *videoCache {
	return &videoCache{size: size, videos: map[room.VideoID]keptVideo{}}
}

func (c *videoCache) get(id room.VideoID, now time.Time) (Video, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	kept, ok := c.videos[id]
	if !ok || now.Sub(kept.at) >= videoCacheLife {
		return Video{}, false
	}

	return kept.video, true
}

func (c *videoCache) put(v Video, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.videos[v.ID]; !ok && len(c.videos) >= c.size {
		c.dropOldest()
	}

	c.videos[v.ID] = keptVideo{video: v, at: now}
}

func (c *videoCache) dropOldest() {
	var (
		oldest room.VideoID
		at     time.Time
	)

	for id, kept := range c.videos {
		if oldest == "" || kept.at.Before(at) {
			oldest, at = id, kept.at
		}
	}

	delete(c.videos, oldest)
}
