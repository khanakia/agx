package pick

import (
	"errors"
	"strings"
	"testing"

	"github.com/khanakia/agx/provider"
)

func cand(name string, err error, pcts ...float64) Candidate {
	u := provider.Usage{}
	for _, p := range pcts {
		u.Windows = append(u.Windows, provider.Window{Percent: p})
	}
	return Candidate{Profile: provider.Profile{Name: name, Billing: provider.BillingPlan}, Usage: u, Err: err}
}

func TestBest(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		cands []Candidate
		want  string
		score float64
	}{
		// Score is the MAX window: personal's 77% weekly beats its 5% session.
		{"highest window decides", []Candidate{cand("personal", nil, 5, 77), cand("work", nil, 9, 3)}, "work", 9},
		{"tie keeps config order", []Candidate{cand("a", nil, 10), cand("b", nil, 10)}, "a", 10},
		{"errored candidate skipped", []Candidate{cand("a", errors.New("expired"), 0), cand("b", nil, 50)}, "b", 50},
		{"no windows scores zero", []Candidate{cand("a", nil, 1), cand("b", nil)}, "b", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			res, err := Best(tc.cands)
			if err != nil {
				t.Fatal(err)
			}
			if res.Chosen.Name != tc.want || res.Score != tc.score {
				t.Errorf("chose %s (%v), want %s (%v)", res.Chosen.Name, res.Score, tc.want, tc.score)
			}
		})
	}
}

func TestBest_APIBilledNeverChosen(t *testing.T) {
	t.Parallel()
	api := cand("kimi", nil)
	api.Profile.Billing = provider.BillingAPI
	res, err := Best([]Candidate{api, cand("work", nil, 90)})
	if err != nil || res.Chosen.Name != "work" {
		t.Fatalf("chose %+v, %v", res.Chosen, err)
	}
	if !errors.Is(res.Skipped["kimi"], provider.ErrAPIBilled) {
		t.Errorf("skipped = %v", res.Skipped)
	}
}

func TestBest_NoCandidate(t *testing.T) {
	t.Parallel()
	_, err := Best([]Candidate{cand("a", errors.New("expired")), cand("b", errors.New("offline"))})
	if !errors.Is(err, ErrNoCandidate) || !strings.Contains(err.Error(), "a: expired; b: offline") {
		t.Errorf("err = %v", err)
	}
	if _, err := Best(nil); !errors.Is(err, ErrNoCandidate) {
		t.Errorf("empty err = %v", err)
	}
}

func TestExplain(t *testing.T) {
	t.Parallel()
	res, err := Best([]Candidate{cand("personal", nil, 78), cand("x", nil, 40), cand("work", nil, 9)})
	if err != nil {
		t.Fatal(err)
	}
	if got := res.Explain(); got != "auto → work (max 9%) over x (max 40%), personal (max 78%)" {
		t.Errorf("Explain = %q", got)
	}
	solo, _ := Best([]Candidate{cand("only", nil, 1)})
	if solo.Explain() != "auto → only (max 1%)" {
		t.Errorf("solo Explain = %q", solo.Explain())
	}
}
