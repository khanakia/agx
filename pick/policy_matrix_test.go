package pick

import (
	"errors"
	"strings"
	"testing"

	"github.com/khanakia/agx/provider"
)

func apiCand(name string, err error, pcts ...float64) Candidate {
	c := cand(name, err, pcts...)
	c.Profile.Billing = provider.BillingAPI
	return c
}

// TestBest_Matrix crosses window count × ties × exhaustion × skips: the
// chosen profile, its score, the order of the others and who was skipped.
func TestBest_Matrix(t *testing.T) {
	t.Parallel()
	errExpired := errors.New("expired")
	for _, tc := range []struct {
		name    string
		cands   []Candidate
		chosen  string
		score   float64
		others  string // names, best first
		skipped string // names, sorted by input order
	}{
		{"single", []Candidate{cand("only", nil, 42)}, "only", 42, "", ""},
		{"single exhausted still chosen", []Candidate{cand("only", nil, 100)}, "only", 100, "", ""},
		{"all exhausted: config order", []Candidate{cand("a", nil, 100, 100), cand("b", nil, 100)}, "a", 100, "b", ""},
		{"over 100 counts as worse", []Candidate{cand("a", nil, 130), cand("b", nil, 100)}, "b", 100, "a", ""},
		{"last wins when strictly lower", []Candidate{cand("a", nil, 3), cand("b", nil, 2), cand("c", nil, 1)}, "c", 1, "b,a", ""},
		{"tie among later keeps order", []Candidate{cand("a", nil, 90), cand("b", nil, 5), cand("c", nil, 5)}, "b", 5, "c,a", ""},
		{"others ties keep order", []Candidate{cand("a", nil, 1), cand("b", nil, 50), cand("c", nil, 50), cand("d", nil, 20)}, "a", 1, "d,b,c", ""},
		{"many windows: max decides", []Candidate{cand("a", nil, 0, 0, 99), cand("b", nil, 50, 50, 50)}, "b", 50, "a", ""},
		{"zero windows ties zero windows", []Candidate{cand("a", nil), cand("b", nil)}, "a", 0, "b", ""},
		{"errored and api skipped", []Candidate{cand("e", errExpired, 0), apiCand("k", nil, 0), cand("w", nil, 99)}, "w", 99, "", "e,k"},
		{"unicode names", []Candidate{cand("wörk", nil, 7), cand("日本", nil, 3)}, "日本", 3, "wörk", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			res, err := Best(tc.cands)
			if err != nil {
				t.Fatal(err)
			}
			if res.Chosen.Name != tc.chosen || res.Score != tc.score {
				t.Errorf("chose %s (%v), want %s (%v)", res.Chosen.Name, res.Score, tc.chosen, tc.score)
			}
			var others []string
			for _, o := range res.Others {
				others = append(others, o.Profile.Name)
			}
			if strings.Join(others, ",") != tc.others {
				t.Errorf("others %v, want %s", others, tc.others)
			}
			var skipped []string
			for _, c := range tc.cands {
				if _, ok := res.Skipped[c.Profile.Name]; ok {
					skipped = append(skipped, c.Profile.Name)
				}
			}
			if strings.Join(skipped, ",") != tc.skipped || len(res.Skipped) != len(skipped) {
				t.Errorf("skipped %v, want %s", res.Skipped, tc.skipped)
			}
		})
	}
}

// TestBest_ErrorBeatsAPIReason: a fetch error is the reported reason even
// for an api-billed profile, and an all-skipped set lists every reason in
// input order.
func TestBest_ErrorBeatsAPIReason(t *testing.T) {
	t.Parallel()
	errOffline := errors.New("offline")
	_, err := Best([]Candidate{apiCand("k", errOffline), apiCand("j", nil), cand("p", errors.New("429"))})
	if !errors.Is(err, ErrNoCandidate) {
		t.Fatalf("err = %v", err)
	}
	want := "k: offline; j: " + provider.ErrAPIBilled.Error() + "; p: 429"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("err = %v, want it to contain %q", err, want)
	}
	if reasons(nil, map[string]error{}) != "no profiles" {
		t.Error("empty reasons")
	}
}

// TestExplain_Rounding: percents render as whole numbers.
func TestExplain_Rounding(t *testing.T) {
	t.Parallel()
	res, err := Best([]Candidate{cand("a", nil, 4.6), cand("b", nil, 99.4)})
	if err != nil {
		t.Fatal(err)
	}
	if got := res.Explain(); got != "auto → a (max 5%) over b (max 99%)" {
		t.Errorf("Explain = %q", got)
	}
}
