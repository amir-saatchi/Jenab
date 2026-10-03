package project

import "syscall"

// isNetwork reports a network file system at p or its nearest existing
// parent.
func isNetwork(p string) bool {
	var st syscall.Statfs_t
	if statfsUp(p, func(q string) error { return syscall.Statfs(q, &st) }) != nil {
		return false
	}
	return networkMagic(uint32(st.Type))
}

func volumeService(string) string { return "" }
