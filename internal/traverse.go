package internal

// TraverseErr traverses the err error chain until fn returns true.
// Traversal stops on nil errors, fn(nil) is never called.
// Returns true if fn matched, false otherwise.
func TraverseErr(err error, fn func(error) (ok bool)) (ok bool) {
	if err == nil {
		return false
	}

	if fn(err) {
		return true
	}

	switch err := err.(type) {
	case interface{ Unwrap() error }:
		return TraverseErr(err.Unwrap(), fn)

	case interface{ Unwrap() []error }:
		for _, err2 := range err.Unwrap() {
			if TraverseErr(err2, fn) {
				return true
			}
		}
	}

	return false
}
