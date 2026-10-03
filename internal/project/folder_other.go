//go:build !windows && !linux && !darwin

package project

func isNetwork(string) bool       { return false }
func volumeService(string) string { return "" }
