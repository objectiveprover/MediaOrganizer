package scan

import (
	"regexp"
	"strings"
)

var langAliases = map[string][]string{
	"en": {"english", "eng", "en"},
	"es": {"spanish", "español", "espanol", "castellano", "latino", "lat", "spa", "esp", "es"},
	"fr": {"french", "français", "francais", "fre", "fra", "fr"},
	"de": {"german", "deutsch", "ger", "deu", "de"},
	"it": {"italian", "italiano", "ita", "it"},
	"pt": {"portuguese", "português", "portugues", "brazilian", "por", "pob", "ptbr", "pt-br", "pt"},
	"nl": {"dutch", "nederlands", "dut", "nld", "nl"},
	"ru": {"russian", "rus", "ru"},
	"ja": {"japanese", "jpn", "ja", "jp"},
	"zh": {"chinese", "mandarin", "chi", "zho", "chs", "cht", "zh"},
	"ko": {"korean", "kor", "ko"},
	"ar": {"arabic", "ara", "ar"},
	"sv": {"swedish", "swe", "sv"},
	"no": {"norwegian", "nor", "nob", "no"},
	"da": {"danish", "dan", "da"},
	"fi": {"finnish", "fin", "fi"},
	"pl": {"polish", "pol", "pl"},
	"tr": {"turkish", "tur", "tr"},
	"el": {"greek", "gre", "ell", "el"},
	"he": {"hebrew", "heb", "he"},
	"hu": {"hungarian", "hun", "hu"},
	"cs": {"czech", "cze", "ces", "cs"},
	"ro": {"romanian", "rum", "ron", "ro"},
	"bg": {"bulgarian", "bul", "bg"},
	"hr": {"croatian", "hrv", "hr"},
	"sr": {"serbian", "srp", "sr"},
	"sk": {"slovak", "slo", "slk", "sk"},
	"sl": {"slovenian", "slv", "sl"},
	"uk": {"ukrainian", "ukr", "uk"},
	"hi": {"hindi", "hin"},
	"th": {"thai", "tha", "th"},
	"vi": {"vietnamese", "vie", "vi"},
	"id": {"indonesian", "ind", "id"},
	"ms": {"malay", "may", "msa", "ms"},
	"fa": {"persian", "farsi", "per", "fas", "fa"},
	"ca": {"catalan", "cat", "ca"},
	"eu": {"basque", "baq", "eus", "eu"},
	"gl": {"galician", "glg", "gl"},
	"et": {"estonian", "est", "et"},
	"lv": {"latvian", "lav", "lv"},
	"lt": {"lithuanian", "lit", "lt"},
	"is": {"icelandic", "ice", "isl"},
}

var langLookup = func() map[string]string {
	m := map[string]string{}
	for code, names := range langAliases {
		for _, n := range names {
			m[n] = code
		}
	}
	return m
}()

var reTagSplit = regexp.MustCompile(`[\s._\-()\[\]]+`)

// parseSubTags extracts a language code and flags (forced, sdh) from the
// part of a subtitle name that follows the video name. If fullName is true,
// the text is a whole subtitle name and only its last tokens are considered.
func parseSubTags(s string, fullName bool) (lang string, forced, sdh bool) {
	toks := reTagSplit.Split(strings.ToLower(s), -1)
	if fullName && len(toks) > 3 {
		toks = toks[len(toks)-3:]
	}
	sawHI := false
	for i := len(toks) - 1; i >= 0; i-- {
		t := toks[i]
		switch t {
		case "":
			continue
		case "forced", "foreign":
			forced = true
			continue
		case "sdh", "cc":
			sdh = true
			continue
		case "hi":
			sawHI = true
			continue
		}
		// "pt-br" was split; rejoin.
		if lang == "" {
			if c, ok := langLookup[t]; ok {
				lang = c
			}
		}
	}
	if sawHI {
		if lang == "" {
			lang = "hi"
		} else {
			sdh = true
		}
	}
	return lang, forced, sdh
}
