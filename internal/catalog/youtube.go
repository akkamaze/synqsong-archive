package catalog

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"synqsong/internal/room"
)

var ErrVideoNotFound = errors.New("that video was not found on YouTube; it may be private or removed")

// Client looks videos up on YouTube. With an API key it uses the Data API, which gives the
// length; without a key, or when the Data API fails, it uses oEmbed, and the length stays unknown.
type Client struct {
	key           string
	http          *http.Client
	apiBase       string
	oembedBase    string
	reportFailure func(error)
}

type Option func(*Client)

// WithBaseURLs points the client at test servers.
func WithBaseURLs(api, oembed string) Option {
	return func(c *Client) { c.apiBase, c.oembedBase = api, oembed }
}

// WhenDataAPIFails is told why the Data API failed before a look-up falls back to oEmbed,
// so a spent quota shows up in the server's logs. The error never holds the key.
func WhenDataAPIFails(report func(error)) Option {
	return func(c *Client) { c.reportFailure = report }
}

func NewClient(key string, opts ...Option) *Client {
	c := &Client{
		key:           key,
		http:          &http.Client{Timeout: 10 * time.Second},
		apiBase:       "https://www.googleapis.com/youtube/v3",
		oembedBase:    "https://www.youtube.com",
		reportFailure: func(error) {},
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

func (c *Client) LookUp(ctx context.Context, id room.VideoID) (Video, error) {
	if c.key == "" {
		return c.lookUpOEmbed(ctx, id)
	}

	v, err := c.lookUpAPI(ctx, id)
	if err == nil || errors.Is(err, ErrVideoNotFound) || ctx.Err() != nil {
		return v, err
	}

	c.reportFailure(err)

	return c.lookUpOEmbed(ctx, id)
}

type apiVideos struct {
	Items []struct {
		ID      string `json:"id"`
		Snippet struct {
			Title       string  `json:"title"`
			Channel     string  `json:"channelTitle"`
			LiveContent *string `json:"liveBroadcastContent"`
			Thumbnails  struct {
				High struct {
					URL string `json:"url"`
				} `json:"high"`
			} `json:"thumbnails"`
		} `json:"snippet"`
		ContentDetails struct {
			Duration string `json:"duration"`
		} `json:"contentDetails"`
		Status struct {
			Embeddable *bool `json:"embeddable"`
		} `json:"status"`
	} `json:"items"`
}

func (c *Client) lookUpAPI(ctx context.Context, id room.VideoID) (Video, error) {
	q := url.Values{"part": {"snippet,contentDetails,status"}, "id": {string(id)}, "key": {c.key}}

	var body apiVideos
	if status, err := c.getJSON(ctx, c.apiBase+"/videos?"+q.Encode(), &body); err != nil {
		return Video{}, err
	} else if status != http.StatusOK {
		return Video{}, fmt.Errorf("YouTube Data API answered %d for video %s", status, id)
	}

	if len(body.Items) == 0 {
		return Video{}, ErrVideoNotFound
	}

	item := body.Items[0]
	length, _ := parseISODuration(item.ContentDetails.Duration)
	// Only a missing or "none" broadcast state is a normal video, and only an explicit false blocks embedding.
	live := item.Snippet.LiveContent != nil && *item.Snippet.LiveContent != "none"
	embeddable := item.Status.Embeddable == nil || *item.Status.Embeddable

	return Video{
		ID:         id,
		Title:      cleanText(item.Snippet.Title),
		Channel:    cleanText(item.Snippet.Channel),
		Thumbnail:  item.Snippet.Thumbnails.High.URL,
		Duration:   length,
		Live:       live,
		Embeddable: embeddable,
	}, nil
}

type oembed struct {
	Title     string `json:"title"`
	Author    string `json:"author_name"`
	Thumbnail string `json:"thumbnail_url"`
}

func (c *Client) lookUpOEmbed(ctx context.Context, id room.VideoID) (Video, error) {
	q := url.Values{"url": {"https://www.youtube.com/watch?v=" + string(id)}, "format": {"json"}}

	var body oembed

	status, err := c.getJSON(ctx, c.oembedBase+"/oembed?"+q.Encode(), &body)
	if err != nil {
		return Video{}, err
	}

	switch status {
	case http.StatusOK:
		return Video{ID: id, Title: cmp.Or(body.Title, "Untitled"), Channel: body.Author, Thumbnail: cmp.Or(body.Thumbnail, defaultThumbnail(id)), Embeddable: true}, nil
	case http.StatusUnauthorized, http.StatusForbidden:
		return Video{ID: id, Embeddable: false}, nil
	case http.StatusNotFound, http.StatusBadRequest:
		return Video{}, ErrVideoNotFound
	}

	return Video{}, fmt.Errorf("YouTube oEmbed answered %d for video %s", status, id)
}

func defaultThumbnail(id room.VideoID) string {
	return "https://i.ytimg.com/vi/" + string(id) + "/hqdefault.jpg"
}

// getJSON never returns the request URL in an error, because the Data API URL holds the key.
func (c *Client) getJSON(ctx context.Context, rawURL string, into any) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return 0, errors.New("build YouTube request")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err
		}

		return 0, fmt.Errorf("reach YouTube: %w", err)
	}
	defer drainAndClose(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return resp.StatusCode, nil
	}

	if err := json.NewDecoder(io.LimitReader(resp.Body, maxAnswerBytes)).Decode(into); err != nil {
		return 0, errors.New("read YouTube's answer: it was not the JSON expected")
	}

	return resp.StatusCode, nil
}

const maxAnswerBytes = 1 << 20

// drainAndClose reads what is left, up to the limit, so the connection can carry the next request.
func drainAndClose(body io.ReadCloser) {
	_, _ = io.Copy(io.Discard, io.LimitReader(body, maxAnswerBytes))
	_ = body.Close()
}
