package limiter

func max(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
