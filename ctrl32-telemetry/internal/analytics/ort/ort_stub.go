//go:build !(windows || (linux && (amd64 || arm64)) || darwin)

// Package ort: ONNX Runtime is not available on this platform (32-bit ARM and others); object counting with the
// built-in detector needs a 64-bit Linux, Windows or macOS server.
package ort

import "errors"

var errUnsupported = errors.New("object counting needs a 64-bit Linux, Windows or macOS server (ONNX Runtime)")

// Supported reports whether this platform can load ONNX Runtime.
const Supported = false

// Load always fails here.
func Load(string) (string, error) { return "", errUnsupported }

// Session is not available on this platform.
type Session struct{ InputName, OutputName string }

// NewSession always fails here.
func NewSession([]byte, int) (*Session, error) { return nil, errUnsupported }

// Run always fails here.
func (s *Session) Run([]float32, []int64) ([]float32, []int64, error) {
	return nil, nil, errUnsupported
}

// Close does nothing.
func (s *Session) Close() {}
