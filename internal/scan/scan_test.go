package scan

import (
	"os"
	"path/filepath"
	"testing"

	"renamer/internal/media"
)

func touch(t *testing.T, root, rel string, size int64) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	if size > 0 {
		f.Truncate(size)
	}
	f.Close()
}

func TestScan(t *testing.T) {
	root := t.TempDir()
	out := filepath.Join(root, "library")
	files := []string{
		"Inception (2010)/Inception.2010.1080p.mkv",
		"Inception (2010)/Inception.2010.1080p.en.srt",
		"Inception (2010)/Inception.2010.1080p.spa.forced.srt",
		"Inception (2010)/Subs/3_Spanish.srt",
		"Inception (2010)/sample-inception.mkv",
		"Inception (2010)/Featurettes/making.mkv",
		"Breaking Bad/Season 1/01.mkv",
		"Breaking Bad/Season 1/02.mkv",
		"Show.S01.1080p/Show.S01E01.mkv",
		"Show.S01.1080p/Show.S01E02.mkv",
		"Show.S01.1080p/Subs/Show.S01E01/2_English.srt",
		"Show.S01.1080p/Subs/Show.S01E02/2_English.srt",
		"Show.S01.1080p/Subs/Show.S01E02/3_English_SDH.srt",
		"Anime/Some Anime/01 - Start.mkv",
		"Anime/Some Anime/02 - Next.mkv",
		"orphan.srt",
		".hidden/x.mkv",
		"library/Movies/Old (2000)/Old (2000).mkv",
		"notes.txt",
	}
	for _, f := range files {
		touch(t, root, f, 0)
	}
	res, err := Scan(root, out)
	if err != nil {
		t.Fatal(err)
	}
	byRel := map[string]*Video{}
	for _, v := range res.Videos {
		byRel[v.Rel] = v
	}
	if len(res.Videos) != 7 {
		for _, v := range res.Videos {
			t.Log(v.Rel)
		}
		t.Fatalf("got %d videos, want 7", len(res.Videos))
	}
	inc := byRel["Inception (2010)/Inception.2010.1080p.mkv"]
	if inc == nil || len(inc.Subs) != 3 {
		t.Fatalf("inception subs: %+v", inc)
	}
	langs := map[string]bool{}
	for _, s := range inc.Subs {
		key := s.Lang
		if s.Forced {
			key += ".forced"
		}
		langs[key] = true
	}
	for _, k := range []string{"en", "es.forced", "es"} {
		if !langs[k] {
			t.Errorf("missing subtitle %s in %v", k, langs)
		}
	}
	ep2 := byRel["Show.S01.1080p/Show.S01E02.mkv"]
	if len(ep2.Subs) != 2 || !(ep2.Subs[0].SDH || ep2.Subs[1].SDH) {
		t.Errorf("episode 2 subs: %+v", ep2.Subs)
	}
	bb := byRel["Breaking Bad/Season 1/02.mkv"]
	if bb.Parsed.Kind != media.TV || bb.Parsed.Title != "Breaking Bad" || bb.Parsed.Episodes[0] != 2 {
		t.Errorf("breaking bad parsed: %+v", bb.Parsed)
	}
	an := byRel["Anime/Some Anime/02 - Next.mkv"]
	if an.Parsed.Kind != media.TV || an.Parsed.Title != "Some Anime" || an.Parsed.Season != 1 || an.Parsed.Episodes[0] != 2 {
		t.Errorf("anime parsed: %+v", an.Parsed)
	}
	reasons := map[string]string{}
	for _, ig := range res.Ignored {
		reasons[ig.Rel] = ig.Reason
	}
	if reasons["orphan.srt"] == "" || reasons["Inception (2010)/sample-inception.mkv"] != "sample" ||
		reasons["Inception (2010)/Featurettes/making.mkv"] == "" {
		t.Errorf("ignored: %v", reasons)
	}
}

func TestParseSubTags(t *testing.T) {
	cases := []struct {
		in        string
		full      bool
		lang      string
		forced, h bool
	}{
		{".en", false, "en", false, false},
		{".eng.forced", false, "en", true, false},
		{".English.SDH", false, "en", false, true},
		{"2_English", true, "en", false, false},
		{"English (HI)", true, "en", false, true},
		{".pt-BR", false, "pt", false, false},
		{".Castellano", false, "es", false, false},
		{"", false, "", false, false},
	}
	for _, c := range cases {
		l, f, h := parseSubTags(c.in, c.full)
		if l != c.lang || f != c.forced || h != c.h {
			t.Errorf("%q: got %q %v %v", c.in, l, f, h)
		}
	}
}
