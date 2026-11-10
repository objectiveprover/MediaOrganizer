// Package match ranks TMDB candidates against a parsed title and year.
package match

import (
	"sort"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"

	"renamer/internal/media"
	"renamer/internal/naming"
	"renamer/internal/tmdb"
)

// Candidate is a TMDB movie or show considered for a group of files.
type Candidate struct {
	ID               int
	Kind             media.Kind
	Title            string // English (or best available) title
	OriginalTitle    string
	OriginalLanguage string
	Year             int
	Overview         string
	Popularity       float64

	Sim   float64 // title similarity to the query, 0..1
	Score float64 // overall score used for ranking
}

// Name is the title used for file names (original if Latin, else English).
func (c Candidate) Name() string { return naming.DisplayTitle(c.OriginalTitle, c.Title) }

// FromMovie converts a TMDB movie.
func FromMovie(m tmdb.Movie) Candidate {
	return Candidate{ID: m.ID, Kind: media.Movie, Title: m.Title, OriginalTitle: m.OriginalTitle,
		OriginalLanguage: m.OriginalLanguage, Year: tmdb.Year(m.ReleaseDate), Overview: m.Overview, Popularity: m.Popularity}
}

// FromShow converts a TMDB show.
func FromShow(s tmdb.Show) Candidate {
	return Candidate{ID: s.ID, Kind: media.TV, Title: s.Name, OriginalTitle: s.OriginalName,
		OriginalLanguage: s.OriginalLanguage, Year: tmdb.Year(s.FirstAirDate), Overview: s.Overview, Popularity: s.Popularity}
}

// Confidence is how sure we are about the best candidate.
type Confidence int

const (
	None Confidence = iota
	Low
	Medium
	High
	Manual // chosen by the user
)

func (c Confidence) String() string {
	return [...]string{"none", "low", "medium", "high", "manual"}[c]
}

// stripMarks returns a fresh transformer; transformers are stateful and
// must not be shared between goroutines.
func stripMarks() transform.Transformer {
	return transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
}

// Normalize lowercases, strips accents and punctuation, and drops a leading article.
func Normalize(s string) string {
	if t, _, err := transform.String(stripMarks(), s); err == nil {
		s = t
	}
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "&", " and ")
	s = strings.NewReplacer("'", "", "’", "", "`", "").Replace(s)
	s = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		return ' '
	}, s)
	f := strings.Fields(s)
	if len(f) > 1 && (f[0] == "the" || f[0] == "a" || f[0] == "an") {
		f = f[1:]
	}
	return strings.Join(f, " ")
}

func levenshtein(a, b []rune) int {
	if len(a) == 0 {
		return len(b)
	}
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}

// Similarity returns a 0..1 score between two titles.
func Similarity(a, b string) float64 {
	na, nb := Normalize(a), Normalize(b)
	if na == "" || nb == "" {
		return 0
	}
	if na == nb {
		return 1
	}
	// Ignore spaces for the edit-distance part so "Spider Man" == "Spiderman".
	ra := []rune(strings.ReplaceAll(na, " ", ""))
	rb := []rune(strings.ReplaceAll(nb, " ", ""))
	if string(ra) == string(rb) {
		return 0.98
	}
	lev := 1 - float64(levenshtein(ra, rb))/float64(max(len(ra), len(rb)))
	// Token overlap (Dice).
	ta, tb := strings.Fields(na), strings.Fields(nb)
	set := map[string]int{}
	for _, t := range ta {
		set[t] |= 1
	}
	for _, t := range tb {
		set[t] |= 2
	}
	inter := 0
	for _, v := range set {
		if v == 3 {
			inter++
		}
	}
	dice := 2 * float64(inter) / float64(len(ta)+len(tb))
	return max(lev, dice*0.95)
}

// Rank scores candidates for the given query/alternatives/year, best first.
func Rank(queries []string, year int, cands []Candidate) []Candidate {
	out := make([]Candidate, len(cands))
	copy(out, cands)
	maxPop := 0.0
	for _, c := range out {
		maxPop = max(maxPop, c.Popularity)
	}
	for i := range out {
		c := &out[i]
		c.Sim = 0
		for _, q := range queries {
			if q == "" {
				continue
			}
			c.Sim = max(c.Sim, Similarity(q, c.Title), Similarity(q, c.OriginalTitle))
		}
		ys := 0.1 // unknown year: neutral
		if year > 0 {
			switch d := abs(c.Year - year); {
			case c.Year == 0:
				ys = 0.05
			case d == 0:
				ys = 0.25
			case d == 1:
				ys = 0.18
			default:
				ys = 0
			}
		}
		pop := 0.0
		if maxPop > 0 {
			pop = 0.03 * c.Popularity / maxPop
		}
		c.Score = 0.75*c.Sim + ys + pop
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// Assess returns how confident we are in ranked[0].
func Assess(ranked []Candidate, year int) Confidence {
	if len(ranked) == 0 {
		return None
	}
	b := ranked[0]
	yearOK := year == 0 || (b.Year > 0 && abs(b.Year-year) <= 1)
	margin := 1.0
	if len(ranked) > 1 {
		margin = b.Score - ranked[1].Score
	}
	switch {
	case b.Sim >= 0.9 && yearOK && (margin >= 0.04 || (year > 0 && b.Year == year && ranked[1].Year != year)):
		return High
	case b.Sim >= 0.75 && (yearOK || b.Sim >= 0.95):
		return Medium
	}
	return Low
}
