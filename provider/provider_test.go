package provider

import (
	"slices"
	"testing"
)

func TestSetAndUnsetEnv(t *testing.T) {
	t.Parallel()
	base := []string{"A=1", "B=2", "AB=3"}
	got := SetEnv(base, "A", "9")
	if !slices.Equal(got, []string{"B=2", "AB=3", "A=9"}) {
		t.Errorf("SetEnv = %v", got)
	}
	if !slices.Equal(base, []string{"A=1", "B=2", "AB=3"}) {
		t.Error("SetEnv mutated its input")
	}
	// A prefix match must not remove a longer name (A vs AB).
	if got := UnsetEnv(base, "A"); !slices.Equal(got, []string{"B=2", "AB=3"}) {
		t.Errorf("UnsetEnv = %v", got)
	}
}

func TestHomeEnv(t *testing.T) {
	t.Parallel()
	env := []string{"X=1", "HOME_VAR=/stale"}
	if got := HomeEnv(env, "HOME_VAR", "/h/.tool/", "/h/.tool"); slices.Contains(got, "HOME_VAR=/stale") || len(got) != 1 {
		t.Errorf("default home must remove the var: %v", got)
	}
	if got := HomeEnv(env, "HOME_VAR", "/h/.tool-work", "/h/.tool"); !slices.Contains(got, "HOME_VAR=/h/.tool-work") {
		t.Errorf("non-default home must set the var: %v", got)
	}
}

func TestMaxPercent(t *testing.T) {
	t.Parallel()
	if (Usage{}).MaxPercent() != 0 {
		t.Error("empty usage should be 0")
	}
	u := Usage{Windows: []Window{{Percent: 5}, {Percent: 77}, {Percent: 60}}}
	if u.MaxPercent() != 77 {
		t.Errorf("MaxPercent = %v", u.MaxPercent())
	}
}
