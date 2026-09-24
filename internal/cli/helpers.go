package cli

// ellipsis marks truncated text.
const ellipsis = "…"

// truncate shortens s to n runes, marking the cut.
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + ellipsis
}
