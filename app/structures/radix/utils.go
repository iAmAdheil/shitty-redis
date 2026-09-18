package radix

// longest common prefix
func LCP(a, b []byte) int {
	var res int = 0
	for i := 0; i < min(len(a), len(b)); i++ {
		if a[i] != b[i] {
			break
		}

		res++
	}

	return res
}
