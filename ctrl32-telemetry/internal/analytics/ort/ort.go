//go:build windows || (linux && (amd64 || arm64)) || darwin

// Package ort runs an ONNX model on the CPU through the C API of ONNX Runtime (MIT licence), loaded at run time from
// its shared library (onnxruntime.dll, libonnxruntime.so) — the binary stays free of cgo. Only what a detector needs
// is bound: a session from model bytes, one float input, one float output.
package ort

import (
	"errors"
	"fmt"
	"runtime"
	"sync"
	"unsafe"
)

// apiVersion is the oldest C API version the bindings need (ONNX Runtime 1.17); newer libraries serve it too.
const apiVersion = 17

// Indices of the functions in struct OrtApi (onnxruntime_c_api.h; the table only ever grows at the end).
const (
	fnGetErrorMessage                  = 2
	fnCreateEnv                        = 3
	fnCreateSessionFromArray           = 8
	fnRun                              = 9
	fnCreateSessionOptions             = 10
	fnSetSessionGraphOptimizationLevel = 23
	fnSetIntraOpNumThreads             = 24
	fnSetInterOpNumThreads             = 25
	fnSessionGetInputName              = 36
	fnSessionGetOutputName             = 37
	fnCreateTensorWithDataAsOrtValue   = 49
	fnGetTensorMutableData             = 51
	fnGetDimensionsCount               = 61
	fnGetDimensions                    = 62
	fnGetTensorTypeAndShape            = 65
	fnCreateCpuMemoryInfo              = 69
	fnAllocatorFree                    = 76
	fnGetAllocatorWithDefaultOptions   = 78
	fnReleaseEnv                       = 92
	fnReleaseStatus                    = 93
	fnReleaseMemoryInfo                = 94
	fnReleaseSession                   = 95
	fnReleaseValue                     = 96
	fnReleaseTensorTypeAndShapeInfo    = 99
	fnReleaseSessionOptions            = 100
)

var (
	loadMu  sync.Mutex
	api     uintptr // const OrtApi*
	version string
	env     uintptr
)

// ptr turns an address returned by the library into a pointer (go vet accepts this form).
func ptr(p uintptr) unsafe.Pointer {
	var u unsafe.Pointer
	*(*uintptr)(unsafe.Pointer(&u)) = p
	return u
}

func fn(i int) uintptr { return *(*uintptr)(unsafe.Add(ptr(api), i*int(unsafe.Sizeof(uintptr(0))))) }

func cstring(p uintptr) string {
	if p == 0 {
		return ""
	}
	n := 0
	for *(*byte)(unsafe.Add(ptr(p), n)) != 0 && n < 4096 {
		n++
	}
	return string(unsafe.Slice((*byte)(ptr(p)), n))
}

func cstr(s string) []byte { return append([]byte(s), 0) }

// check turns an OrtStatus* into an error (and releases it).
func check(st uintptr) error {
	if st == 0 {
		return nil
	}
	msg := cstring(call(fn(fnGetErrorMessage), st))
	call(fn(fnReleaseStatus), st)
	return errors.New("onnxruntime: " + msg)
}

// Load opens the ONNX Runtime library once. Version is its version string.
func Load(lib string) (string, error) {
	loadMu.Lock()
	defer loadMu.Unlock()
	if api != 0 {
		return version, nil
	}
	base, err := openLibrary(lib)
	if err != nil {
		return "", err
	}
	// struct OrtApiBase { const OrtApi* (*GetApi)(uint32_t); const char* (*GetVersionString)(void); }
	getAPI := *(*uintptr)(ptr(base))
	getVersion := *(*uintptr)(unsafe.Add(ptr(base), unsafe.Sizeof(uintptr(0))))
	version = cstring(call(getVersion))
	a := call(getAPI, apiVersion)
	if a == 0 {
		return version, fmt.Errorf("onnxruntime %s does not offer C API version %d (1.17 or newer needed)", version, apiVersion)
	}
	api = a
	logid := cstr("ctrl32-analytics")
	var e uintptr
	err = check(call(fn(fnCreateEnv), 3 /* ORT_LOGGING_LEVEL_ERROR */, uintptr(unsafe.Pointer(&logid[0])), uintptr(unsafe.Pointer(&e))))
	runtime.KeepAlive(logid)
	if err != nil {
		api = 0
		return version, err
	}
	env = e
	return version, nil
}

// Session is a loaded model with one input and one output.
type Session struct {
	mu                    sync.Mutex
	sess                  uintptr
	mem                   uintptr
	inName                []byte
	outName               []byte
	InputName, OutputName string
}

