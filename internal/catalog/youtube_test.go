package catalog

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const (
	secretKey = "AIzaSECRET-test-key"
	videoID   = "dQw4w9WgXcQ"
)

func answer(status int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = fmt.Fprint(w, body)
	}
}

func dataAPI(t *testing.T, then http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		parts := strings.Split(q.Get("part"), ",")

		if r.URL.Path != "/videos" || q.Get("id") != videoID || q.Get("key") != secretKey {
			t.Errorf("Data API asked for path %q id %q with key %q", r.URL.Path, q.Get("id"), q.Get("key"))
		}

		for _, part := range []string{"snippet", "contentDetails", "status"} {
			if !slices.Contains(parts, part) {
				t.Errorf("Data API part %q is missing %q", q.Get("part"), part)
			}
		}

		then(w, r)
	}
}

func oEmbed(t *testing.T, then http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()

		if r.URL.Path != "/oembed" || q.Get("url") != "https://www.youtube.com/watch?v="+videoID || q.Get("format") != "json" {
			t.Errorf("oEmbed asked for path %q url %q format %q", r.URL.Path, q.Get("url"), q.Get("format"))
		}

		if q.Has("key") {
			t.Error("oEmbed was sent the API key")
		}

		then(w, r)
	}
}

func notCalled(t *testing.T, name string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		t.Errorf("%s was called", name)
		w.WriteHeader(http.StatusTeapot)
	}
}

// fakeYouTube serves the Data API and oEmbed from separate servers, so a mixed-up base URL fails.
func fakeYouTube(t *testing.T, key string, api, embed http.HandlerFunc) *Client {
	apiServer := httptest.NewServer(api)
	t.Cleanup(apiServer.Close)

	embedServer := httptest.NewServer(embed)
	t.Cleanup(embedServer.Close)

	return NewClient(key, WithBaseURLs(apiServer.URL, embedServer.URL))
}

func lookUp(t *testing.T, c *Client) (Video, error) {
	t.Helper()

	return c.LookUp(context.Background(), videoID)
}

const oneVideo = `{"items":[{"id":"dQw4w9WgXcQ",
 "snippet":{"title":"Don&#39;t Stop","channelTitle":"Band &amp; Co","liveBroadcastContent":"none",
  "thumbnails":{"high":{"url":"https://i.ytimg.com/vi/dQw4w9WgXcQ/hqdefault.jpg"}}},
 "contentDetails":{"duration":"PT3M33S"},
 "status":{"embeddable":true}}]}`

func TestLookUpReadsTheDataAPI(t *testing.T) {
	c := fakeYouTube(t, secretKey, dataAPI(t, answer(http.StatusOK, oneVideo)), notCalled(t, "oEmbed"))

	v, err := lookUp(t, c)
	if err != nil {
		t.Fatalf("look up: %v", err)
	}

	want := Video{ID: videoID, Title: "Don't Stop", Channel: "Band & Co", Thumbnail: "https://i.ytimg.com/vi/dQw4w9WgXcQ/hqdefault.jpg", Duration: 3*time.Minute + 33*time.Second, Embeddable: true}
	if v != want {
		t.Fatalf("video = %+v, want %+v", v, want)
	}
}

func TestAnyBroadcastStateButNoneIsLive(t *testing.T) {
	cases := map[string]bool{
		`"liveBroadcastContent":"live",`:       true,
		`"liveBroadcastContent":"upcoming",`:   true,
		`"liveBroadcastContent":"premiering",`: true,
		`"liveBroadcastContent":"none",`:       false,
		`"liveBroadcastContent":"",`:           true,
		``:                                     false,
	}

	for field, wantLive := range cases {
		body := strings.Replace(oneVideo, `"liveBroadcastContent":"none",`, field, 1)
		c := fakeYouTube(t, secretKey, dataAPI(t, answer(http.StatusOK, body)), notCalled(t, "oEmbed"))

		if v, err := lookUp(t, c); err != nil || v.Live != wantLive {
			t.Errorf("snippet with %q: %+v, %v; want Live %v", field, v, err, wantLive)
		}
	}
}

func TestDataAPIEmbeddingOffIsNotQueueable(t *testing.T) {
	cases := map[string]error{
		`"status":{"embeddable":false}`: ErrNotEmbeddable,
		`"status":{}`:                   nil,
	}

	for status, want := range cases {
		body := strings.Replace(oneVideo, `"status":{"embeddable":true}`, status, 1)
		c := fakeYouTube(t, secretKey, dataAPI(t, answer(http.StatusOK, body)), notCalled(t, "oEmbed"))

		v, err := lookUp(t, c)
		if err != nil {
			t.Fatalf("look up: %v", err)
		}

		if err := v.Queueable(); !errors.Is(err, want) {
			t.Errorf("video with %s: Queueable = %v, want %v", status, err, want)
		}
	}
}

