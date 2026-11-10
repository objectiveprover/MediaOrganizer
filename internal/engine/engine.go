// Package engine groups scanned videos, matches them against TMDB, and
// builds the rename plan with conflict detection.
package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"renamer/internal/match"
	"renamer/internal/media"
	"renamer/internal/naming"
	"renamer/internal/scan"
	"renamer/internal/tmdb"
)

// API is the subset of the TMDB client the engine needs.
type API interface {
	SearchMovie(ctx context.Context, q string, year int) ([]tmdb.Movie, error)
	SearchTV(ctx context.Context, q string, year int) ([]tmdb.Show, error)
	GetMovie(ctx context.Context, id int) (tmdb.Movie, error)
	GetTV(ctx context.Context, id int) (tmdb.Show, error)
	GetSeason(ctx context.Context, tvID, season int, lang string) (tmdb.Season, error)
}

// Status of one item in the plan.
type Status int

const (
	StatusOK        Status = iota // confident match, ready
	StatusReview                  // probably right, worth a look
	StatusLow                     // low confidence: not selected by default
	StatusUnmatched               // no TMDB match / missing info
	StatusConflict                // target collides
	StatusInPlace                 // already at target
)

// NeedsAttention reports whether the status warrants user review.
func (s Status) NeedsAttention() bool { return s != StatusOK && s != StatusInPlace }

// SubOp is a subtitle move attached to an item.
type SubOp struct {
	Src, Dst string
	Skip     string // reason it won't be moved, if any
}

// Item is one video file and its planned destination.
type Item struct {
	Video    *scan.Video
	Group    *Group
	Season   int
	Episodes []int
	Selected bool
	Target   string
	Subs     []SubOp
	Status   Status
	Notes    []string
}

// SeasonInfo holds episode names for one season. A key present in Titles
// means TMDB knows the episode (the name may be "").
type SeasonInfo struct {
	Titles map[int]string
	Err    error
}

// Group is a set of items that share one TMDB lookup (a show, or a movie).
type Group struct {
	Key        string
	Kind       media.Kind
	Query      string
	Year       int
	Alts       []string
	Candidates []match.Candidate
	Chosen     *match.Candidate
	Confidence match.Confidence
	Seasons    map[int]*SeasonInfo
	Items      []*Item
	Err        error
}

// Label is a short description for display.
func (g *Group) Label() string {
	if g.Chosen != nil {
		return naming.WithYear(g.Chosen.Name(), g.Chosen.Year)
	}
	if g.Query == "" {
		return "(no title found)"
	}
	if g.Year > 0 {
		return fmt.Sprintf("%s (%d)?", g.Query, g.Year)
	}
	return g.Query + "?"
}

// NeededSeasons lists seasons referenced by the group's items.
func (g *Group) NeededSeasons() []int {
	seen := map[int]bool{}
	var out []int
	for _, it := range g.Items {
		if it.Season >= 0 && !seen[it.Season] {
			seen[it.Season] = true
			out = append(out, it.Season)
		}
	}
	sort.Ints(out)
	return out
}

// Engine holds the whole session state.
type Engine struct {
	API     API
	Root    string
	Out     string
	Groups  []*Group
	Items   []*Item
	Ignored []scan.Ignored
}

// New builds groups and items from a scan.
func New(api API, sr *scan.Result, out string) *Engine {
	e := &Engine{API: api, Root: sr.Root, Out: out, Ignored: sr.Ignored}
	byKey := map[string]*Group{}
	for _, v := range sr.Videos {
		p := v.Parsed
		key := fmt.Sprintf("%s|%s|%d", p.Kind, match.Normalize(p.Title), p.Year)
		if p.Title == "" {
			key = "untitled|" + v.Path
		}
		g := byKey[key]
		if g == nil {
			g = &Group{Key: key, Kind: p.Kind, Query: p.Title, Year: p.Year, Alts: p.AltTitles, Seasons: map[int]*SeasonInfo{}}
			byKey[key] = g
			e.Groups = append(e.Groups, g)
		} else {
			for _, a := range p.AltTitles {
				if !contains(g.Alts, a) {
					g.Alts = append(g.Alts, a)
				}
			}
		}
		it := &Item{Video: v, Group: g, Season: p.Season, Episodes: p.Episodes}
		g.Items = append(g.Items, it)
		e.Items = append(e.Items, it)
	}
	e.Sort()
	return e
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if strings.EqualFold(x, s) {
			return true
		}
	}
	return false
}

