//go:build windows

package main

import (
 "testing"
 "golang.org/x/sys/windows"
)

func TestConfigBaseIgnoresLauncherEnvironment(t *testing.T) {
 expected, err := windows.KnownFolderPath(windows.FOLDERID_RoamingAppData,0)
 if err != nil { t.Fatal(err) }
 t.Setenv("AppData", t.TempDir())
 actual, err := configBaseDir()
 if err != nil { t.Fatal(err) }
 if actual != expected { t.Fatal("collector and native UI must use the current user's known folder") }
}
