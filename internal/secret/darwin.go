//go:build darwin

package secret

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func helper(action, dir, value string) (string, error) {
	exe, e := os.Executable()
	if e != nil {
		return "", e
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, filepath.Join(filepath.Dir(exe), "chronos-keychain"), action, dir)
	cmd.Stdin = strings.NewReader(value)
	cmd.Stderr = &bytes.Buffer{}
	out, e := cmd.Output()
	if e != nil {
		return "", errors.New("Keychain helper failed; reinstall the complete macOS package")
	}
	return string(out), nil
}
func Save(dir, value string) error    { _, e := helper("save", dir, value); return e }
func Read(dir string) (string, error) { return helper("read", dir, "") }
func Delete(dir string) error         { _, e := helper("delete", dir, ""); return e }
func Backend() string                 { return "macOS Keychain" }
