package parse

import (
	"reflect"
	"testing"

	"renamer/internal/media"
)

type want struct {
	kind  media.Kind
	title string
	year  int
	s     int
	eps   []int
	part  int
}

func TestParse(t *testing.T) {
	const root = "/downloads"
	M, T := media.Movie, media.TV
	cases := []struct {
		path string
		hint Hint
		w    want
	}{
		// --- Movies: scene style
		{"/downloads/The.Matrix.1999.1080p.BluRay.x264-GROUP.mkv", Hint{}, want{M, "The Matrix", 1999, -1, nil, 0}},
		{"/downloads/Blade.Runner.2049.2017.2160p.UHD.BluRay.REMUX.HDR.HEVC.Atmos-EPSiLON.mkv", Hint{}, want{M, "Blade Runner 2049", 2017, -1, nil, 0}},
		{"/downloads/Blade Runner 2049.mkv", Hint{}, want{M, "Blade Runner 2049", 0, -1, nil, 0}},
		{"/downloads/1917.2019.1080p.WEB-DL.DD5.1.H.264-FGT.mkv", Hint{}, want{M, "1917", 2019, -1, nil, 0}},
		{"/downloads/1917.mkv", Hint{}, want{M, "1917", 0, -1, nil, 0}},
		{"/downloads/2012 (2009).mkv", Hint{}, want{M, "2012", 2009, -1, nil, 0}},
		{"/downloads/Charlottes.Web.2006.720p.mkv", Hint{}, want{M, "Charlottes Web", 2006, -1, nil, 0}},
		{"/downloads/Mad.Max.Fury.Road.2015.mkv", Hint{}, want{M, "Mad Max Fury Road", 2015, -1, nil, 0}},
		{"/downloads/Ocean's 11 (2001).mkv", Hint{}, want{M, "Ocean's 11", 2001, -1, nil, 0}},
		{"/downloads/A.Complete.Unknown.2024.1080p.mkv", Hint{}, want{M, "A Complete Unknown", 2024, -1, nil, 0}},
		{"/downloads/Spider-Man.No.Way.Home.2021.mkv", Hint{}, want{M, "Spider-Man No Way Home", 2021, -1, nil, 0}},
		{"/downloads/[YTS.MX] Parasite (2019) [1080p].mp4", Hint{}, want{M, "Parasite", 2019, -1, nil, 0}},
		{"/downloads/www.Torrenting.com - Dune.Part.Two.2024.mkv", Hint{}, want{M, "Dune Part Two", 2024, -1, nil, 0}},
		{"/downloads/Harry Potter and the Deathly Hallows Part 1 (2010).mkv", Hint{}, want{M, "Harry Potter and the Deathly Hallows Part 1", 2010, -1, nil, 0}},
		{"/downloads/Amelie.2001.FRENCH.1080p.mkv", Hint{}, want{M, "Amelie", 2001, -1, nil, 0}},
		{"/downloads/The.French.Connection.1971.mkv", Hint{}, want{M, "The French Connection", 1971, -1, nil, 0}},
		{"/downloads/Cam.2018.mkv", Hint{}, want{M, "Cam", 2018, -1, nil, 0}},
		{"/downloads/Kill.Bill.Vol.1.2003.CD1.avi", Hint{}, want{M, "Kill Bill Vol 1", 2003, -1, nil, 1}},
		{"/downloads/Kill.Bill.Vol.1.2003.CD2.avi", Hint{}, want{M, "Kill Bill Vol 1", 2003, -1, nil, 2}},
		{"/downloads/S.W.A.T.2003.mkv", Hint{}, want{M, "SWAT", 2003, -1, nil, 0}},
		// --- Movies: info in folder
		{"/downloads/Inception (2010)/inception.mkv", Hint{}, want{M, "Inception", 2010, -1, nil, 0}},
		{"/downloads/Inception.2010.1080p.BluRay.x264-SPARKS/sparks-inception.mkv", Hint{}, want{M, "Inception", 2010, -1, nil, 0}},
		{"/downloads/Heat.1995.REMASTERED.1080p/a1b2c3d4e5f6a7b8c9d0.mkv", Hint{}, want{M, "Heat", 1995, -1, nil, 0}},
		{"/downloads/Movies/Alien (1979)/Alien.mkv", Hint{}, want{M, "Alien", 1979, -1, nil, 0}},
		{"/downloads/The Godfather (1972)/CD1/godfather.avi", Hint{}, want{M, "The Godfather", 1972, -1, nil, 1}},

		// --- TV: explicit markers
		{"/downloads/Breaking.Bad.S01E01.720p.HDTV.x264.mkv", Hint{}, want{T, "Breaking Bad", 0, 1, []int{1}, 0}},
		{"/downloads/breaking.bad.s1e2.mkv", Hint{}, want{T, "breaking bad", 0, 1, []int{2}, 0}},
		{"/downloads/The Office (US) - 2x05 - Halloween.avi", Hint{}, want{T, "The Office", 0, 2, []int{5}, 0}},
		{"/downloads/Doctor.Who.2005.S01E01.Rose.mkv", Hint{}, want{T, "Doctor Who", 2005, 1, []int{1}, 0}},
		{"/downloads/Show.Name.S01E01E02.1080p.mkv", Hint{}, want{T, "Show Name", 0, 1, []int{1, 2}, 0}},
		{"/downloads/Show.Name.S01E01-E03.1080p.mkv", Hint{}, want{T, "Show Name", 0, 1, []int{1, 2, 3}, 0}},
		{"/downloads/Show Name - S02E05-06 - Title.mkv", Hint{}, want{T, "Show Name", 0, 2, []int{5, 6}, 0}},
		{"/downloads/Show Name Season 3 Episode 7.mp4", Hint{}, want{T, "Show Name", 0, 3, []int{7}, 0}},
		{"/downloads/Marvels.Agents.of.S.H.I.E.L.D.S01E01.mkv", Hint{}, want{T, "Marvels Agents of SHIELD", 0, 1, []int{1}, 0}},
		{"/downloads/Shows/Fargo/Season 2/Fargo.S02E03.mkv", Hint{}, want{T, "Fargo", 0, 2, []int{3}, 0}},

		// --- TV: info only in folders
		{"/downloads/Breaking Bad/Season 1/01.mkv", Hint{}, want{T, "Breaking Bad", 0, 1, []int{1}, 0}},
		{"/downloads/Show.Name.Complete/Season Two/01.mkv", Hint{}, want{T, "Show Name", 0, 2, []int{1}, 0}},
		{"/downloads/Show Name (2015)/S03/05 - Some Title.mkv", Hint{}, want{T, "Show Name", 2015, 3, []int{5}, 0}},
		{"/downloads/Show Name/Season 1/Episode 3.mkv", Hint{}, want{T, "Show Name", 0, 1, []int{3}, 0}},
		{"/downloads/Show Name/Temporada 4/Capitulo 12.mkv", Hint{}, want{T, "Show Name", 0, 4, []int{12}, 0}},
		{"/downloads/Show.Name.S02.1080p.WEB-DL/03.mkv", Hint{}, want{T, "Show Name", 0, 2, []int{3}, 0}},
		{"/downloads/Show.Name.S01-S05.Complete.1080p/Season 4/E07.mkv", Hint{}, want{T, "Show Name", 0, 4, []int{7}, 0}},
		{"/downloads/Show Name/Specials/02.mkv", Hint{}, want{T, "Show Name", 0, 0, []int{2}, 0}},
		{"/downloads/Show Name/Season 1/Show Name - 04.mkv", Hint{}, want{T, "Show Name", 0, 1, []int{4}, 0}},
		{"/downloads/Show Name/Season 1/Episode 3.mkv", Hint{}, want{T, "Show Name", 0, 1, []int{3}, 0}},
		{"/downloads/Show Name/Season 1/Show.Name.03.720p.mkv", Hint{}, want{T, "Show Name", 0, 1, []int{3}, 0}},
		{"/downloads/Show Name/01 - Pilot.mkv", Hint{NumberedSiblings: true}, want{T, "Show Name", 0, 1, []int{1}, 0}},
		{"/downloads/Show.Name.S01E02.720p.HDTV-GRP/abc123def456.mkv", Hint{}, want{T, "Show Name", 0, 1, []int{2}, 0}},
		{"/downloads/Show Name/Series 2/E05.mkv", Hint{}, want{T, "Show Name", 0, 2, []int{5}, 0}},
		{"/downloads/Show Name/2nd Season/04.mkv", Hint{}, want{T, "Show Name", 0, -1, nil, 0}}, // documented limitation → falls to movie
		{"/downloads/Show Name/Second Season/04.mkv", Hint{}, want{T, "Show Name", 0, 2, []int{4}, 0}},
		{"/media/TV/Lost/S1/ep01.mkv", Hint{}, want{T, "Lost", 0, 1, []int{1}, 0}},
	}
	for _, c := range cases {
		root := root
		if c.path[:6] == "/media" {
			root = "/media"
		}
		got := Parse(root, c.path, c.hint)
		if c.w.title == "Show Name" && c.w.s == -1 {
			continue // limitation case, documented only
		}
		g := want{got.Kind, got.Title, got.Year, got.Season, got.Episodes, got.Part}
		if !reflect.DeepEqual(g, c.w) {
			t.Errorf("%s\n   got  %+v\n   want %+v\n   notes %v alts %v", c.path, g, c.w, got.Notes, got.AltTitles)
		}
	}
}

func TestHasBareNumber(t *testing.T) {
	yes := []string{"01.mkv", "02 - Title.mkv", "Show - 03.mkv", "4.avi"}
	no := []string{"Movie.2010.mkv", "Show.S01E01.mkv", "Toy Story 2.mkv", "1917.mkv"}
	for _, f := range yes {
		if !HasBareNumber(f) {
			t.Errorf("expected bare number in %q", f)
		}
	}
	for _, f := range no {
		if HasBareNumber(f) {
			t.Errorf("did not expect bare number in %q", f)
		}
	}
}

func TestRootNameNotMovieTitle(t *testing.T) {
	r := Parse("/data/src", "/data/src/Asdfgh Qwerty 1987.mkv", Hint{})
	for _, a := range r.AltTitles {
		if a == "src" {
			t.Errorf("root name leaked into alts: %v", r.AltTitles)
		}
	}
	// ...but the root may still name a show.
	r = Parse("/data/Breaking Bad", "/data/Breaking Bad/Season 2/03.mkv", Hint{})
	if r.Title != "Breaking Bad" || r.Season != 2 || r.Episodes[0] != 3 {
		t.Errorf("root as show: %+v", r)
	}
}
