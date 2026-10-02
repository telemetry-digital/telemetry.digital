//go:build (linux && (amd64 || arm64)) || darwin

package ort

import (
	"fmt"
	"runtime"

	"github.com/ebitengine/purego"
)

// openLibrary loads libonnxruntime and returns OrtGetApiBase() (purego: no cgo needed).
func openLibrary(lib string) (uintptr, error) {
	if lib == "" {
		lib = "libonnxruntime.so"
		if runtime.GOOS == "darwin" {
			lib = "libonnxruntime.dylib"
		}
	}
	h, err := purego.Dlopen(lib, purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return 0, fmt.Errorf("onnxruntime library %s: %w", lib, err)
	}
	sym, err := purego.Dlsym(h, "OrtGetApiBase")
	if err != nil {
		return 0, fmt.Errorf("onnxruntime library %s: %w", lib, err)
	}
	r, _, _ := purego.SyscallN(sym)
	if r == 0 {
		return 0, fmt.Errorf("onnxruntime library %s: OrtGetApiBase returned nothing", lib)
	}
	return r, nil
}

func call(f uintptr, args ...uintptr) uintptr {
	r, _, _ := purego.SyscallN(f, args...)
	return r
}
