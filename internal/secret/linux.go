//go:build linux

package secret

import (
	"bytes"
	"context"
	"errors"
	"github.com/Jimmy-xuzimo/Chronos-Agents/internal/privatefile"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func Save(dir, value string) error {
	if os.Getenv("DBUS_SESSION_BUS_ADDRESS") != "" {
		if tool, e := exec.LookPath("secret-tool"); e == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, tool, "store", "--label=Chronos Agents", "service", "chronos-agents", "directory", dir)
			cmd.Stdin = strings.NewReader(value)
			cmd.Stdout = &bytes.Buffer{}
			cmd.Stderr = &bytes.Buffer{}
			if cmd.Run() == nil {
				if e := privatefile.Write(filepath.Join(dir, "secret-backend"), []byte("keyring")); e != nil {
					return e
				}
				_ = os.Remove(filepath.Join(dir, "device.secret"))
				return nil
			}
		}
	}
	if e := privatefile.Write(filepath.Join(dir, "device.secret"), []byte(value)); e != nil {
		return e
	}
	return privatefile.Write(filepath.Join(dir, "secret-backend"), []byte("file"))
}
func Read(dir string) (string, error) {
	backend, _ := os.ReadFile(filepath.Join(dir, "secret-backend"))
	if string(backend) == "keyring" {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		out, e := exec.CommandContext(ctx, "secret-tool", "lookup", "service", "chronos-agents", "directory", dir).Output()
		if e != nil {
			return "", errors.New("keyring unavailable")
		}
		return strings.TrimSpace(string(out)), nil
	}
	path := filepath.Join(dir, "device.secret")
	info, e := os.Stat(path)
	if e != nil {
		return "", e
	}
	if info.Mode().Perm()&0077 != 0 {
		return "", errors.New("secret file permissions must be 0600")
	}
	b, e := os.ReadFile(path)
	return string(b), e
}
func Delete(dir string) error {
	backend, _ := os.ReadFile(filepath.Join(dir, "secret-backend"))
	if string(backend) == "keyring" {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = exec.CommandContext(ctx, "secret-tool", "clear", "service", "chronos-agents", "directory", dir).Run()
	}
	_ = os.Remove(filepath.Join(dir, "secret-backend"))
	e := os.Remove(filepath.Join(dir, "device.secret"))
	if os.IsNotExist(e) {
		return nil
	}
	return e
}
func Backend() string {
	if os.Getenv("DBUS_SESSION_BUS_ADDRESS") != "" {
		return "Secret Service when available; otherwise 0600 file"
	}
	return "0600 file/service account; disk encryption is deployment-owned"
}
