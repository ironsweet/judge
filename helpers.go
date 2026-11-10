package judge

func truncate(s string, size int) string {
	if len(s) > size {
		return s[:size] + "..."
	}
	return s
}
