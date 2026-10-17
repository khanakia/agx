package provider

import (
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"
)

// testHomeVar stands in for CLAUDE_CONFIG_DIR / CODEX_HOME: HomeEnv is
// provider-neutral, so the matrix uses one name for both.
const testHomeVar = "TOOL_HOME"

// homeVarValues returns every value env holds for key (more than one would
// mean a duplicate entry, which the child process resolves unpredictably).
func homeVarValues(env []string, key string) []string {
	var out []string
	for _, kv := range env {
		if v, ok := strings.CutPrefix(kv, key+"="); ok {
			out = append(out, v)
		}
	}
	return out
}

// TestHomeEnv_Matrix pins the rule the whole launcher rests on: the default
// home runs with the variable ABSENT (even when the parent shell exported a
// stale value, or the default spelled differently), every other home runs
// with exactly one entry set to that home.
func TestHomeEnv_Matrix(t *testing.T) {
	t.Parallel()
	const def = "/u/.tool"
	inherited := [][]string{
		nil,
		{},
		{"PATH=/bin"},
		{"PATH=/bin", testHomeVar + "=/stale"},
		{testHomeVar + "=" + def}, // shell exported the default itself
		{testHomeVar + "=/a", "X=1", testHomeVar + "=/b"}, // duplicated export
		{testHomeVar + "="}, // exported empty
		{testHomeVar + "X=keep", "X" + testHomeVar + "=keep"},
	}
	for _, tc := range []struct {
		name, home string
		absent     bool
	}{
		{"exact default", def, true},
		{"default with trailing slash", def + "/", true},
		{"default with dot segment", "/u/./.tool", true},
		{"default with double slash", "/u//.tool", true},
		{"default via parent segment", "/u/x/../.tool", true},
		{"sibling home", "/u/.tool-work", false},
		{"prefix of default", "/u/.too", false},
		{"default as prefix", "/u/.tool/sub", false},
		{"unicode home", "/u/.tool-ünïcødé", false},
		{"home with spaces", "/u/my tool home", false},
		{"relative home", ".tool", false},
		{"case differs", "/u/.TOOL", false},
	} {
		for i, env := range inherited {
			before := slices.Clone(env)
			got := HomeEnv(env, testHomeVar, tc.home, def)
			vals := homeVarValues(got, testHomeVar)
			if tc.absent && len(vals) != 0 {
				t.Errorf("%s/env%d: default home must run with %s absent, got %v", tc.name, i, testHomeVar, vals)
			}
			if !tc.absent && !slices.Equal(vals, []string{tc.home}) {
				t.Errorf("%s/env%d: want exactly %s=%s, got %v", tc.name, i, testHomeVar, tc.home, vals)
			}
			if !slices.Equal(env, before) {
				t.Errorf("%s/env%d: HomeEnv mutated its input", tc.name, i)
			}
			// Unrelated variables, including names that merely share a prefix, survive.
			for _, kv := range env {
				if !strings.HasPrefix(kv, testHomeVar+"=") && !slices.Contains(got, kv) {
					t.Errorf("%s/env%d: dropped unrelated %q", tc.name, i, kv)
				}
			}
		}
	}
}

func TestSetUnsetEnv_Matrix(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, key, value string
		env, wantSet     []string
		wantUnset        []string
	}{
		{"nil env", "K", "v", nil, []string{"K=v"}, []string{}},
		{"replace", "K", "v", []string{"K=old"}, []string{"K=v"}, []string{}},
		{"duplicates collapse", "K", "v", []string{"K=1", "A=2", "K=3"}, []string{"A=2", "K=v"}, []string{"A=2"}},
		{"value with equals", "K", "a=b=c", []string{"A=1"}, []string{"A=1", "K=a=b=c"}, []string{"A=1"}},
		{"empty value", "K", "", []string{"K=x"}, []string{"K="}, []string{}},
		{"unicode value", "K", "héllo wörld", nil, []string{"K=héllo wörld"}, []string{}},
		{"longer name kept", "K", "v", []string{"KK=1", "K_X=2"}, []string{"KK=1", "K_X=2", "K=v"}, []string{"KK=1", "K_X=2"}},
		{"bare name without equals kept", "K", "v", []string{"K"}, []string{"K", "K=v"}, []string{"K"}},
	} {
		before := slices.Clone(tc.env)
		if got := SetEnv(tc.env, tc.key, tc.value); !slices.Equal(got, tc.wantSet) {
			t.Errorf("%s: SetEnv = %q, want %q", tc.name, got, tc.wantSet)
		}
		if got := UnsetEnv(tc.env, tc.key); !slices.Equal(got, tc.wantUnset) {
			t.Errorf("%s: UnsetEnv = %q, want %q", tc.name, got, tc.wantUnset)
		}
		if !slices.Equal(tc.env, before) {
			t.Errorf("%s: input mutated", tc.name)
		}
	}
}

