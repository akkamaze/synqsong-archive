package catalog

import (
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"synqsong/internal/room"
)

func countingDataAPI(t *testing.T, calls *atomic.Int32, then http.HandlerFunc) http.HandlerFunc {
	return dataAPI(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		then(w, r)
	})
}

func TestALookedUpVideoIsNotAskedForAgainUntilItExpires(t *testing.T) {
	var calls atomic.Int32

	at := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	c := fakeYouTube(t, secretKey, countingDataAPI(t, &calls, answer(http.StatusOK, oneVideo)), notCalled(t, "oEmbed"))
	c.now = func() time.Time { return at }

	first, _ := lookUp(t, c)
	second, err := lookUp(t, c)

	if err != nil || second != first || calls.Load() != 1 {
		t.Fatalf("second look-up = %+v, %v after %d calls; want the first answer from 1 call", second, err, calls.Load())
	}

	at = at.Add(videoCacheLife)

	if _, err := lookUp(t, c); err != nil || calls.Load() != 2 {
		t.Fatalf("look-up after %s: %v after %d calls; want a fresh call", videoCacheLife, err, calls.Load())
	}
}

func TestOnlyFullDataAPIAnswersAreKept(t *testing.T) {
	var calls atomic.Int32

	failing := countingDataAPI(t, &calls, answer(http.StatusForbidden, `{"error":{"message":"quota exceeded"}}`))
	c := fakeYouTube(t, secretKey, failing, oEmbed(t, answer(http.StatusOK, `{"title":"Song"}`)))

	for range 2 {
		if _, err := lookUp(t, c); err != nil {
			t.Fatalf("look up: %v", err)
		}
	}

	if calls.Load() != 2 {
		t.Fatalf("Data API calls = %d; want 2, because an oEmbed answer has no length and must not be kept", calls.Load())
	}

	missing := fakeYouTube(t, secretKey, countingDataAPI(t, &calls, answer(http.StatusOK, `{"items":[]}`)), notCalled(t, "oEmbed"))
	calls.Store(0)

	for range 2 {
		_, _ = lookUp(t, missing)
	}

	if calls.Load() != 2 {
		t.Fatalf("Data API calls for a missing video = %d; want 2, so a video made public later is found", calls.Load())
	}
}

func TestTheCacheHoldsABoundedNumberOfVideos(t *testing.T) {
	cache := newVideoCache(3)
	at := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)

	for i, id := range []string{"aaaaaaaaaaa", "bbbbbbbbbbb", "ccccccccccc", "ddddddddddd"} {
		cache.put(Video{ID: room.VideoID(id)}, at.Add(time.Duration(i)*time.Second))
	}

	if len(cache.videos) != 3 {
		t.Fatalf("cache holds %d videos, want 3", len(cache.videos))
	}

	if _, ok := cache.get("aaaaaaaaaaa", at.Add(5*time.Second)); ok {
		t.Fatal("the oldest video survived a full cache")
	}

	if _, ok := cache.get("ddddddddddd", at.Add(5*time.Second)); !ok {
		t.Fatal("the newest video was dropped")
	}
}

func TestALiveVideoIsAskedForAgainSoItCanBeAddedOnceItEnds(t *testing.T) {
	var calls atomic.Int32

	live := strings.Replace(oneVideo, `"liveBroadcastContent":"none"`, `"liveBroadcastContent":"upcoming"`, 1)
	c := fakeYouTube(t, secretKey, countingDataAPI(t, &calls, answer(http.StatusOK, live)), notCalled(t, "oEmbed"))

	for range 2 {
		if v, err := lookUp(t, c); err != nil || !v.Live {
			t.Fatalf("look up = %+v, %v; want a live video", v, err)
		}
	}

	if calls.Load() != 2 {
		t.Fatalf("Data API calls = %d; want 2, because a premiere becomes addable once it airs", calls.Load())
	}
}
