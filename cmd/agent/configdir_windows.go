//go:build windows

package main

import "golang.org/x/sys/windows"

// Match the native UI's SpecialFolder.ApplicationData even when a launcher
// provides an incorrect or inherited AppData environment variable.
func configBaseDir() (string, error) {
	return windows.KnownFolderPath(windows.FOLDERID_RoamingAppData, 0)
}
