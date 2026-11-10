//go:build e2e

// End-to-end test against the real TMDB API.
// Run with: TMDB_TOKEN=... go test -tags e2e ./internal/engine -run E2E -v
package engine

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"renamer/internal/scan"
	"renamer/internal/tmdb"
)

func TestE2E(t *testing.T) {
	tok, key := os.Getenv("TMDB_TOKEN"), os.Getenv("TMDB_API_KEY")
	if tok == "" && key == "" {
		t.Skip("no TMDB credentials")
	}
	root := t.TempDir()
	out := t.TempDir()
	files := []string{
		// movies
		"The.Matrix.1999.1080p.BluRay.x264-GROUP.mkv",
		"Spirited.Away.2001.1080p.BluRay.mkv",
		"Amelie.2001.FRENCH.1080p.mkv",
		"Parasite (2019)/parasite.mkv",
		"Inception.2010.1080p.BluRay.x264-SPARKS/sparks-inception.mkv",
		"Inception.2010.1080p.BluRay.x264-SPARKS/sparks-inception.en.srt",
		"Kill.Bill.Vol.1.2003.CD1.avi",
		"Kill.Bill.Vol.1.2003.CD2.avi",
		"Dune.2021.2160p.mkv",
		"Oldboy.2003.KOREAN.mkv",
		// tv: markers
		"Breaking.Bad.S01E01.720p.HDTV.x264.mkv",
		"The Office (US) - 2x05 - Halloween.avi",
		// tv: folders only
		"Dark/Season 1/01.mkv",
		"Dark/Season 1/02.mkv",
		"Dark/Season 1/Subs/01/2_English.srt",
		"Money Heist/Temporada 1/Capitulo 3.mkv",
		"Shogun.2024.S01.1080p.WEB-DL/03.mkv",
		"Attack on Titan/Season 1/Episode 5.mkv",
		"Fleabag/Series 2/E01.mkv",
		// junk
		"sample-matrix.mkv",
		"Asdfgh Qwerty 1987.mkv",
	}
	for _, f := range files {
		p := filepath.Join(root, f)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte("x"), 0o644)
	}
	sr, err := scan.Scan(root, out)
	if err != nil {
		t.Fatal(err)
	}
	c := tmdb.New(tok, key, tmdb.NewCache(filepath.Join(os.TempDir(), "renamer-e2e-cache"), time.Hour))
	e := New(c, sr, out)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	e.MatchAll(ctx, 8, nil)

	var lines []string
	for _, it := range e.Items {
		r, _ := filepath.Rel(out, it.Target)
		s := []string{"OK", "REVIEW", "LOW", "UNMATCHED", "CONFLICT", "INPLACE"}[it.Status]
		line := s + "\t" + it.Video.Rel + "\n\t→ " + r
		for _, sub := range it.Subs {
			sr, _ := filepath.Rel(out, sub.Dst)
			line += "\n\t  + " + sr
		}
		for _, n := range it.Notes {
			line += "\n\t  ! " + n
		}
		lines = append(lines, line)
	}
	sort.Strings(lines)
	for _, l := range lines {
		t.Log(l)
	}
	for _, ig := range e.Ignored {
		t.Logf("IGNORED %s (%s)", ig.Rel, ig.Reason)
	}

	want := map[string]string{
		"The.Matrix.1999.1080p.BluRay.x264-GROUP.mkv":                  "Movies/The Matrix (1999)/The Matrix (1999).mkv",
		"Spirited.Away.2001.1080p.BluRay.mkv":                          "Movies/Spirited Away (2001)/Spirited Away (2001).mkv",
		"Parasite (2019)/parasite.mkv":                                 "Movies/Parasite (2019)/Parasite (2019).mkv",
		"Inception.2010.1080p.BluRay.x264-SPARKS/sparks-inception.mkv": "Movies/Inception (2010)/Inception (2010).mkv",
		"Kill.Bill.Vol.1.2003.CD2.avi":                                 "Movies/Kill Bill - Vol. 1 (2003)/Kill Bill - Vol. 1 (2003) - part2.avi",
		"Breaking.Bad.S01E01.720p.HDTV.x264.mkv":                       "TV/Breaking Bad (2008)/Season 01/Breaking Bad (2008) - S01E01 - Pilot.mkv",
		"Dark/Season 1/01.mkv":                                         "TV/Dark (2017)/Season 01/Dark (2017) - S01E01 - Geheimnisse.mkv",
	}
	for _, it := range e.Items {
		if w, ok := want[it.Video.Rel]; ok {
			r, _ := filepath.Rel(out, it.Target)
			if r != w {
				t.Errorf("%s\n  got  %s\n  want %s", it.Video.Rel, r, w)
			}
		}
	}
}