// Sort orders items by group label, then season/episode, then path.
func (e *Engine) Sort() {
	sort.SliceStable(e.Groups, func(i, j int) bool {
		a, b := e.Groups[i], e.Groups[j]
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		return strings.ToLower(a.Label()) < strings.ToLower(b.Label())
	})
	e.Items = e.Items[:0]
	for _, g := range e.Groups {
		sort.SliceStable(g.Items, func(i, j int) bool {
			a, b := g.Items[i], g.Items[j]
			if a.Season != b.Season {
				return a.Season < b.Season
			}
			ea, eb := first(a.Episodes), first(b.Episodes)
			if ea != eb {
				return ea < eb
			}
			return a.Video.Rel < b.Video.Rel
		})
		e.Items = append(e.Items, g.Items...)
	}
}

func first(xs []int) int {
	if len(xs) == 0 {
		return -1
	}
	return xs[0]
}

// ---- Network work (safe to run off the UI goroutine; does not mutate groups)

// GroupResult is the outcome of looking up one group.
type GroupResult struct {
	Group      *Group
	Candidates []match.Candidate
	Chosen     *match.Candidate
	Confidence match.Confidence
	Seasons    map[int]*SeasonInfo
	Err        error
	Manual     bool
}

// Search runs a TMDB search of the given kind and ranks the results.
func (e *Engine) Search(ctx context.Context, kind media.Kind, query string, year int, alts []string) ([]match.Candidate, error) {
	var cands []match.Candidate
	if kind == media.TV {
		rs, err := e.API.SearchTV(ctx, query, year)
		if err != nil {
			return nil, err
		}
		for _, r := range rs {
			cands = append(cands, match.FromShow(r))
		}
	} else {
		rs, err := e.API.SearchMovie(ctx, query, year)
		if err != nil {
			return nil, err
		}
		for _, r := range rs {
			cands = append(cands, match.FromMovie(r))
		}
	}
	return match.Rank(append([]string{query}, alts...), year, cands), nil
}

// Lookup searches TMDB for a group and, for TV, loads episode names.
func (e *Engine) Lookup(ctx context.Context, g *Group) GroupResult {
	res := GroupResult{Group: g}
	queries := append([]string{g.Query}, g.Alts...)
	var best []match.Candidate
	bestConf := match.None
	for _, q := range queries {
		if strings.TrimSpace(q) == "" {
			continue
		}
		ranked, err := e.Search(ctx, g.Kind, q, g.Year, queries)
		if err != nil {
			res.Err = err
			break
		}
		if len(ranked) == 0 && g.Year > 0 {
			if ranked, err = e.Search(ctx, g.Kind, q, 0, queries); err != nil {
				res.Err = err
				break
			}
		}
		if len(ranked) == 0 {
			continue
		}
		// Re-rank with the group's year even if the search dropped it.
		ranked = match.Rank(queries, g.Year, ranked)
		conf := match.Assess(ranked, g.Year)
		if best == nil || ranked[0].Score > best[0].Score {
			best, bestConf = ranked, conf
		}
		if bestConf == match.High {
			break
		}
	}
	if len(best) > 15 {
		best = best[:15]
	}
	res.Candidates, res.Confidence = best, bestConf
	if len(best) > 0 {
		c := best[0]
		res.Chosen = &c
		res.Err = nil
	}
	if res.Chosen != nil && res.Chosen.Kind == media.TV {
		res.Seasons = e.LoadSeasons(ctx, *res.Chosen, g.NeededSeasons())
	}
	return res
}

// Choose prepares a manual choice for a group (loads seasons for TV).
func (e *Engine) Choose(ctx context.Context, g *Group, c match.Candidate) GroupResult {
	res := GroupResult{Group: g, Candidates: g.Candidates, Chosen: &c, Confidence: match.Manual, Manual: true}
	if c.Kind == media.TV {
		seasons := g.NeededSeasons()
		if len(seasons) == 0 {
			seasons = []int{1}
		}
		res.Seasons = e.LoadSeasons(ctx, c, seasons)
	}
	return res
}

// ByID fetches a candidate directly by TMDB ID.
func (e *Engine) ByID(ctx context.Context, kind media.Kind, id int) (match.Candidate, error) {
	if kind == media.TV {
		s, err := e.API.GetTV(ctx, id)
		if err != nil {
			return match.Candidate{}, err
		}
		return match.FromShow(s), nil
	}
	m, err := e.API.GetMovie(ctx, id)
	if err != nil {
		return match.Candidate{}, err
	}
	return match.FromMovie(m), nil
}

