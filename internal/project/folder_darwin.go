package project

import "syscall"

// isNetwork reports a network file system at p or its nearest existing
// parent.
func isNetwork(p string) bool {
	var st syscall.Statfs_t
	if statfsUp(p, func(q string) error { return syscall.Statfs(q, &st) }) != nil {
		return false
	}
	b := make([]byte, 0, len(st.Fstypename))
	for _, c := range st.Fstypename {
		if c == 0 {
			break
		}
		b = append(b, byte(c))
	}
	return networkFSType(string(b))
}

func volumeService(string) string { return "" }
