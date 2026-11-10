package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"renamer/internal/match"
	"renamer/internal/media"
	"renamer/internal/scan"
	"renamer/internal/tmdb"
)

type fakeAPI struct{}

func (fakeAPI) SearchMovie(_ context.Context, q string, year int) ([]tmdb.Movie, error) {
	all := []tmdb.Movie{
		{ID: 27205, Title: "Inception", OriginalTitle: "Inception", OriginalLanguage: "en", ReleaseDate: "2010-07-15", Popularity: 50},
		{ID: 129, Title: "Spirited Away", OriginalTitle: "千と千尋の神隠し", OriginalLanguage: "ja", ReleaseDate: "2001-07-20", Popularity: 40},
	}
	var out []tmdb.Movie
	for _, m := range all {
		if match.Similarity(q, m.Title) > 0.8 && (year == 0 || tmdb.Year(m.ReleaseDate) == year) {
			out = append(out, m)
		}
	}
	return out, nil
}

func (fakeAPI) SearchTV(_ context.Context, q string, year int) ([]tmdb.Show, error) {
	if match.Similarity(q, "Dark") > 0.9 {
		return []tmdb.Show{{ID: 70523, Name: "Dark", OriginalName: "Dark", OriginalLanguage: "de", FirstAirDate: "2017-12-01"}}, nil
	}
	return nil, nil
}
func (fakeAPI) GetMovie(context.Context, int) (tmdb.Movie, error) {
	return tmdb.Movie{}, tmdb.ErrNotFound
}
func (fakeAPI) GetTV(context.Context, int) (tmdb.Show, error) { return tmdb.Show{}, tmdb.ErrNotFound }
func (fakeAPI) GetSeason(_ context.Context, id, s int, lang string) (tmdb.Season, error) {
	if s != 1 {
		return tmdb.Season{}, tmdb.ErrNotFound
	}
	if lang == "de" {
		return tmdb.Season{SeasonNumber: 1, Episodes: []tmdb.Episode{{EpisodeNumber: 1, SeasonNumber: 1, Name: "Geheimnisse"}, {EpisodeNumber: 2, SeasonNumber: 1, Name: "Episode 2"}}}, nil
	}
	return tmdb.Season{SeasonNumber: 1, Episodes: []tmdb.Episode{{EpisodeNumber: 1, SeasonNumber: 1, Name: "Secrets"}, {EpisodeNumber: 2, SeasonNumber: 1, Name: "Lies"}}}, nil
}

func touch(t *testing.T, root, rel string) {
	p := filepath.Join(root, rel)
	os.MkdirAll(filepath.Dir(p), 0o755)
	os.WriteFile(p, []byte("x"), 0o644)
}

func TestEngine(t *testing.T) {
	root := t.TempDir()
	out := t.TempDir()
	for _, f := range []string{
		"Inception.2010.1080p.mkv",
		"Inception.2010.1080p.en.srt",
		"Inception.2010.720p.mkv", // duplicate → conflict
		"Spirited.Away.2001.mkv",
		"Dark/Season 1/01.mkv",
		"Dark/Season 1/02.mkv",
		"Dark/Season 3/01.mkv",
		"Unknown.Thing.1999.mkv",
	} {
		touch(t, root, f)
	}
	sr, err := scan.Scan(root, out)
	if err != nil {
		t.Fatal(err)
	}
	e := New(fakeAPI{}, sr, out)
	e.MatchAll(context.Background(), 4, nil)

	got := map[string]*Item{}
	for _, it := range e.Items {
		got[it.Video.Rel] = it
	}
	rel := func(it *Item) string {
		r, _ := filepath.Rel(out, it.Target)
		return r
	}

	sa := got["Spirited.Away.2001.mkv"]
	if rel(sa) != "Movies/Spirited Away (2001)/Spirited Away (2001).mkv" || sa.Status != StatusOK {
		t.Errorf("spirited away: %s %v %v", rel(sa), sa.Status, sa.Notes)
	}
	d1 := got["Dark/Season 1/01.mkv"]
	if rel(d1) != "TV/Dark (2017)/Season 01/Dark (2017) - S01E01 - Geheimnisse.mkv" || d1.Status != StatusOK {
		t.Errorf("dark 1: %s %v %v", rel(d1), d1.Status, d1.Notes)
	}
	d2 := got["Dark/Season 1/02.mkv"]
	if rel(d2) != "TV/Dark (2017)/Season 01/Dark (2017) - S01E02 - Lies.mkv" {
		t.Errorf("dark 2 should fall back to English: %s", rel(d2))
	}
	d3 := got["Dark/Season 3/01.mkv"]
	if d3.Status != StatusReview || !strings.Contains(strings.Join(d3.Notes, ";"), "season 3 not found") {
		t.Errorf("dark s3: %v %v", d3.Status, d3.Notes)
	}
	a, b := got["Inception.2010.1080p.mkv"], got["Inception.2010.720p.mkv"]
	if a.Status != StatusConflict || b.Status != StatusConflict {
		t.Errorf("duplicates should conflict: %v %v", a.Status, b.Status)
	}
	if un := got["Unknown.Thing.1999.mkv"]; un.Status != StatusUnmatched {
		t.Errorf("unknown: %v", un.Status)
	}

	// Resolve the conflict by deselecting one copy.
	b.Selected = false
	e.Rebuild()
	if a.Status != StatusOK || len(a.Subs) != 1 || !strings.HasSuffix(a.Subs[0].Dst, "Inception (2010).en.srt") {
		t.Errorf("after deselect: %v %v %+v", a.Status, a.Notes, a.Subs)
	}

	// Target exists on disk → conflict.
	touch(t, out, "Movies/Spirited Away (2001)/Spirited Away (2001).mkv")
	e.Rebuild()
	if sa.Status != StatusConflict {
		t.Errorf("existing target should conflict: %v", sa.Status)
	}

	ops := e.Plan()
	srcs := map[string]bool{}
	for _, op := range ops {
		srcs[filepath.Base(op.Src)] = true
	}
	if !srcs["Inception.2010.1080p.mkv"] || !srcs["Inception.2010.1080p.en.srt"] || srcs["Spirited.Away.2001.mkv"] || srcs["Inception.2010.720p.mkv"] {
		t.Errorf("plan: %v", srcs)
	}

	// Manual choice switches kind and selects items.
	g := got["Unknown.Thing.1999.mkv"].Group
	r := e.Choose(context.Background(), g, match.Candidate{ID: 1, Kind: media.Movie, Title: "Unknown Thing", OriginalTitle: "Unknown Thing", Year: 1999})
	e.ApplyResult(r)
	e.Rebuild()
	if it := g.Items[0]; it.Status != StatusOK || !it.Selected {
		t.Errorf("manual choice: %v %v", it.Status, it.Notes)
	}
}
