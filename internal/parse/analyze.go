package parse

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// info is what can be learned from a single name (file or folder).
type info struct {
	Title    string // text before any marker/year/noise
	FullText string // title words including a trailing year-like token (alternative query)
	Year     int
	Season   int   // -1 if unknown
	Episodes []int // nil if unknown
	HasSE    bool  // explicit season+episode marker
	EpOnly   bool  // episode marker without season (E03, Episode 3)
	Range    bool  // multi-season pack (S01-S05)
	Part     int   // CD1 / Disc 2
	Specials bool  // "Specials" folder
}

var (
	reSE = regexp.MustCompile(`(?i)\bs(\d{1,3}) ?-? ?e(\d{1,4})((?:(?: ?-? ?e)\d{1,4})*)(?:-(\d{1,4}))?\b`)
	reX  = regexp.MustCompile(`(?i)\b(\d{1,2})x(\d{2,3})(?:(?:x|-)(\d{2,3}))?\b`)
	// "Season 2 Episode 5"
	reSeasonEpText = regexp.MustCompile(`(?i)\b(?:season|temporada|series|saison|staffel) ?(\d{1,2}) ?-? ?(?:episode|episodio|ep|e) ?(\d{1,4})\b`)
	reSeasonRange  = regexp.MustCompile(`(?i)\b(?:s(\d{1,2}) ?- ?s?(\d{1,2})|(?:seasons?|temporadas?|series) ?(\d{1,2}) ?(?:-|to|a) ?(\d{1,2}))\b`)
	reSeasonNum    = regexp.MustCompile(`(?i)\b(?:s|(?:season|series|temporada|saison|staffel|stagione) ?)(\d{1,2})\b`)
	reSeasonWord   = regexp.MustCompile(`(?i)\b(?:season|temporada|series|saison) ([a-z]+)\b`)
	reSeasonOrd    = regexp.MustCompile(`(?i)\b([a-z]+) (?:season|temporada)\b`)
	reEpOnly       = regexp.MustCompile(`(?i)\b(?:(?:episode|episodio|ep|cap[ií]tulo|cap) ?\.? ?(\d{1,4})|e(\d{1,4}))\b`)
	reEpE          = regexp.MustCompile(`(?i)e(\d{1,4})`)
	rePart         = regexp.MustCompile(`(?i)\b(?:cd|disc|disk) ?(\d)\b`)
	reSpecials     = regexp.MustCompile(`(?i)^(?:specials?|season 0+|s0+|extras? episodes?)$`)
	reCompleteTail = regexp.MustCompile(`(?i)(?:\s|-)+(?:the )?complete(?: (?:series|collection|seasons?|serie|box ?set))?$`)
	reTrimPunct    = regexp.MustCompile(`^[\s\-–—:,.+&]+|[\s\-–—:,.+&]+$`)
	reHasLetter    = regexp.MustCompile(`\pL`)
)

func atoi(s string) int { n, _ := strconv.Atoi(s); return n }

func expand(from, to int) []int {
	if to < from || to-from > 50 {
		return []int{from}
	}
	out := make([]int, 0, to-from+1)
	for i := from; i <= to; i++ {
		out = append(out, i)
	}
	return out
}

func uniqSorted(xs []int) []int {
	sort.Ints(xs)
	out := xs[:0]
	for i, x := range xs {
		if i == 0 || x != xs[i-1] {
			out = append(out, x)
		}
	}
	return out
}