func TestApplyLoginEnv_InputSpace(t *testing.T) {
	t.Parallel()
	const fakeToken = "fake-token-0001"
	for _, tc := range []struct {
		name  string
		env   []string
		set   map[string]string
		unset []string
		want  []string
	}{
		{"nothing", []string{"A=1"}, nil, nil, []string{"A=1"}},
		{"unset only", []string{"A=1", "KEY=x"}, nil, []string{"KEY"}, []string{"A=1"}},
		{"set replaces inherited", []string{"TOK=old"}, map[string]string{"TOK": fakeToken}, nil, []string{"TOK=" + fakeToken}},
		{"set sorted regardless of map order", nil, map[string]string{"Z": "1", "A": "2", "M": "3"}, nil, []string{"A=2", "M=3", "Z=1"}},
		{"unset then set same key sets it", []string{"TOK=old"}, map[string]string{"TOK": fakeToken}, []string{"TOK"}, []string{"TOK=" + fakeToken}},
		{"unset missing key is no-op", []string{"A=1"}, nil, []string{"NOPE", ""}, []string{"A=1"}},
	} {
		before := slices.Clone(tc.env)
		if got := ApplyLoginEnv(tc.env, tc.set, tc.unset); !slices.Equal(got, tc.want) {
			t.Errorf("%s: ApplyLoginEnv = %q, want %q", tc.name, got, tc.want)
		}
		if !slices.Equal(tc.env, before) {
			t.Errorf("%s: input mutated", tc.name)
		}
	}
}

// TestRateLimitError_RetryAfterMatrix covers both Retry-After forms (RFC 9110
// §10.2.3) and every value that must fall back to the bare sentinel.
func TestRateLimitError_RetryAfterMatrix(t *testing.T) {
	t.Parallel()
	future := time.Now().Add(2 * time.Hour).UTC().Format(http.TimeFormat)
	past := time.Now().Add(-2 * time.Hour).UTC().Format(http.TimeFormat)
	for _, tc := range []struct {
		name, value, want string
	}{
		{"absent", "", ""},
		{"seconds", "42", "retry after 42s"},
		{"zero seconds", "0", "retry after 0s"},
		{"large seconds", "86400", "retry after 24h0m0s"},
		{"negative seconds", "-5", ""},
		{"float seconds", "1.5", ""},
		{"garbage", "soon", ""},
		{"unicode digits", "４２", ""},
		{"http date in future", future, "retry after 1h59m"}, // rounded to a second, so 1h59m59s or 2h0m0s
		{"http date in past", past, ""},
		{"spaces", " 42 ", ""},
	} {
		h := http.Header{}
		if tc.value != "" {
			h.Set(retryAfterHeader, tc.value)
		}
		err := RateLimitError(h)
		if !errors.Is(err, ErrRateLimited) {
			t.Errorf("%s: %v is not ErrRateLimited", tc.name, err)
			continue
		}
		msg := err.Error()
		switch {
		case tc.name == "http date in future":
			if !strings.Contains(msg, "retry after 1h59m") && !strings.Contains(msg, "retry after 2h0m0s") {
				t.Errorf("%s: %q, want ~2h", tc.name, msg)
			}
		case tc.want == "":
			if msg != ErrRateLimited.Error() {
				t.Errorf("%s: %q, want the bare sentinel", tc.name, msg)
			}
		case !strings.Contains(msg, tc.want):
			t.Errorf("%s: %q, want %q", tc.name, msg, tc.want)
		}
	}
}