// LoadSeasons fetches episode names for the given seasons, preferring the
// show's original language and falling back to English for non-Latin or
// placeholder names.
func (e *Engine) LoadSeasons(ctx context.Context, show match.Candidate, seasons []int) map[int]*SeasonInfo {
	out := map[int]*SeasonInfo{}
	primary := "en-US"
	if show.OriginalLanguage != "" && show.OriginalLanguage != "en" {
		primary = show.OriginalLanguage
	}
	for _, s := range seasons {
		si := &SeasonInfo{Titles: map[int]string{}}
		out[s] = si
		se, err := e.API.GetSeason(ctx, show.ID, s, primary)
		if err != nil {
			si.Err = err
			continue
		}
		needEN := false
		for _, ep := range se.Episodes {
			name := strings.TrimSpace(ep.Name)
			if naming.IsGenericEpisodeName(name) || !naming.IsLatin(name) {
				name = ""
				needEN = true
			}
			si.Titles[ep.EpisodeNumber] = name
		}
		if needEN && primary != "en-US" {
			if en, err := e.API.GetSeason(ctx, show.ID, s, "en-US"); err == nil {
				for _, ep := range en.Episodes {
					if cur, ok := si.Titles[ep.EpisodeNumber]; ok && cur == "" && !naming.IsGenericEpisodeName(ep.Name) {
						si.Titles[ep.EpisodeNumber] = strings.TrimSpace(ep.Name)
					}
				}
			}
		}
	}
	return out
}

