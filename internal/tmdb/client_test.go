package tmdb

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestClient(t *testing.T) {
	var hits, fails atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(401)
			return
		}
		switch r.URL.Path {
		case "/search/movie":
			if r.URL.Query().Get("primary_release_year") != "1999" {
				t.Errorf("year param missing: %v", r.URL.Query())
			}
			if fails.Add(1) == 1 {
				w.Header().Set("Retry-After", "0")
				w.WriteHeader(429)
				return
			}
			w.Write([]byte(`{"results":[{"id":603,"title":"The Matrix","original_title":"The Matrix","original_language":"en","release_date":"1999-03-30","popularity":80}]}`))
		case "/tv/1/season/1":
			if r.URL.Query().Get("language") != "de" {
				t.Errorf("language param: %v", r.URL.Query())
			}
			w.Write([]byte(`{"season_number":1,"episodes":[{"episode_number":1,"season_number":1,"name":"Geheimnisse"}]}`))
		default:
			w.WriteHeader(404)
			w.Write([]byte(`{"status_code":34}`))
		}
	}))
	defer srv.Close()

	c := New("tok", "", NewCache(t.TempDir(), time.Hour))
	c.BaseURL = srv.URL
	c.interval = 0
	ctx := context.Background()

	ms, err := c.SearchMovie(ctx, "The Matrix", 1999)
	if err != nil || len(ms) != 1 || ms[0].ID != 603 || Year(ms[0].ReleaseDate) != 1999 {
		t.Fatalf("search: %v %+v", err, ms)
	}
	before := hits.Load()
	if _, err := c.SearchMovie(ctx, "The Matrix", 1999); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != before {
		t.Error("second identical request should be served from cache")
	}
	s, err := c.GetSeason(ctx, 1, 1, "de")
	if err != nil || s.Episodes[0].Name != "Geheimnisse" {
		t.Fatalf("season: %v %+v", err, s)
	}
	if _, err := c.GetMovie(ctx, 42); !errors.Is(err, ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

func TestAPIKeyParam(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("api_key") != "k" || r.Header.Get("Authorization") != "" {
			w.WriteHeader(401)
			return
		}
		w.Write([]byte(`{"results":[]}`))
	}))
	defer srv.Close()
	c := New("", "k", nil)
	c.BaseURL = srv.URL
	if _, err := c.SearchTV(context.Background(), "x", 0); err != nil {
		t.Fatal(err)
	}
}
