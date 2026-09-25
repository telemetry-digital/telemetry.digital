// Package ctrl32telemetry exposes repository-root files that are embedded into the binary.
package ctrl32telemetry

import _ "embed"

// License is the full license text shown at /license and shipped with every binary.
//
//go:embed LICENSE
var License string
