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
