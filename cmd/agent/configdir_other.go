//go:build !windows

package main

import "os"

func configBaseDir() (string, error) { return os.UserConfigDir() }
