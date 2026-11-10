package parse

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	reBrackets    = regexp.MustCompile(`\[[^\]]*\]|\{[^}]*\}|【[^】]*】`)
	reParen       = regexp.MustCompile(`\(([^)]*)\)`)
	reYearOnly    = regexp.MustCompile(`^(19|20)\d\d$`)
	reSitePrefixA = regexp.MustCompile(`(?i)^\s*(?:www\.[a-z0-9-]+\.[a-z]{2,4}\b\s*-?\s*|[a-z0-9-]+\.(?:com|org|net|to|io|me|tv|cc|ru|se|lol|xyz)\s+-\s+)`)
	reAcronym     = regexp.MustCompile(`\b(?:[A-Za-z]\.){2,}[A-Za-z]?\b\.?`)
	reCodecDots   = regexp.MustCompile(`(?i)\b([hx])\.(26[45])\b`)
	reAudioDots   = regexp.MustCompile(`(?i)\b(ddp?|dd\+|e?ac3|aac|dts|truehd|atmos|flac|opus)[ .]?([0-9])\.([0-9])\b`)
	reWebDL       = regexp.MustCompile(`(?i)\b(web|blu|hd)[ .-]?(dl|rip|ray)\b`)
	reSpaces      = regexp.MustCompile(`\s+`)
)

// normalize turns a raw file or folder name (without extension) into a
// space-separated string with release-group tags and dots removed.
func normalize(s string) string {
	s = reSitePrefixA.ReplaceAllString(s, "")
	s = reBrackets.ReplaceAllString(s, " ")
	s = reParen.ReplaceAllStringFunc(s, func(m string) string {
		inner := strings.TrimSpace(m[1 : len(m)-1])
		if reYearOnly.MatchString(inner) {
			return " " + inner + " "
		}
		return " "
	})
	s = reCodecDots.ReplaceAllString(s, "$1$2")
	s = reAudioDots.ReplaceAllString(s, "$1$2$3")
	s = reWebDL.ReplaceAllString(s, "$1$2")
	s = reAcronym.ReplaceAllStringFunc(s, func(m string) string {
		out := strings.ReplaceAll(m, ".", "")
		if strings.HasSuffix(m, ".") {
			out += " "
		}
		return out
	})
	s = strings.Map(func(r rune) rune {
		if r == '.' || r == '_' {
			return ' '
		}
		return r
	}, s)
	return strings.TrimSpace(reSpaces.ReplaceAllString(s, " "))
}

var noiseExact = map[string]bool{
	"4k": true, "uhd": true, "hdr": true, "hdr10": true, "hdr10+": true, "dv": true, "dovi": true,
	"hevc": true, "avc": true, "xvid": true, "divx": true, "x264": true, "x265": true, "h264": true, "h265": true,
	"webdl": true, "web-dl": true, "webrip": true, "web-rip": true, "bluray": true, "blu-ray": true, "blurip": true,
	"bdrip": true, "brrip": true, "bdremux": true, "hdrip": true, "dvdrip": true, "hdtv": true, "pdtv": true, "dvdscr": true,
	"hdtvrip": true, "remux": true, "proper": true, "repack": true, "rerip": true, "internal": true, "limited": true,
	"extended": true, "unrated": true, "uncut": true, "remastered": true, "imax": true, "criterion": true,
	"multi": true, "dubbed": true, "subbed": true, "vostfr": true, "truefrench": true, "castellano": true, "latino": true,
	"amzn": true, "nf": true, "dsnp": true, "hmax": true, "atvp": true, "pcok": true, "hulu": true,
	"10bit": true, "8bit": true, "12bit": true, "sdr": true, "hdr10plus": true, "dl": true, "dvd": true,
	"720p": true, "1080p": true, "2160p": true, "480p": true, "576p": true, "1080i": true, "720i": true,
	"directors": true, "theatrical": true,
}

