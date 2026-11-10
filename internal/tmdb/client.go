// Package tmdb is a minimal client for the TMDB v3 API with rate limiting,
// request de-duplication and an on-disk response cache.
package tmdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"
)

const DefaultBaseURL = "https://api.themoviedb.org/3"

// ErrNotFound is returned for 404 responses.
var ErrNotFound = errors.New("tmdb: not found")

// Client talks to TMDB. It is safe for concurrent use.
type Client struct {
	BaseURL string
	Token   string // v4 read access token (preferred)
	APIKey  string // v3 api key
	HTTP    *http.Client
	Cache   *Cache // optional

	mu       sync.Mutex
	next     time.Time
	interval time.Duration

	flightMu sync.Mutex
	flight   map[string]*call
}

type call struct {
	done chan struct{}
	body []byte
	err  error
}

// New returns a client with sensible defaults (~40 requests/second).
func New(token, apiKey string, cache *Cache) *Client {
	return &Client{
		BaseURL:  DefaultBaseURL,
		Token:    token,
		APIKey:   apiKey,
		HTTP:     &http.Client{Timeout: 20 * time.Second},
		Cache:    cache,
		interval: 25 * time.Millisecond,
		flight:   map[string]*call{},
	}
}

func (c *Client) wait(ctx context.Context) error {
	c.mu.Lock()
	now := time.Now()
	if c.next.Before(now) {
		c.next = now
	}
	at := c.next
	c.next = c.next.Add(c.interval)
	c.mu.Unlock()
	if d := time.Until(at); d > 0 {
		t := time.NewTimer(d)
		defer t.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
		}
	}
	return nil
}

