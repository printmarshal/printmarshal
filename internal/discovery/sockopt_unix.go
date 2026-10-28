//go:build darwin || linux || freebsd

package discovery

import "syscall"

func setReuse(fd uintptr) error {
	if err := syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1); err != nil {
		return err
	}
	return syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, 0x200 /* SO_REUSEPORT */, 1)
}
