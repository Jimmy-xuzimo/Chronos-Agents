//go:build windows

package secret

import (
	"errors"
	"github.com/Jimmy-xuzimo/Chronos-Agents/internal/privatefile"
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"unsafe"
)

type blob struct {
	N uint32
	P *byte
}

var crypt = windows.NewLazySystemDLL("crypt32.dll")

func protect(b []byte, decode bool) ([]byte, error) {
	if len(b) == 0 {
		return nil, errors.New("empty secret")
	}
	in := blob{uint32(len(b)), &b[0]}
	var out blob
	var ok uintptr
	if decode {
		ok, _, _ = crypt.NewProc("CryptUnprotectData").Call(uintptr(unsafe.Pointer(&in)), 0, 0, 0, 0, 1, uintptr(unsafe.Pointer(&out)))
	} else {
		ok, _, _ = crypt.NewProc("CryptProtectData").Call(uintptr(unsafe.Pointer(&in)), 0, 0, 0, 0, 1, uintptr(unsafe.Pointer(&out)))
	}
	if ok == 0 {
		return nil, errors.New("DPAPI operation failed")
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.P)))
	return append([]byte{}, unsafe.Slice(out.P, int(out.N))...), nil
}
func Save(dir, value string) error {
	b, e := protect([]byte(value), false)
	if e != nil {
		return e
	}
	return privatefile.Write(filepath.Join(dir, "device.secret"), b)
}
func Read(dir string) (string, error) {
	b, e := os.ReadFile(filepath.Join(dir, "device.secret"))
	if e != nil {
		return "", e
	}
	b, e = protect(b, true)
	return string(b), e
}
func Delete(dir string) error { return os.Remove(filepath.Join(dir, "device.secret")) }
func Backend() string         { return "Windows DPAPI/current user" }
