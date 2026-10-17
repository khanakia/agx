package provider

import (
	"slices"
	"testing"
)

// TestApplyLoginEnv_Matrix: variable present/absent/duplicated × set/unset/
// both, and the input is never modified.
func TestApplyLoginEnv_Matrix(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name  string
		env   []string
		set   map[string]string
		unset []string
		want  []string
	}{
		{"nothing", []string{"A=1"}, nil, nil, []string{"A=1"}},
		{"set-new", []string{"A=1"}, map[string]string{"T": "v"}, nil, []string{"A=1", "T=v"}},
		{"set-replaces", []string{"T=old", "A=1"}, map[string]string{"T": "v"}, nil, []string{"A=1", "T=v"}},
		{"unset-present", []string{"K=x", "A=1"}, nil, []string{"K"}, []string{"A=1"}},
		{"unset-absent", []string{"A=1"}, nil, []string{"K"}, []string{"A=1"}},
		{"unset-duplicates", []string{"K=x", "A=1", "K=y"}, nil, []string{"K"}, []string{"A=1"}},
		{"unset-prefix-only", []string{"KEY=x", "K=y"}, nil, []string{"K"}, []string{"KEY=x"}},
		{"both", []string{"K=x", "T=old", "A=1"}, map[string]string{"T": "v"}, []string{"K", "Z"}, []string{"A=1", "T=v"}},
		{"value-with-equals", nil, map[string]string{"T": "a=b"}, nil, []string{"T=a=b"}},
	} {
		in := slices.Clone(c.env)
		got := ApplyLoginEnv(in, c.set, c.unset)
		slices.Sort(got)
		if !slices.Equal(got, c.want) {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
		if !slices.Equal(in, c.env) {
			t.Errorf("%s: input modified to %v", c.name, in)
		}
	}
}

// Set names are applied in sorted order, so the env is identical on every
// call; without sorting, map iteration would reorder T1..T8 between calls.
func TestApplyLoginEnv_DeterministicOrder(t *testing.T) {
	t.Parallel()
	set := map[string]string{"T8": "8", "T3": "3", "T1": "1", "T6": "6", "T2": "2", "T7": "7", "T5": "5", "T4": "4"}
	want := []string{"A=1", "T1=1", "T2=2", "T3=3", "T4=4", "T5=5", "T6=6", "T7=7", "T8=8"}
	for range 50 {
		if got := ApplyLoginEnv([]string{"A=1"}, set, nil); !slices.Equal(got, want) {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}