// NewSession loads a model from its bytes. threads is the number of CPU threads of one inference (1 is usually right
// for a camera at a few frames per second; more cameras run in parallel processes).
func NewSession(model []byte, threads int) (*Session, error) {
	if api == 0 {
		return nil, errors.New("onnxruntime is not loaded")
	}
	if len(model) == 0 {
		return nil, errors.New("empty model")
	}
	var opts uintptr
	if err := check(call(fn(fnCreateSessionOptions), uintptr(unsafe.Pointer(&opts)))); err != nil {
		return nil, err
	}
	defer call(fn(fnReleaseSessionOptions), opts)
	if threads < 1 {
		threads = 1
	}
	if err := check(call(fn(fnSetIntraOpNumThreads), opts, uintptr(threads))); err != nil {
		return nil, err
	}
	if err := check(call(fn(fnSetInterOpNumThreads), opts, 1)); err != nil {
		return nil, err
	}
	if err := check(call(fn(fnSetSessionGraphOptimizationLevel), opts, 99 /* ORT_ENABLE_ALL */)); err != nil {
		return nil, err
	}
	s := &Session{}
	err := check(call(fn(fnCreateSessionFromArray), env, uintptr(unsafe.Pointer(&model[0])), uintptr(len(model)), opts, uintptr(unsafe.Pointer(&s.sess))))
	runtime.KeepAlive(model)
	if err != nil {
		return nil, err
	}
	if err := check(call(fn(fnCreateCpuMemoryInfo), 1 /* OrtArenaAllocator */, 0 /* OrtMemTypeDefault */, uintptr(unsafe.Pointer(&s.mem)))); err != nil {
		s.Close()
		return nil, err
	}
	var alloc uintptr
	if err := check(call(fn(fnGetAllocatorWithDefaultOptions), uintptr(unsafe.Pointer(&alloc)))); err != nil {
		s.Close()
		return nil, err
	}
	name := func(f int) (string, error) {
		var p uintptr
		if err := check(call(fn(f), s.sess, 0, alloc, uintptr(unsafe.Pointer(&p)))); err != nil {
			return "", err
		}
		n := cstring(p)
		call(fn(fnAllocatorFree), alloc, p)
		return n, nil
	}
	if s.InputName, err = name(fnSessionGetInputName); err != nil {
		s.Close()
		return nil, err
	}
	if s.OutputName, err = name(fnSessionGetOutputName); err != nil {
		s.Close()
		return nil, err
	}
	s.inName, s.outName = cstr(s.InputName), cstr(s.OutputName)
	return s, nil
}

// Run runs the model on one float32 tensor of the given shape and returns the first output and its shape.
func (s *Session) Run(input []float32, shape []int64) ([]float32, []int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sess == 0 {
		return nil, nil, errors.New("session closed")
	}
	n := int64(1)
	for _, d := range shape {
		n *= d
	}
	if n != int64(len(input)) || n == 0 {
		return nil, nil, fmt.Errorf("input of %d values does not match the shape %v", len(input), shape)
	}
	var in uintptr
	if err := check(call(fn(fnCreateTensorWithDataAsOrtValue), s.mem, uintptr(unsafe.Pointer(&input[0])), uintptr(len(input)*4),
		uintptr(unsafe.Pointer(&shape[0])), uintptr(len(shape)), 1 /* ONNX_TENSOR_ELEMENT_DATA_TYPE_FLOAT */, uintptr(unsafe.Pointer(&in)))); err != nil {
		return nil, nil, err
	}
	defer call(fn(fnReleaseValue), in)
	inNames := []uintptr{uintptr(unsafe.Pointer(&s.inName[0]))}
	outNames := []uintptr{uintptr(unsafe.Pointer(&s.outName[0]))}
	inputs := []uintptr{in}
	outputs := []uintptr{0}
	err := check(call(fn(fnRun), s.sess, 0, uintptr(unsafe.Pointer(&inNames[0])), uintptr(unsafe.Pointer(&inputs[0])), 1,
		uintptr(unsafe.Pointer(&outNames[0])), 1, uintptr(unsafe.Pointer(&outputs[0]))))
	runtime.KeepAlive(input)
	runtime.KeepAlive(shape)
	runtime.KeepAlive(inNames)
	runtime.KeepAlive(outNames)
	runtime.KeepAlive(s.inName)
	runtime.KeepAlive(s.outName)
	if err != nil {
		return nil, nil, err
	}
	out := outputs[0]
	defer call(fn(fnReleaseValue), out)
	var info uintptr
	if err := check(call(fn(fnGetTensorTypeAndShape), out, uintptr(unsafe.Pointer(&info)))); err != nil {
		return nil, nil, err
	}
	defer call(fn(fnReleaseTensorTypeAndShapeInfo), info)
	var nd uintptr
	if err := check(call(fn(fnGetDimensionsCount), info, uintptr(unsafe.Pointer(&nd)))); err != nil {
		return nil, nil, err
	}
	if nd == 0 || nd > 8 {
		return nil, nil, fmt.Errorf("unexpected output rank %d", nd)
	}
	dims := make([]int64, nd)
	if err := check(call(fn(fnGetDimensions), info, uintptr(unsafe.Pointer(&dims[0])), nd)); err != nil {
		return nil, nil, err
	}
	total := int64(1)
	for _, d := range dims {
		total *= d
	}
	var data uintptr
	if err := check(call(fn(fnGetTensorMutableData), out, uintptr(unsafe.Pointer(&data)))); err != nil {
		return nil, nil, err
	}
	if total <= 0 || total > 1<<26 || data == 0 {
		return nil, nil, fmt.Errorf("unexpected output size %v", dims)
	}
	res := make([]float32, total)
	copy(res, unsafe.Slice((*float32)(ptr(data)), total))
	return res, dims, nil
}

// Close releases the session.
func (s *Session) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.mem != 0 {
		call(fn(fnReleaseMemoryInfo), s.mem)
		s.mem = 0
	}
	if s.sess != 0 {
		call(fn(fnReleaseSession), s.sess)
		s.sess = 0
	}
}

// Supported reports whether this platform can load ONNX Runtime.
const Supported = true