func TestACanceledLookUpStopsWithoutFallingBack(t *testing.T) {
	apiServer := httptest.NewServer(notCalled(t, "Data API"))
	t.Cleanup(apiServer.Close)

	embedServer := httptest.NewServer(notCalled(t, "oEmbed"))
	t.Cleanup(embedServer.Close)

	reported := 0
	c := NewClient(secretKey, WithBaseURLs(apiServer.URL, embedServer.URL), WhenDataAPIFails(func(error) { reported++ }))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if v, err := c.LookUp(ctx, videoID); !errors.Is(err, context.Canceled) || reported != 0 {
		t.Errorf("canceled look up = %+v, %v with %d failures reported; want context.Canceled and none", v, err, reported)
	}
}

func TestAMissingVideoSaysSo(t *testing.T) {
	c := fakeYouTube(t, secretKey, dataAPI(t, answer(http.StatusOK, `{"items":[]}`)), notCalled(t, "oEmbed"))

	if _, err := lookUp(t, c); !errors.Is(err, ErrVideoNotFound) {
		t.Fatalf("look up of a removed video: %v, want ErrVideoNotFound", err)
	}
}

func TestWithoutAKeyOEmbedGivesTitleButNoLength(t *testing.T) {
	body := `{"title":"Song &amp; Dance","author_name":"Tom&regina &#39;s","thumbnail_url":"https://i.ytimg.com/vi/dQw4w9WgXcQ/mqdefault.jpg"}`
	c := fakeYouTube(t, "", notCalled(t, "Data API"), oEmbed(t, answer(http.StatusOK, body)))

	v, err := lookUp(t, c)

	want := Video{ID: videoID, Title: "Song &amp; Dance", Channel: "Tom&regina &#39;s", Thumbnail: "https://i.ytimg.com/vi/dQw4w9WgXcQ/mqdefault.jpg", Embeddable: true}
	if err != nil || v != want {
		t.Fatalf("oEmbed video = %+v, %v; want %+v: text as written, unknown length", v, err, want)
	}
}

func TestOEmbedFillsInAMissingTitleAndThumbnail(t *testing.T) {
	c := fakeYouTube(t, "", notCalled(t, "Data API"), oEmbed(t, answer(http.StatusOK, `{"author_name":"Ch"}`)))

	v, err := lookUp(t, c)

	want := Video{ID: videoID, Title: "Untitled", Channel: "Ch", Thumbnail: "https://i.ytimg.com/vi/dQw4w9WgXcQ/hqdefault.jpg", Embeddable: true}
	if err != nil || v != want {
		t.Fatalf("oEmbed without title or thumbnail = %+v, %v; want %+v", v, err, want)
	}
}

func TestOEmbedAnswers(t *testing.T) {
	const found = `{"title":"Song","author_name":"Ch","thumbnail_url":"https://i.ytimg.com/vi/dQw4w9WgXcQ/hqdefault.jpg"}`

	cases := []struct {
		status         int
		wantEmbeddable bool
		wantErr        error
	}{
		{http.StatusOK, true, nil},
		{http.StatusUnauthorized, false, nil},
		{http.StatusForbidden, false, nil},
		{http.StatusNotFound, false, ErrVideoNotFound},
		{http.StatusBadRequest, false, ErrVideoNotFound},
	}

	for _, tc := range cases {
		c := fakeYouTube(t, "", notCalled(t, "Data API"), oEmbed(t, answer(tc.status, found)))

		v, err := lookUp(t, c)
		if !errors.Is(err, tc.wantErr) || (err == nil && (v.ID != videoID || v.Embeddable != tc.wantEmbeddable)) {
			t.Errorf("oEmbed %d = %+v, %v; want embeddable %v, error %v", tc.status, v, err, tc.wantEmbeddable, tc.wantErr)
		}
	}

	c := fakeYouTube(t, "", notCalled(t, "Data API"), oEmbed(t, answer(http.StatusInternalServerError, "")))
	if _, err := lookUp(t, c); err == nil || errors.Is(err, ErrVideoNotFound) {
		t.Errorf("oEmbed 500: %v, want an error that is not ErrVideoNotFound", err)
	}
}

