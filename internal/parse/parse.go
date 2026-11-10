// Package parse extracts title, year, season and episode information from
// media file names and the folders that contain them.
package parse

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"renamer/internal/media"
)

// Result is the parsed interpretation of one video file.
type Result struct {
	Kind      media.Kind
	Title     string // movie title or show name used as the TMDB query
	Year      int    // 0 if unknown
	Season    int    // -1 if unknown (movies)
	Episodes  []int  // nil for movies
	Part      int    // CD1/Disc 2 for split movies, 0 if none
	AltTitles []string
	Notes     []string // human-readable explanation of where values came from
}

// Hint carries context computed from the directory, not the file itself.
type Hint struct {
	// NumberedSiblings is true when most videos in the same directory
	// are named with bare numbers ("01.mkv", "02 - Title.mkv").
	NumberedSiblings bool
}

func stripExt(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	if media.IsVideo(ext) || media.IsSubtitle(ext) {
		return name[:len(name)-len(ext)]
	}
	return name
}

var reObfuscated = regexp.MustCompile(`(?i)^(?:[a-z0-9]{2,10}-[a-z0-9]{2,16}|[a-f0-9]{12,}|[a-z0-9]{20,})$`)

func looksObfuscated(base string) bool {
	n := normalize(base)
	if !reHasLetter.MatchString(n) {
		return true
	}
	return !strings.Contains(n, " ") && reObfuscated.MatchString(n)
}

// Parse interprets the file at path, using folders between root and the file
// (plus root's own name) as extra context.
func Parse(root, path string, hint Hint) Result {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		rel = filepath.Base(path)
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	file := parts[len(parts)-1]
	var anc []string // nearest first
	for i := len(parts) - 2; i >= 0; i-- {
		anc = append(anc, parts[i])
	}
	if rb := filepath.Base(root); rb != "/" && rb != "." {
		anc = append(anc, rb)
	}
	base := stripExt(file)
	fi := analyze(base)

	// Gather folder context.
	var (
		show            string
		showYear        int
		showFrom        string
		season          = -1
		seasonFrom      string
		folderPart      int
		parent          info
		parentName      string
		parentHasRealTi bool
	)
	inRoot := len(parts) - 1 // number of ancestors inside root; the rest is root's own name
	for i, a := range anc {
		if IsGenericFolder(a) || IsExtrasFolder(a) {
			continue
		}
		ai := analyze(a)
		if ai.Part > 0 && ai.Title == "" {
			folderPart = ai.Part
			continue
		}
		// The root's own name may name a show (renamer run on a show folder),
		// but is never used as a movie's parent folder.
		if parentName == "" && i < inRoot {
			parent, parentName = ai, a
			parentHasRealTi = ai.Title != ""
		}
		if season < 0 && ai.Season >= 0 {
			season, seasonFrom = ai.Season, a
		}
		if ai.Title != "" {
			show, showYear, showFrom = ai.Title, ai.Year, a
			break
		}
	}

	r := Result{Season: -1}
	addAlt := func(ts ...string) {
		for _, t := range ts {
			if t == "" || strings.EqualFold(t, r.Title) {
				continue
			}
			dup := false
			for _, x := range r.AltTitles {
				if strings.EqualFold(x, t) {
					dup = true
				}
			}
			if !dup {
				r.AltTitles = append(r.AltTitles, t)
			}
		}
	}
	note := func(f string, a ...any) { r.Notes = append(r.Notes, fmt.Sprintf(f, a...)) }
	useShow := func() {
		r.Title, r.Year = show, showYear
		if show != "" {
			note("show from folder %q", showFrom)
		}
	}
	pickSeason := func(fromFile int) {
		switch {
		case fromFile >= 0:
			r.Season = fromFile
		case season >= 0:
			r.Season = season
			note("season from folder %q", seasonFrom)
		default:
			r.Season = 1
			note("no season found, assuming 1")
		}
	}

	switch {
	case fi.HasSE:
		r.Kind = media.TV
		r.Season, r.Episodes = fi.Season, fi.Episodes
		if fi.Title != "" && !looksObfuscated(fi.Title) {
			r.Title, r.Year = fi.Title, fi.Year
			addAlt(fi.FullText, show)
		} else {
			useShow()
		}
		return r

	case fi.EpOnly:
		r.Kind = media.TV
		r.Episodes = fi.Episodes
		pickSeason(fi.Season)
		if fi.Title != "" && show == "" {
			r.Title, r.Year = fi.Title, fi.Year
		} else {
			useShow()
			addAlt(fi.Title)
		}
		return r
	}

	if season >= 0 || hint.NumberedSiblings || fi.Season >= 0 {
		if ep, t, ok := bareEpisode(base, season < 0 && fi.Season < 0); ok {
			r.Kind = media.TV
			r.Episodes = []int{ep}
			pickSeason(fi.Season)
			if show != "" {
				useShow()
				addAlt(t)
			} else {
				r.Title = t
			}
			note("episode number from bare number in file name")
			return r
		}
	}

	// Obfuscated file inside a per-episode scene folder: "Show.S01E02.720p/abc.mkv".
	if parentName != "" && (fi.Title == "" || looksObfuscated(base)) {
		if pi := analyze(parentName); pi.HasSE {
			r.Kind = media.TV
			r.Season, r.Episodes = pi.Season, pi.Episodes
			r.Title, r.Year = pi.Title, pi.Year
			note("all info from folder %q", parentName)
			return r
		}
	}

	// Movie.
	r.Kind = media.Movie
	r.Part = fi.Part
	if r.Part == 0 {
		r.Part = folderPart
	}
	r.Title, r.Year = fi.Title, fi.Year
	useFolder := fi.Title == "" || looksObfuscated(base)
	if !useFolder && fi.Year == 0 && parentHasRealTi && parent.Year > 0 {
		useFolder = true
	}
	if useFolder && parentHasRealTi {
		r.Title, r.Year = parent.Title, parent.Year
		note("title from folder %q", parentName)
		addAlt(fi.Title, fi.FullText, parent.FullText)
	} else {
		if r.Year == 0 && parentHasRealTi && parent.Year > 0 && strings.EqualFold(parent.Title, r.Title) {
			r.Year = parent.Year
		}
		addAlt(fi.FullText)
		if parentHasRealTi {
			addAlt(parent.Title)
		}
	}
	return r
}
