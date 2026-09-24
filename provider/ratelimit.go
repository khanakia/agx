package provider

import (
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// retryAfterHeader is the standard HTTP header carrying a 429's back-off.
const retryAfterHeader = "Retry-After"

// RateLimitError wraps ErrRateLimited with the server's Retry-After hint,
// when it sent one. Both providers' usage clients return it for HTTP 429 so
// the message is the same everywhere.
func RateLimitError(h http.Header) error {
	if d, ok := retryAfter(h.Get(retryAfterHeader)); ok {
		return fmt.Errorf("%w (retry after %s)", ErrRateLimited, d)
	}
	return ErrRateLimited
}

// retryAfter parses a Retry-After value: delta-seconds or an HTTP date.
func retryAfter(v string) (time.Duration, bool) {
	if v == "" {
		return 0, false
	}
	if secs, err := strconv.Atoi(v); err == nil && secs >= 0 {
		return time.Duration(secs) * time.Second, true
	}
	if t, err := http.ParseTime(v); err == nil {
		if d := time.Until(t).Round(time.Second); d > 0 {
			return d, true
		}
	}
	return 0, false
}
