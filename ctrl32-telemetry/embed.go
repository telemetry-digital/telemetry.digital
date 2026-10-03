// Package ctrl32telemetry exposes repository-root files that are embedded into the binary.
package ctrl32telemetry

import _ "embed"

// License is the full license text shown at /license and shipped with every binary.
//
//go:embed LICENSE
var License string

// SecurityPolicy is the product's security policy (how to report a vulnerability), served at /security-policy and
// named as the Policy of /.well-known/security.txt, so it is public wherever the product runs.
//
//go:embed SECURITY.md
var SecurityPolicy string
