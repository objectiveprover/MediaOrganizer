// Package naming builds Plex/Jellyfin-style target paths.
package naming

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// IsLatin reports whether every letter in s is from the Latin script.
func IsLatin(s string) bool {
	hasLetter := false
	for _, r := range s {
		if unicode.IsLetter(r) {
			hasLetter = true
			if !unicode.Is(unicode.Latin, r) {
				return false
			}
		}
	}
	return hasLetter || s != ""
}

// DisplayTitle picks the original title when it's in Latin script,
// otherwise the English title.
func DisplayTitle(original, english string) string {
	if original != "" && IsLatin(original) {
		return original
	}
	if english != "" {
		return english
	}
	return original
}

var reGenericEpisode = regexp.MustCompile(`(?i)^\s*(episode|episodio|épisode|episódio|cap[ií]tulo|folge|aflevering|odcinek|avsnitt|jakso|bölüm|epizod[ae]?|серия|эпизод|ep\.?)\s*#?\s*\d+\s*$`)

// IsGenericEpisodeName reports whether a TMDB episode name is a placeholder
// like "Episode 5" (what TMDB returns when there's no translation).
func IsGenericEpisodeName(s string) bool {
	return strings.TrimSpace(s) == "" || reGenericEpisode.MatchString(s)
}

var (
	reColon   = regexp.MustCompile(`\s*:\s*`)
	reBad     = regexp.MustCompile(`[<>"\\|?*\x00-\x1f]`)
	reSpaces  = regexp.MustCompile(`\s+`)
	reTrimEnd = regexp.MustCompile(`[\s.]+$`)
)

// Sanitize makes s safe as a single path component on Linux, Windows and SMB shares.
func Sanitize(s string) string {
	s = reColon.ReplaceAllString(s, " - ")
	s = strings.ReplaceAll(s, "/", "-")
	s = reBad.ReplaceAllString(s, "")
	s = reSpaces.ReplaceAllString(s, " ")
	s = strings.TrimSpace(s)
	s = reTrimEnd.ReplaceAllString(s, "")
	s = strings.TrimLeft(s, ". ")
	if s == "" {
		s = "Unknown"
	}
	return s
}

// WithYear returns "Title (Year)" or just "Title" when year is 0.
func WithYear(title string, year int) string {
	t := Sanitize(title)
	if year > 0 {
		return fmt.Sprintf("%s (%d)", t, year)
	}
	return t
}

const maxNameBytes = 250

// truncateBytes cuts s to at most n bytes without splitting a rune.
func truncateBytes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	s = s[:n]
	for !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return strings.TrimSpace(s)
}

// MoviePath returns the path relative to the output root.
func MoviePath(title string, year, part int, ext string) string {
	name := WithYear(title, year)
	file := name
	if part > 0 {
		file += fmt.Sprintf(" - part%d", part)
	}
	return filepath.Join("Movies", name, file+strings.ToLower(ext))
}

// EpisodeCode formats S01E02 or S01E02-E03.
func EpisodeCode(season int, eps []int) string {
	if len(eps) == 0 {
		return fmt.Sprintf("S%02d", season)
	}
	code := fmt.Sprintf("S%02dE%02d", season, eps[0])
	if len(eps) > 1 {
		code += fmt.Sprintf("-E%02d", eps[len(eps)-1])
	}
	return code
}

// EpisodePath returns the path relative to the output root. titles may
// contain empty strings for unknown episode names.
func EpisodePath(show string, year, season int, eps []int, titles []string, ext string) string {
	showDir := WithYear(show, year)
	seasonDir := fmt.Sprintf("Season %02d", season)
	base := showDir + " - " + EpisodeCode(season, eps)
	var uniq []string
	seen := map[string]bool{}
	for _, t := range titles {
		t = Sanitize(t)
		if strings.TrimSpace(t) == "" || t == "Unknown" || seen[t] {
			continue
		}
		seen[t] = true
		uniq = append(uniq, t)
	}
	if len(uniq) > 0 {
		et := strings.Join(uniq, " & ")
		room := maxNameBytes - len(base) - len(" - ") - len(ext) - 20 // leave room for subtitle suffixes
		if room > 10 {
			base += " - " + truncateBytes(et, room)
		}
	}
	return filepath.Join("TV", showDir, seasonDir, base+strings.ToLower(ext))
}

// SubtitlePath derives a subtitle path from its video's target path.
// counter > 1 is appended to disambiguate subtitles with the same tags.
func SubtitlePath(videoTarget, lang string, forced, sdh bool, counter int, ext string) string {
	base := strings.TrimSuffix(videoTarget, filepath.Ext(videoTarget))
	if lang != "" {
		base += "." + lang
	}
	if counter > 1 {
		base += fmt.Sprintf(".%d", counter)
	}
	if sdh {
		base += ".sdh"
	}
	if forced {
		base += ".forced"
	}
	return base + strings.ToLower(ext)
}
