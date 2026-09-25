// Package audit appends entries to the hash-chained audit trail and verifies the chain (ADR-0005).
// The hash and sequence are computed by the database trigger; this package only supplies the content.
package audit

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Entry is one audit record. Action is "<entity>.<verb>"; the database requires Reason for
// update/delete/revoke/acknowledge/suppress/deactivate/annotate/redecode verbs.
type Entry struct {
	OrgID      string // uuid or ""
	ActorType  string // user | api_token | device | system
	ActorID    string // uuid or ""
	ActorName  string
	Action     string
	EntityType string
	EntityID   string // uuid or ""
	Old        any
	New        any
	Reason     string
	RequestID  string
}

// Result of a chain verification.
type Result struct {
	OK          bool
	Checked     int64
	FirstBadSeq *int64
	HeadSeq     *int64
	HeadHash    *string
}

// Append inserts the entry and returns the assigned sequence and hash.
func Append(ctx context.Context, pool *pgxpool.Pool, e Entry) (seq int64, hash string, err error) {
	oldJSON, err := marshalNullable(e.Old)
	if err != nil {
		return 0, "", err
	}
	newJSON, err := marshalNullable(e.New)
	if err != nil {
		return 0, "", err
	}
	err = pool.QueryRow(ctx, `
		INSERT INTO core.audit_trail (org_id, actor_type, actor_id, actor_name, action, entity_type, entity_id, old_value, new_value, reason, request_id)
		VALUES (NULLIF($1,'')::uuid, $2, NULLIF($3,'')::uuid, $4, $5, $6, NULLIF($7,'')::uuid, $8::jsonb, $9::jsonb, NULLIF($10,''), NULLIF($11,''))
		RETURNING seq, hash`,
		e.OrgID, e.ActorType, e.ActorID, e.ActorName, e.Action, e.EntityType, e.EntityID, oldJSON, newJSON, e.Reason, e.RequestID,
	).Scan(&seq, &hash)
	if err != nil {
		return 0, "", fmt.Errorf("audit: append %s: %w", e.Action, err)
	}
	return seq, hash, nil
}

// Verify re-walks the whole chain in the database.
func Verify(ctx context.Context, pool *pgxpool.Pool) (Result, error) {
	var r Result
	err := pool.QueryRow(ctx, `SELECT ok, checked, first_bad_seq, head_seq, head_hash FROM core.verify_audit_chain()`).
		Scan(&r.OK, &r.Checked, &r.FirstBadSeq, &r.HeadSeq, &r.HeadHash)
	if err != nil {
		return r, fmt.Errorf("audit: verify: %w", err)
	}
	return r, nil
}

func marshalNullable(v any) (*string, error) {
	if v == nil {
		return nil, nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("audit: marshal: %w", err)
	}
	s := string(b)
	return &s, nil
}
