//go:build !(darwin || linux || freebsd)

package discovery

func setReuse(fd uintptr) error { return nil }
