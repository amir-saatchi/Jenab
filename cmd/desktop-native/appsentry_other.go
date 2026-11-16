//go:build !windows

package main

// refreshAppsEntry does nothing outside Windows: MyGo's updater brings
// Linux's desktop entry up to date itself.
func refreshAppsEntry(string) (bool, error) { return false, nil }