// analyze extracts as much as possible from one name.
func analyze(raw string) info {
	in := info{Season: -1}
	n := normalize(raw)
	if reSpecials.MatchString(n) {
		in.Season = 0
		in.Specials = true
		return in
	}
	cut := len(n)
	setCut := func(i int) {
		if i < cut {
			cut = i
		}
	}

	if m := reSE.FindStringSubmatchIndex(n); m != nil {
		in.HasSE = true
		in.Season = atoi(n[m[2]:m[3]])
		first := atoi(n[m[4]:m[5]])
		eps := []int{first}
		last := first
		if m[6] >= 0 {
			for _, e := range reEpE.FindAllStringSubmatch(n[m[6]:m[7]], -1) {
				v := atoi(e[1])
				// "E01-E03" is a range, "E01E02" a list.
				eps = append(eps, v)
				last = v
			}
			if strings.Contains(n[m[6]:m[7]], "-") && len(eps) == 2 {
				eps = expand(first, last)
			}
		}
		if m[8] >= 0 {
			eps = expand(first, atoi(n[m[8]:m[9]]))
		}
		in.Episodes = uniqSorted(eps)
		setCut(m[0])
	} else if m := reSeasonEpText.FindStringSubmatchIndex(n); m != nil {
		in.HasSE = true
		in.Season = atoi(n[m[2]:m[3]])
		in.Episodes = []int{atoi(n[m[4]:m[5]])}
		setCut(m[0])
	} else if m := reX.FindStringSubmatchIndex(n); m != nil {
		in.HasSE = true
		in.Season = atoi(n[m[2]:m[3]])
		first := atoi(n[m[4]:m[5]])
		in.Episodes = []int{first}
		if m[6] >= 0 {
			in.Episodes = expand(first, atoi(n[m[6]:m[7]]))
		}
		setCut(m[0])
	} else if m := reSeasonRange.FindStringSubmatchIndex(n); m != nil {
		in.Range = true
		setCut(m[0])
	} else {
		if m := reSeasonNum.FindStringSubmatchIndex(n); m != nil {
			in.Season = atoi(n[m[2]:m[3]])
			setCut(m[0])
		} else if m := reSeasonWord.FindStringSubmatchIndex(n); m != nil {
			if v, ok := wordNumbers[strings.ToLower(n[m[2]:m[3]])]; ok {
				in.Season = v
				setCut(m[0])
			}
		} else if m := reSeasonOrd.FindStringSubmatchIndex(n); m != nil {
			if v, ok := wordNumbers[strings.ToLower(n[m[2]:m[3]])]; ok {
				in.Season = v
				setCut(m[0])
			}
		}
		if m := reEpOnly.FindStringSubmatchIndex(n); m != nil {
			v := m[2]
			e := m[3]
			if v < 0 {
				v, e = m[4], m[5]
			}
			in.EpOnly = true
			in.Episodes = []int{atoi(n[v:e])}
			setCut(m[0])
		}
	}
	if m := rePart.FindStringSubmatchIndex(n); m != nil {
		in.Part = atoi(n[m[2]:m[3]])
		setCut(m[0])
	}

	// Token scan of the prefix for year and noise.
	toks := strings.Fields(n[:cut])
	stop := len(toks)
	for i, t := range toks {
		if isNoise(t) {
			stop = i
			break
		}
	}
	toks = toks[:stop]
	yearIdx := -1
	for i, t := range toks {
		if i == 0 {
			continue
		}
		if y := yearOf(t); y > 0 {
			yearIdx = i
			in.Year = y
		}
	}
	full := strings.Join(toks, " ")
	if yearIdx >= 0 {
		toks = toks[:yearIdx]
	}
	in.Title = cleanTitle(strings.Join(toks, " "))
	in.FullText = cleanTitle(full)
	if in.FullText == in.Title {
		in.FullText = ""
	}
	return in
}

func cleanTitle(s string) string {
	s = reCompleteTail.ReplaceAllString(s, "")
	s = reTrimPunct.ReplaceAllString(s, "")
	s = strings.TrimSpace(reSpaces.ReplaceAllString(s, " "))
	if !reHasLetter.MatchString(s) && len(s) < 3 {
		// lone numbers like "01" are not titles
		return ""
	}
	return s
}

var (
	reBareWhole   = regexp.MustCompile(`^(\d{1,3})$`)
	reBareLeading = regexp.MustCompile(`^(\d{1,3})(?:\s*[-.]\s*|\s+)(.*)$`)
	reBareDash    = regexp.MustCompile(`^(.*?)\s+-\s+(\d{1,3})(?:\s|$)`)
	reBareTrail   = regexp.MustCompile(`^(.*\pL.*?)\s+(\d{1,3})$`)
)

// bareEpisode finds an episode number in a name with no explicit marker.
// strict limits it to patterns that strongly suggest numbering
// (whole name, leading number, " - 03").
func bareEpisode(raw string, strict bool) (ep int, title string, ok bool) {
	n := normalize(raw)
	// Drop noise tail so "Show 03 1080p" works.
	toks := strings.Fields(n)
	for i, t := range toks {
		if isNoise(t) {
			toks = toks[:i]
			break
		}
	}
	n = strings.Join(toks, " ")
	if m := reBareWhole.FindStringSubmatch(n); m != nil {
		return atoi(m[1]), "", true
	}
	if m := reBareLeading.FindStringSubmatch(n); m != nil {
		if yearOf(m[1]) == 0 {
			return atoi(m[1]), "", true
		}
	}
	if m := reBareDash.FindStringSubmatch(n); m != nil {
		return atoi(m[2]), cleanTitle(m[1]), true
	}
	if !strict {
		if m := reBareTrail.FindStringSubmatch(n); m != nil {
			return atoi(m[2]), cleanTitle(m[1]), true
		}
	}
	return 0, "", false
}

// HasBareNumber reports whether a filename looks like a numbered episode
// with no other markers (used for sibling detection).
func HasBareNumber(filename string) bool {
	in := analyze(stripExt(filename))
	if in.HasSE || in.EpOnly || in.Year > 0 {
		return false
	}
	_, _, ok := bareEpisode(stripExt(filename), true)
	return ok
}
