// Package scan walks a source directory, finds videos and subtitles,
// pairs them up and parses each video.
package scan

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"renamer/internal/media"
	"renamer/internal/parse"
)

// Subtitle is a subtitle file attached to a video.
type Subtitle struct {
	Path   string
	Lang   string // ISO 639-1, "" if unknown
	Forced bool
	SDH    bool
}

// Video is one video file found by the scan.
type Video struct {
	Path   string
	Rel    string // relative to the scan root
	Size   int64
	Parsed parse.Result
	Subs   []Subtitle
}

// Ignored is a file deliberately skipped.
type Ignored struct {
	Rel    string
	Reason string
}

// Result holds everything found under Root.
type Result struct {
	Root    string
	Videos  []*Video
	Ignored []Ignored
}

const sampleMaxSize = 300 << 20

var (
	reSample  = regexp.MustCompile(`(?i)(^|[\s._\-\[(])sample([\s._\-\])]|$)`)
	subsDirRe = regexp.MustCompile(`(?i)^(subs?|subtitles?|subtitulos|subtítulos|legendas)$`)
)

// Scan walks root. Any path in exclude (e.g. the output directory) is skipped.
func Scan(root string, exclude ...string) (*Result, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	excl := map[string]bool{}
	for _, e := range exclude {
		if a, err := filepath.Abs(e); err == nil {
			excl[a] = true
		}
	}
	res := &Result{Root: root}
	var subs []string
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			rel, _ := filepath.Rel(root, path)
			res.Ignored = append(res.Ignored, Ignored{rel, "unreadable: " + err.Error()})
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if path == root {
			return nil
		}
		name := d.Name()
		rel, _ := filepath.Rel(root, path)
		if d.IsDir() {
			if strings.HasPrefix(name, ".") || excl[path] {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(name, ".") {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(name))
		switch {
		case media.IsVideo(ext):
			if d.Type()&fs.ModeSymlink != 0 {
				res.Ignored = append(res.Ignored, Ignored{rel, "symlink"})
				return nil
			}
			info, err := d.Info()
			if err != nil {
				res.Ignored = append(res.Ignored, Ignored{rel, "stat failed: " + err.Error()})
				return nil
			}
			if inExtras(rel) {
				res.Ignored = append(res.Ignored, Ignored{rel, "extras/sample folder"})
				return nil
			}
			if reSample.MatchString(strings.TrimSuffix(name, filepath.Ext(name))) && info.Size() < sampleMaxSize {
				res.Ignored = append(res.Ignored, Ignored{rel, "sample"})
				return nil
			}
			res.Videos = append(res.Videos, &Video{Path: path, Rel: rel, Size: info.Size()})
		case media.IsSubtitle(ext):
			if d.Type()&fs.ModeSymlink == 0 {
				subs = append(subs, path)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(res.Videos, func(i, j int) bool { return res.Videos[i].Rel < res.Videos[j].Rel })

	// Directory-level hint: are most videos numbered?
	byDir := map[string][]*Video{}
	for _, v := range res.Videos {
		byDir[filepath.Dir(v.Path)] = append(byDir[filepath.Dir(v.Path)], v)
	}
	for _, vs := range byDir {
		n := 0
		for _, v := range vs {
			if parse.HasBareNumber(filepath.Base(v.Path)) {
				n++
			}
		}
		hint := parse.Hint{NumberedSiblings: n >= 2 && n*2 >= len(vs)}
		for _, v := range vs {
			v.Parsed = parse.Parse(root, v.Path, hint)
		}
	}

	for _, s := range subs {
		if !attachSubtitle(s, byDir) {
			rel, _ := filepath.Rel(root, s)
			res.Ignored = append(res.Ignored, Ignored{rel, "subtitle with no matching video"})
		}
	}
	sort.Slice(res.Ignored, func(i, j int) bool { return res.Ignored[i].Rel < res.Ignored[j].Rel })
	return res, nil
}

func inExtras(rel string) bool {
	parts := strings.Split(filepath.ToSlash(filepath.Dir(rel)), "/")
	for _, p := range parts {
		if p != "." && parse.IsExtrasFolder(p) {
			return true
		}
	}
	return false
}

func baseNoExt(p string) string {
	b := filepath.Base(p)
	return strings.TrimSuffix(b, filepath.Ext(b))
}

// matchPrefix finds the video in vs whose base name is the longest prefix of
// subBase, returning it and the remaining tag text.
func matchPrefix(subBase string, vs []*Video) (*Video, string) {
	var best *Video
	bestLen := -1
	lsb := strings.ToLower(subBase)
	for _, v := range vs {
		vb := strings.ToLower(baseNoExt(v.Path))
		if lsb == vb || (strings.HasPrefix(lsb, vb) && len(lsb) > len(vb) && strings.ContainsRune("._- ", rune(lsb[len(vb)]))) {
			if len(vb) > bestLen {
				best, bestLen = v, len(vb)
			}
		}
	}
	if best == nil {
		return nil, ""
	}
	return best, subBase[bestLen:]
}

func attachSubtitle(path string, byDir map[string][]*Video) bool {
	dir := filepath.Dir(path)
	sb := baseNoExt(path)
	attach := func(v *Video, tags string, full bool) bool {
		lang, forced, sdh := parseSubTags(tags, full)
		v.Subs = append(v.Subs, Subtitle{Path: path, Lang: lang, Forced: forced, SDH: sdh})
		return true
	}

	// 1. Same directory, name starts with the video's name.
	if v, rest := matchPrefix(sb, byDir[dir]); v != nil {
		return attach(v, rest, false)
	}

	// 2. Subs/ folder next to the video, or Subs/<video name>/x.srt.
	if subsDirRe.MatchString(filepath.Base(dir)) {
		vdir := filepath.Dir(dir)
		if v, rest := matchPrefix(sb, byDir[vdir]); v != nil {
			return attach(v, rest, false)
		}
		if len(byDir[vdir]) == 1 {
			return attach(byDir[vdir][0], sb, true)
		}
	}
	if parent := filepath.Dir(dir); subsDirRe.MatchString(filepath.Base(parent)) {
		vdir := filepath.Dir(parent)
		epName := strings.ToLower(filepath.Base(dir))
		for _, v := range byDir[vdir] {
			if strings.ToLower(baseNoExt(v.Path)) == epName {
				return attach(v, sb, true)
			}
		}
	}

	// 3. Only one video in the directory.
	if vs := byDir[dir]; len(vs) == 1 {
		return attach(vs[0], sb, true)
	}
	return false
}

// Exists is a small helper used by callers and tests.
func Exists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}
