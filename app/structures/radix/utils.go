package radix

import "errors"

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

func Predecessor[T byte](a []T, val T) (T, error) {
	var r *T
	for _, v := range a {
		if v < val {
			if r == nil || (r != nil && *r < v) {
				r = &v
			}
		}
	}
	if r == nil {
		return 0x0, errors.New("No predecessor exists")
	}
	return *r, nil
}

func Successor[T byte](a []T, val T) (T, error) {
	var r *T
	for _, v := range a {
		if v > val {
			if r == nil || (r != nil && *r > v) {
				r = &v
			}
		}
	}
	if r == nil {
		return 0x0, errors.New("No predecessor exists")
	}
	return *r, nil
}
