//go:build windows

package ort

import (
	"fmt"
	"syscall"
)

// openLibrary loads onnxruntime.dll and returns OrtGetApiBase().
func openLibrary(lib string) (uintptr, error) {
	if lib == "" {
		lib = "onnxruntime.dll"
	}
	d, err := syscall.LoadDLL(lib)
	if err != nil {
		return 0, fmt.Errorf("onnxruntime library %s: %w", lib, err)
	}
	p, err := d.FindProc("OrtGetApiBase")
	if err != nil {
		return 0, fmt.Errorf("onnxruntime library %s: %w", lib, err)
	}
	r, _, _ := syscall.SyscallN(p.Addr())
	if r == 0 {
		return 0, fmt.Errorf("onnxruntime library %s: OrtGetApiBase returned nothing", lib)
	}
	return r, nil
}

func call(f uintptr, args ...uintptr) uintptr {
	r, _, _ := syscall.SyscallN(f, args...)
	return r
}