// MatchAll looks up every group concurrently and applies the results.
// progress is called after each group completes.
func (e *Engine) MatchAll(ctx context.Context, workers int, progress func(done, total int)) {
	jobs := make(chan *Group)
	results := make(chan GroupResult)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for g := range jobs {
				results <- e.Lookup(ctx, g)
			}
		}()
	}
	go func() {
		for _, g := range e.Groups {
			jobs <- g
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()
	done := 0
	for r := range results {
		e.ApplyResult(r)
		done++
		if progress != nil {
			progress(done, len(e.Groups))
		}
	}
	e.Sort()
	e.Rebuild()
}

// ---- State mutation (call from a single goroutine)

// ApplyResult stores a lookup/choice result on its group.
func (e *Engine) ApplyResult(r GroupResult) {
	g := r.Group
	if r.Candidates != nil {
		g.Candidates = r.Candidates
	}
	g.Err = r.Err
	if r.Chosen == nil {
		g.Chosen = nil
		g.Confidence = match.None
		for _, it := range g.Items {
			it.Selected = false
		}
		return
	}
	prevKind := g.Kind
	g.Chosen = r.Chosen
	g.Kind = r.Chosen.Kind
	g.Confidence = r.Confidence
	if r.Manual || prevKind != g.Kind {
		g.Seasons = map[int]*SeasonInfo{}
	}
	for s, si := range r.Seasons {
		g.Seasons[s] = si
	}
	for _, it := range g.Items {
		if g.Kind == media.TV && it.Season < 0 {
			it.Season = 1
		}
		if g.Kind == media.Movie {
			it.Season, it.Episodes = -1, nil
		}
		it.Selected = r.Confidence >= match.Medium
	}
}

// SetEpisode overrides season/episodes for one item.
func (e *Engine) SetEpisode(it *Item, season int, eps []int) {
	it.Season, it.Episodes = season, eps
	it.Selected = true
}

// MissingSeasons returns seasons referenced by the group that aren't loaded.
func (g *Group) MissingSeasons() []int {
	var out []int
	for _, s := range g.NeededSeasons() {
		if _, ok := g.Seasons[s]; !ok {
			out = append(out, s)
		}
	}
	return out
}

// Rebuild recomputes targets, statuses and conflicts for every item.
func (e *Engine) Rebuild() {
	for _, it := range e.Items {
		e.buildTarget(it)
	}
	// Conflicts between selected items (videos and subtitles share one namespace).
	owners := map[string][]*Item{}
	for _, it := range e.Items {
		if !it.Selected || it.Target == "" || it.Status == StatusUnmatched {
			continue
		}
		owners[it.Target] = append(owners[it.Target], it)
		for _, s := range it.Subs {
			if s.Skip == "" {
				owners[s.Dst] = append(owners[s.Dst], it)
			}
		}
	}
	for _, its := range owners {
		uniq := map[*Item]bool{}
		for _, it := range its {
			uniq[it] = true
		}
		if len(uniq) < 2 {
			continue
		}
		for it := range uniq {
			var others []string
			for o := range uniq {
				if o != it {
					others = append(others, o.Video.Rel)
				}
			}
			sort.Strings(others)
			it.Status = StatusConflict
			it.Notes = append(it.Notes, "same target as "+strings.Join(others, ", "))
		}
	}
}

func (e *Engine) buildTarget(it *Item) {
	it.Target, it.Subs, it.Notes = "", nil, nil
	g := it.Group
	if g.Chosen == nil {
		it.Status = StatusUnmatched
		if g.Err != nil {
			it.Notes = append(it.Notes, "TMDB error: "+g.Err.Error())
		} else if g.Query == "" {
			it.Notes = append(it.Notes, "could not find a title in the file or folder names")
		} else {
			it.Notes = append(it.Notes, "no TMDB results")
		}
		return
	}
	c := g.Chosen
	ext := filepath.Ext(it.Video.Path)
	var rel string
	switch c.Kind {
	case media.TV:
		if len(it.Episodes) == 0 {
			it.Status = StatusUnmatched
			it.Notes = append(it.Notes, "no episode number: press e to set one")
			return
		}
		titles := make([]string, len(it.Episodes))
		si := g.Seasons[it.Season]
		switch {
		case si == nil:
			it.Notes = append(it.Notes, fmt.Sprintf("season %d not loaded", it.Season))
		case si.Err != nil:
			it.Notes = append(it.Notes, fmt.Sprintf("season %d not found on TMDB", it.Season))
		default:
			for i, ep := range it.Episodes {
				t, ok := si.Titles[ep]
				if !ok {
					it.Notes = append(it.Notes, fmt.Sprintf("%s not found on TMDB", naming.EpisodeCode(it.Season, []int{ep})))
				}
				titles[i] = t
			}
		}
		rel = naming.EpisodePath(c.Name(), c.Year, it.Season, it.Episodes, titles, ext)
	default:
		rel = naming.MoviePath(c.Name(), c.Year, it.Video.Parsed.Part, ext)
	}
	it.Target = filepath.Join(e.Out, rel)

	switch {
	case g.Confidence >= match.High:
		it.Status = StatusOK
	case g.Confidence == match.Medium:
		it.Status = StatusReview
		it.Notes = append(it.Notes, "medium confidence match: please verify")
	default:
		it.Status = StatusLow
		it.Notes = append(it.Notes, "low confidence match: not selected unless you confirm")
	}
	if it.Status == StatusOK && len(it.Notes) > 0 {
		it.Status = StatusReview
	}

	// Subtitles.
	counts := map[string]int{}
	for _, s := range it.Video.Subs {
		sext := strings.ToLower(filepath.Ext(s.Path))
		key := fmt.Sprintf("%s|%v|%v|%s", s.Lang, s.Forced, s.SDH, sext)
		counts[key]++
		dst := naming.SubtitlePath(it.Target, s.Lang, s.Forced, s.SDH, counts[key], sext)
		op := SubOp{Src: s.Path, Dst: dst}
		if exists(dst) {
			op.Skip = "target exists"
			it.Notes = append(it.Notes, "a subtitle already exists at target and will be left in place")
		}
		it.Subs = append(it.Subs, op)
	}

	// On-disk state of the target.
	if st, err := os.Lstat(it.Target); err == nil {
		if src, err2 := os.Stat(it.Video.Path); err2 == nil && os.SameFile(src, st) {
			it.Status = StatusInPlace
			it.Notes = append(it.Notes, "already at target")
			return
		}
		it.Status = StatusConflict
		it.Notes = append(it.Notes, "target already exists")
	}
}

func exists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

// Op is one file move.
type Op struct {
	Item  *Item
	Src   string
	Dst   string
	IsSub bool
}

// Plan returns the moves to perform: selected items that are ready.
func (e *Engine) Plan() []Op {
	var ops []Op
	for _, it := range e.Items {
		if !it.Applicable() {
			continue
		}
		ops = append(ops, Op{Item: it, Src: it.Video.Path, Dst: it.Target})
		for _, s := range it.Subs {
			if s.Skip == "" {
				ops = append(ops, Op{Item: it, Src: s.Src, Dst: s.Dst, IsSub: true})
			}
		}
	}
	return ops
}

// Applicable reports whether the item will be moved on apply.
func (it *Item) Applicable() bool {
	if !it.Selected || it.Target == "" {
		return false
	}
	switch it.Status {
	case StatusOK, StatusReview, StatusLow:
		return true
	}
	return false
}
