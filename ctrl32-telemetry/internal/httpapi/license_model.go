package httpapi

// The licence model of 2 Oct 2026 (ADR-0016 amendment, spec 026): the system is free including up to four cameras. A
// licence (one per server) covers more than four cameras, white labeling, changes through AI assistants (MCP writes)
// and the VPN extras, and includes the updates released until its updates_until date. After that date the system keeps
// running with everything it has; only newer versions are not installed until the licence is renewed. Nothing is ever
// switched off automatically: cameras beyond four on a server without a licence keep working, only adding or enabling
// more is refused.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/release"
)

// FreeCameraLimit is the number of enabled cameras a server runs without a licence (all organizations together).
const FreeCameraLimit = 4

// serverLicensed reports whether the server has a valid licence (the one licence feature, white_label, unlocks every
// licensed capability).
func (s *Server) serverLicensed(ctx context.Context) bool { return s.whiteLabelLicensed(ctx) }

type queryRower interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// enabledCameras counts the enabled cameras of all organizations, without the camera except (may be "").
func enabledCameras(ctx context.Context, q queryRower, except string) (int, error) {
	var n int
	err := q.QueryRow(ctx, `SELECT count(*) FROM core.camera WHERE enabled AND removed_at IS NULL AND ($1 = '' OR id::text <> $1)`, except).Scan(&n)
	return n, err
}

// cameraLimitError is the refusal of a fifth enabled camera without a licence.
type cameraLimitError struct{ enabled int }

func (e *cameraLimitError) Error() string {
	return fmt.Sprintf("Without a licence this server runs up to %d cameras (%d enabled now). The existing cameras keep working; more cameras need a licence (System → License).",
		FreeCameraLimit, e.enabled)
}

// cameraSlot is called inside the transaction that enables a camera (a new one, or one switched on): without a licence
// it serializes the check over all organizations and refuses when FreeCameraLimit cameras are already enabled.
func (s *Server) cameraSlot(ctx context.Context, tx pgx.Tx, except string) error {
	if s.serverLicensed(ctx) {
		return nil
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('ctrl32.camera_limit'))`); err != nil {
		return err
	}
	n, err := enabledCameras(ctx, tx, except)
	if err != nil {
		return err
	}
	if n >= FreeCameraLimit {
		return &cameraLimitError{enabled: n}
	}
	return nil
}

// cameraSlotFailed answers a refused (402 license_required) or failed camera check.
func (s *Server) cameraSlotFailed(w http.ResponseWriter, err error) {
	var le *cameraLimitError
	if errors.As(err, &le) {
		writeLicenseRequired(w, le.Error())
		return
	}
	writeErr(w, 500, "camera limit not checked")
}

func writeLicenseRequired(w http.ResponseWriter, msg string) {
	writeJSON(w, http.StatusPaymentRequired, map[string]string{"error": msg, "code": "license_required"})
}

// cameraLimitJSON is shown on the License page and in Camera management.
func (s *Server) cameraLimitJSON(ctx context.Context) map[string]any {
	n, _ := enabledCameras(ctx, s.pool, "")
	return map[string]any{"enabled": n, "free_limit": FreeCameraLimit, "licensed": s.serverLicensed(ctx)}
}

// updatesUntil is the last release date the server's valid licence includes updates for ("" = no limit: no licence,
// a licence that is not valid here, or one issued without the limit). Servers without a licence always update.
func (s *Server) updatesUntil(ctx context.Context) string {
	l := s.currentLicense(ctx).lic
	if !s.serverLicensed(ctx) || l == nil {
		return ""
	}
	return l.UpdatesUntil
}

// updatesJSON describes the included updates for the License and Updates pages.
func (s *Server) updatesJSON(ctx context.Context) map[string]any {
	out := map[string]any{"release_date": release.String()}
	if u := s.updatesUntil(ctx); u != "" {
		out["updates_until"] = u
		out["updates_ended"] = s.currentLicense(ctx).lic.UpdatesEnded(time.Now())
	}
	return out
}

// licenseCoversRelease reports whether a licence with the given updates_until includes this binary (a licence whose
// updates ended before this version was released cannot be installed on it; it can on the versions it covers).
func licenseCoversRelease(updatesUntil string) (bool, string) {
	if updatesUntil == "" {
		return true, ""
	}
	rel, ok := release.Date()
	if !ok {
		return true, ""
	}
	until, err := time.Parse(release.Layout, updatesUntil)
	if err != nil || release.Covered(rel, until) {
		return true, ""
	}
	return false, "the updates included in this licence ended on " + updatesUntil + ", before this version was released (" + rel.Format(release.Layout) +
		"); renew the licence, or use it with a version released until " + updatesUntil
}
