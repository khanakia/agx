package cli

import (
	"errors"
	"testing"

	"github.com/khanakia/agx/internal/render"
	"github.com/khanakia/agx/provider"
)

// TestAnyFailed: only a real error fails a usage run; ErrAPIBilled (wrapped
// or not) is an answer, and no rows is not a failure.
func TestAnyFailed(t *testing.T) {
	t.Parallel()
	boom := errors.New("boom")
	for name, c := range map[string]struct {
		errs []error
		want bool
	}{
		"none":         {nil, false},
		"all ok":       {[]error{nil, nil}, false},
		"api billed":   {[]error{provider.ErrAPIBilled}, false},
		"wrapped api":  {[]error{errors.Join(provider.ErrAPIBilled)}, false},
		"one failure":  {[]error{nil, boom}, true},
		"billed+error": {[]error{provider.ErrAPIBilled, provider.ErrExpired}, true},
	} {
		rs := make([]render.UsageReport, len(c.errs))
		for i, e := range c.errs {
			rs[i].Err = e
		}
		if got := anyFailed(rs); got != c.want {
			t.Errorf("%s: anyFailed = %v, want %v", name, got, c.want)
		}
	}
}

// TestPickOne: Esc becomes exit 1 (still ErrCancelled underneath), other
// errors pass through unchanged, and a choice is returned as is.
func TestPickOne(t *testing.T) {
	t.Parallel()
	boom := errors.New("boom")
	for name, c := range map[string]struct {
		p        *fakePicker
		want     int
		wantCode int
		wantErr  error
	}{
		"choice":    {&fakePicker{choice: 2}, 2, ExitOK, nil},
		"cancelled": {&fakePicker{err: ErrCancelled}, 0, ExitFailure, ErrCancelled},
		"other":     {&fakePicker{err: boom}, 0, ExitFailure, boom},
	} {
		a := &app{Deps: Deps{Picker: c.p}}
		i, err := a.pickOne("x", []string{"a", "b", "c"})
		if (c.wantErr == nil && err != nil) || (c.wantErr != nil && !errors.Is(err, c.wantErr)) || ExitCode(err) != c.wantCode {
			t.Errorf("%s: err = %v (exit %d)", name, err, ExitCode(err))
		}
		if err == nil && i != c.want {
			t.Errorf("%s: index = %d, want %d", name, i, c.want)
		}
	}
}