var (
	reNoiseRes   = regexp.MustCompile(`^\d{3,4}[pi]$`)
	reNoiseAudio = regexp.MustCompile(`^(ddp?|dd\+|e?ac3|aac|dts(-?hd)?(-?ma)?|truehd|atmos|flac|opus|mp3)\d*$`)
	reNoiseBits  = regexp.MustCompile(`^\d{1,2}bits?$`)
	reNoiseCodec = regexp.MustCompile(`^[hx]26[45]`)
)

func isNoise(tok string) bool {
	t := strings.ToLower(strings.Trim(tok, "-+,;!"))
	if t == "" {
		return false
	}
	if i := strings.IndexByte(t, '-'); i > 0 && !noiseExact[t] {
		// "x264-GROUP" → check "x264"
		if isNoise(t[:i]) {
			return true
		}
	}
	return noiseExact[t] || reNoiseRes.MatchString(t) || reNoiseAudio.MatchString(t) ||
		reNoiseBits.MatchString(t) || reNoiseCodec.MatchString(t)
}

var maxYear = time.Now().Year() + 1

// yearOf returns the year if the token (or its part before a dash) is a plausible year.
func yearOf(tok string) int {
	t := tok
	if i := strings.IndexByte(t, '-'); i > 0 {
		t = t[:i]
	}
	if len(t) != 4 || !reYearOnly.MatchString(t) {
		return 0
	}
	y, _ := strconv.Atoi(t)
	if y < 1900 || y > maxYear {
		return 0
	}
	return y
}

var wordNumbers = map[string]int{
	"zero": 0, "one": 1, "two": 2, "three": 3, "four": 4, "five": 5, "six": 6, "seven": 7, "eight": 8,
	"nine": 9, "ten": 10, "eleven": 11, "twelve": 12, "thirteen": 13, "fourteen": 14, "fifteen": 15,
	"sixteen": 16, "seventeen": 17, "eighteen": 18, "nineteen": 19, "twenty": 20,
	"uno": 1, "dos": 2, "tres": 3, "cuatro": 4, "cinco": 5, "seis": 6, "siete": 7, "ocho": 8, "nueve": 9, "diez": 10,
	"first": 1, "second": 2, "third": 3, "fourth": 4, "fifth": 5, "sixth": 6, "seventh": 7, "eighth": 8, "ninth": 9, "tenth": 10,
	"primera": 1, "segunda": 2, "tercera": 3, "cuarta": 4, "quinta": 5,
}

// genericFolders are directory names that never name a show or movie.
var genericFolders = map[string]bool{
	"tv": true, "tv shows": true, "tvshows": true, "shows": true, "series": true, "tv series": true,
	"movies": true, "movie": true, "films": true, "film": true, "peliculas": true, "películas": true,
	"downloads": true, "download": true, "complete": true, "completed": true, "media": true, "videos": true,
	"video": true, "torrents": true, "torrent": true, "plex": true, "jellyfin": true, "emby": true, "library": true,
	"anime": true, "new": true, "incoming": true, "usenet": true, "sabnzbd": true, "nzbget": true,
	"qbittorrent": true, "transmission": true, "deluge": true, "data": true, "mnt": true, "media library": true,
	"documentaries": true, "kids": true, "temp": true, "tmp": true, "unsorted": true, "to sort": true,
}

// IsGenericFolder reports whether a folder name carries no title information.
func IsGenericFolder(name string) bool {
	return genericFolders[strings.ToLower(strings.TrimSpace(normalize(name)))]
}

var extrasFolders = map[string]bool{
	"extras": true, "extra": true, "featurettes": true, "featurette": true, "behind the scenes": true,
	"deleted scenes": true, "interviews": true, "scenes": true, "shorts": true, "trailers": true,
	"trailer": true, "bonus": true, "special features": true, "sample": true, "samples": true,
	"other": true, "others": true, "making of": true, "promos": true, "bloopers": true,
}

// IsExtrasFolder reports whether a folder holds bonus material rather than main content.
func IsExtrasFolder(name string) bool {
	return extrasFolders[strings.ToLower(strings.TrimSpace(normalize(name)))]
}