func TestAFailingDataAPIFallsBackToOEmbed(t *testing.T) {
	const found = `{"title":"Song","author_name":"Ch","thumbnail_url":"https://i.ytimg.com/vi/dQw4w9WgXcQ/hqdefault.jpg"}`
	want := Video{ID: videoID, Title: "Song", Channel: "Ch", Thumbnail: "https://i.ytimg.com/vi/dQw4w9WgXcQ/hqdefault.jpg", Embeddable: true}

	failures := map[string]http.HandlerFunc{
		"403":      answer(http.StatusForbidden, `{"error":{"message":"quota exceeded"}}`),
		"500":      answer(http.StatusInternalServerError, ""),
		"bad JSON": answer(http.StatusOK, "<html>"),
	}

	for name, failure := range failures {
		c := fakeYouTube(t, secretKey, dataAPI(t, failure), oEmbed(t, answer(http.StatusOK, found)))

		if v, err := lookUp(t, c); err != nil || v != want {
			t.Errorf("Data API %s: %+v, %v; want the oEmbed video %+v", name, v, err, want)
		}
	}

	closed := httptest.NewServer(notCalled(t, "a closed server"))
	closed.Close()

	embed := httptest.NewServer(oEmbed(t, answer(http.StatusOK, found)))
	t.Cleanup(embed.Close)

	unreachable := NewClient(secretKey, WithBaseURLs(closed.URL, embed.URL))
	if v, err := lookUp(t, unreachable); err != nil || v != want {
		t.Errorf("Data API unreachable: %+v, %v; want the oEmbed video %+v", v, err, want)
	}

	refused := fakeYouTube(t, secretKey, dataAPI(t, failures["403"]), oEmbed(t, answer(http.StatusUnauthorized, "")))
	if v, err := lookUp(t, refused); err != nil || v.Embeddable {
		t.Errorf("Data API 403, oEmbed 401: %+v, %v; want not embeddable", v, err)
	}
}

func TestAnAnswerOverOneMiBIsRefused(t *testing.T) {
	huge := `{"title":"` + strings.Repeat("a", 1<<20) + `"}`
	c := fakeYouTube(t, "", notCalled(t, "Data API"), oEmbed(t, answer(http.StatusOK, huge)))

	if v, err := lookUp(t, c); err == nil {
		t.Fatalf("an oEmbed answer over 1 MiB gave %d title bytes, want an error", len(v.Title))
	}
}

func TestRefusedAnswersLeaveTheConnectionReusable(t *testing.T) {
	var connections atomic.Int32

	server := httptest.NewUnstartedServer(oEmbed(t, answer(http.StatusUnauthorized, strings.Repeat("x", 64<<10))))
	server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			connections.Add(1)
		}
	}
	server.Start()
	t.Cleanup(server.Close)

	c := NewClient("", WithBaseURLs(server.URL, server.URL))

	for range 3 {
		if _, err := lookUp(t, c); err != nil {
			t.Fatalf("look up: %v", err)
		}
	}

	if n := connections.Load(); n != 1 {
		t.Fatalf("3 refused look-ups opened %d connections, want 1", n)
	}
}

func TestADataAPIFailureIsReportedWithoutTheKey(t *testing.T) {
	closed := httptest.NewServer(notCalled(t, "a closed server"))
	closed.Close()

	embed := httptest.NewServer(oEmbed(t, answer(http.StatusOK, `{"title":"Song"}`)))
	t.Cleanup(embed.Close)

	refusing := httptest.NewServer(dataAPI(t, answer(http.StatusForbidden, `{"error":{"message":"quota exceeded"}}`)))
	t.Cleanup(refusing.Close)

	for name, api := range map[string]string{"refused": refusing.URL, "unreachable": closed.URL} {
		var reported []error
		c := NewClient(secretKey, WithBaseURLs(api, embed.URL), WhenDataAPIFails(func(err error) { reported = append(reported, err) }))

		if _, err := lookUp(t, c); err != nil {
			t.Fatalf("%s: look up: %v, want the oEmbed video", name, err)
		}

		if len(reported) != 1 {
			t.Fatalf("%s: %d failures reported, want 1", name, len(reported))
		}

		if msg := reported[0].Error(); strings.Contains(msg, secretKey) || strings.Contains(msg, "key=") {
			t.Errorf("%s: reported failure leaks the API key: %v", name, msg)
		}
	}
}

func TestErrorsNeverCarryTheKey(t *testing.T) {
	refused := fakeYouTube(t, secretKey, answer(http.StatusForbidden, `{"error":{"message":"quota exceeded"}}`), answer(http.StatusInternalServerError, ""))

	closed := httptest.NewServer(notCalled(t, "a closed server"))
	closed.Close()
	unreachable := NewClient(secretKey, WithBaseURLs(closed.URL, closed.URL))

	for name, c := range map[string]*Client{"refused": refused, "unreachable": unreachable} {
		_, err := lookUp(t, c)
		if err == nil {
			t.Errorf("%s: no error", name)

			continue
		}

		if strings.Contains(err.Error(), secretKey) || strings.Contains(err.Error(), "key=") {
			t.Errorf("%s error leaks the API key: %v", name, err)
		}
	}
}
