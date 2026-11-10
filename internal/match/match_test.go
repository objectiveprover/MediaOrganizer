package match

import "testing"

func TestSimilarity(t *testing.T) {
	hi := [][2]string{
		{"The Matrix", "Matrix"},
		{"Amelie", "Amélie"},
		{"Oceans 11", "Ocean's 11"},
		{"Spiderman No Way Home", "Spider-Man: No Way Home"},
		{"Marvels Agents of SHIELD", "Marvel's Agents of S.H.I.E.L.D."},
		{"Fast and Furious", "Fast & Furious"},
	}
	for _, p := range hi {
		if s := Similarity(p[0], p[1]); s < 0.9 {
			t.Errorf("Similarity(%q,%q)=%.2f want >=0.9", p[0], p[1], s)
		}
	}
	if s := Similarity("The Matrix", "Inception"); s > 0.4 {
		t.Errorf("unrelated too similar: %.2f", s)
	}
}

func TestRankAndAssess(t *testing.T) {
	cands := []Candidate{
		{ID: 1, Title: "Dune", OriginalTitle: "Dune", Year: 1984, Popularity: 30},
		{ID: 2, Title: "Dune", OriginalTitle: "Dune", Year: 2021, Popularity: 90},
		{ID: 3, Title: "Dune: Part Two", OriginalTitle: "Dune: Part Two", Year: 2024, Popularity: 100},
	}
	r := Rank([]string{"Dune"}, 1984, cands)
	if r[0].ID != 1 || Assess(r, 1984) != High {
		t.Errorf("year should pick 1984: %+v %v", r[0], Assess(r, 1984))
	}
	r = Rank([]string{"Dune"}, 0, cands)
	if r[0].ID != 2 {
		t.Errorf("popularity tiebreak: %+v", r[0])
	}
	if c := Assess(r, 0); c != Medium {
		t.Errorf("ambiguous without year should be medium, got %v", c)
	}
	r = Rank([]string{"Completely Different"}, 0, cands)
	if c := Assess(r, 0); c != Low {
		t.Errorf("want low, got %v", c)
	}
	if Assess(nil, 0) != None {
		t.Error("want none")
	}
	one := Rank([]string{"Inception"}, 2010, []Candidate{{ID: 9, Title: "Inception", OriginalTitle: "Inception", Year: 2010}})
	if Assess(one, 2010) != High {
		t.Error("single exact match should be high")
	}
}
