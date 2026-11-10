package naming

import "testing"

func TestDisplayTitle(t *testing.T) {
	cases := [][3]string{
		{"千と千尋の神隠し", "Spirited Away", "Spirited Away"},
		{"Le Fabuleux Destin d'Amélie Poulain", "Amélie", "Le Fabuleux Destin d'Amélie Poulain"},
		{"Dark", "Dark", "Dark"},
		{"Мастер и Маргарита", "The Master and Margarita", "The Master and Margarita"},
		{"1917", "1917", "1917"},
	}
	for _, c := range cases {
		if got := DisplayTitle(c[0], c[1]); got != c[2] {
			t.Errorf("DisplayTitle(%q,%q)=%q want %q", c[0], c[1], got, c[2])
		}
	}
}

func TestPaths(t *testing.T) {
	if got := MoviePath("Mission: Impossible", 1996, 0, ".MKV"); got != "Movies/Mission - Impossible (1996)/Mission - Impossible (1996).mkv" {
		t.Error(got)
	}
	if got := MoviePath("Kill Bill: Vol. 1", 2003, 2, ".avi"); got != "Movies/Kill Bill - Vol. 1 (2003)/Kill Bill - Vol. 1 (2003) - part2.avi" {
		t.Error(got)
	}
	if got := MoviePath("Face/Off", 1997, 0, ".mkv"); got != "Movies/Face-Off (1997)/Face-Off (1997).mkv" {
		t.Error(got)
	}
	if got := EpisodePath("Breaking Bad", 2008, 1, []int{1}, []string{"Pilot"}, ".mkv"); got != "TV/Breaking Bad (2008)/Season 01/Breaking Bad (2008) - S01E01 - Pilot.mkv" {
		t.Error(got)
	}
	if got := EpisodePath("Show", 2010, 1, []int{1, 2}, []string{"A", "B"}, ".mkv"); got != "TV/Show (2010)/Season 01/Show (2010) - S01E01-E02 - A & B.mkv" {
		t.Error(got)
	}
	if got := EpisodePath("Show", 2010, 0, []int{3}, []string{""}, ".mkv"); got != "TV/Show (2010)/Season 00/Show (2010) - S00E03.mkv" {
		t.Error(got)
	}
	if got := EpisodePath("Show", 2010, 1, []int{1, 2}, []string{"Part 1", "Part 1"}, ".mkv"); got != "TV/Show (2010)/Season 01/Show (2010) - S01E01-E02 - Part 1.mkv" {
		t.Error(got)
	}
	if got := SubtitlePath("Movies/X (2000)/X (2000).mkv", "es", true, false, 0, ".SRT"); got != "Movies/X (2000)/X (2000).es.forced.srt" {
		t.Error(got)
	}
}

func TestGenericEpisodeName(t *testing.T) {
	for _, s := range []string{"Episode 1", "Episodio 12", "Folge 3", ""} {
		if !IsGenericEpisodeName(s) {
			t.Errorf("%q should be generic", s)
		}
	}
	if IsGenericEpisodeName("Pilot") || IsGenericEpisodeName("Episode 1: The Beginning") {
		t.Error("real names flagged generic")
	}
}