// get fetches path with params, returning the raw JSON body.
func (c *Client) get(ctx context.Context, path string, params url.Values) ([]byte, error) {
	key := path + "?" + params.Encode() // never contains credentials
	if c.Cache != nil {
		if b, ok := c.Cache.Get(key); ok {
			return b, nil
		}
	}

	c.flightMu.Lock()
	if c.flight == nil {
		c.flight = map[string]*call{}
	}
	if cl, ok := c.flight[key]; ok {
		c.flightMu.Unlock()
		select {
		case <-cl.done:
			return cl.body, cl.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	cl := &call{done: make(chan struct{})}
	c.flight[key] = cl
	c.flightMu.Unlock()

	cl.body, cl.err = c.fetch(ctx, path, params)
	if cl.err == nil && c.Cache != nil {
		c.Cache.Put(key, cl.body)
	}
	close(cl.done)
	c.flightMu.Lock()
	delete(c.flight, key)
	c.flightMu.Unlock()
	return cl.body, cl.err
}

func (c *Client) fetch(ctx context.Context, path string, params url.Values) ([]byte, error) {
	q := url.Values{}
	for k, v := range params {
		q[k] = v
	}
	if c.Token == "" && c.APIKey != "" {
		q.Set("api_key", c.APIKey)
	}
	u := c.BaseURL + path + "?" + q.Encode()
	var lastErr error
	for attempt := 0; attempt < 4; attempt++ {
		if err := c.wait(ctx); err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/json")
		if c.Token != "" {
			req.Header.Set("Authorization", "Bearer "+c.Token)
		}
		resp, err := c.HTTP.Do(req)
		if err != nil {
			lastErr = err
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			sleep(ctx, time.Duration(attempt+1)*500*time.Millisecond)
			continue
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}
		switch {
		case resp.StatusCode == http.StatusOK:
			return body, nil
		case resp.StatusCode == http.StatusNotFound:
			return nil, ErrNotFound
		case resp.StatusCode == http.StatusUnauthorized:
			return nil, fmt.Errorf("tmdb: unauthorized (check TMDB_TOKEN / TMDB_API_KEY)")
		case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
			lastErr = fmt.Errorf("tmdb: HTTP %d", resp.StatusCode)
			d := time.Duration(attempt+1) * time.Second
			if ra, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && ra > 0 {
				d = time.Duration(ra) * time.Second
			}
			sleep(ctx, d)
		default:
			return nil, fmt.Errorf("tmdb: HTTP %d: %s", resp.StatusCode, truncate(string(body), 200))
		}
	}
	return nil, lastErr
}

func sleep(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

// ---- API types

// Movie is a movie search result or details response.
type Movie struct {
	ID               int     `json:"id"`
	Title            string  `json:"title"`
	OriginalTitle    string  `json:"original_title"`
	OriginalLanguage string  `json:"original_language"`
	ReleaseDate      string  `json:"release_date"`
	Overview         string  `json:"overview"`
	Popularity       float64 `json:"popularity"`
}

// Show is a TV search result or details response.
type Show struct {
	ID               int      `json:"id"`
	Name             string   `json:"name"`
	OriginalName     string   `json:"original_name"`
	OriginalLanguage string   `json:"original_language"`
	FirstAirDate     string   `json:"first_air_date"`
	Overview         string   `json:"overview"`
	Popularity       float64  `json:"popularity"`
	OriginCountry    []string `json:"origin_country"`
	Seasons          []struct {
		SeasonNumber int `json:"season_number"`
		EpisodeCount int `json:"episode_count"`
	} `json:"seasons"`
}

// Episode is one episode inside a season response.
type Episode struct {
	EpisodeNumber int    `json:"episode_number"`
	SeasonNumber  int    `json:"season_number"`
	Name          string `json:"name"`
}

// Season is a season details response.
type Season struct {
	SeasonNumber int       `json:"season_number"`
	Episodes     []Episode `json:"episodes"`
}

// Year extracts the year from a TMDB date string ("2001-07-20").
func Year(date string) int {
	if len(date) < 4 {
		return 0
	}
	y, _ := strconv.Atoi(date[:4])
	return y
}

func (c *Client) getJSON(ctx context.Context, path string, params url.Values, v any) error {
	b, err := c.get(ctx, path, params)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

// SearchMovie searches movies. year may be 0.
func (c *Client) SearchMovie(ctx context.Context, query string, year int) ([]Movie, error) {
	p := url.Values{"query": {query}, "language": {"en-US"}, "include_adult": {"false"}}
	if year > 0 {
		p.Set("primary_release_year", strconv.Itoa(year))
	}
	var r struct {
		Results []Movie `json:"results"`
	}
	err := c.getJSON(ctx, "/search/movie", p, &r)
	return r.Results, err
}

// SearchTV searches TV shows. year (first air date) may be 0.
func (c *Client) SearchTV(ctx context.Context, query string, year int) ([]Show, error) {
	p := url.Values{"query": {query}, "language": {"en-US"}, "include_adult": {"false"}}
	if year > 0 {
		p.Set("first_air_date_year", strconv.Itoa(year))
	}
	var r struct {
		Results []Show `json:"results"`
	}
	err := c.getJSON(ctx, "/search/tv", p, &r)
	return r.Results, err
}

// GetMovie fetches a movie by ID (English title + original title).
func (c *Client) GetMovie(ctx context.Context, id int) (Movie, error) {
	var m Movie
	err := c.getJSON(ctx, "/movie/"+strconv.Itoa(id), url.Values{"language": {"en-US"}}, &m)
	return m, err
}

// GetTV fetches a show by ID (English name + original name).
func (c *Client) GetTV(ctx context.Context, id int) (Show, error) {
	var s Show
	err := c.getJSON(ctx, "/tv/"+strconv.Itoa(id), url.Values{"language": {"en-US"}}, &s)
	return s, err
}

// GetSeason fetches a season's episode list in the given language.
func (c *Client) GetSeason(ctx context.Context, tvID, season int, lang string) (Season, error) {
	var s Season
	p := url.Values{}
	if lang != "" {
		p.Set("language", lang)
	}
	err := c.getJSON(ctx, fmt.Sprintf("/tv/%d/season/%d", tvID, season), p, &s)
	return s, err
}
