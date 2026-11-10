// Package media holds small types shared across packages.
package media

// Kind is the type of a media item.
type Kind int

const (
	Unknown Kind = iota
	Movie
	TV
)

func (k Kind) String() string {
	switch k {
	case Movie:
		return "movie"
	case TV:
		return "tv"
	}
	return "unknown"
}

// Short returns a one-letter label for compact display.
func (k Kind) Short() string {
	switch k {
	case Movie:
		return "M"
	case TV:
		return "T"
	}
	return "?"
}

var videoExts = map[string]bool{
	".mkv": true, ".mp4": true, ".avi": true, ".m4v": true, ".mov": true,
	".wmv": true, ".mpg": true, ".mpeg": true, ".ts": true, ".m2ts": true,
	".webm": true, ".flv": true, ".ogm": true, ".divx": true, ".3gp": true,
	".vob": true, ".iso": false,
}

var subtitleExts = map[string]bool{
	".srt": true, ".ass": true, ".ssa": true, ".sub": true, ".idx": true,
	".vtt": true, ".sup": true, ".smi": true,
}

// IsVideo reports whether a lower-cased extension (with dot) is a video.
func IsVideo(ext string) bool { return videoExts[ext] }

// IsSubtitle reports whether a lower-cased extension (with dot) is a subtitle.
func IsSubtitle(ext string) bool { return subtitleExts[ext] }
