# Feature Specification: Video evidence (holds, fingerprints, signed packages)

**Feature Branch**: `assistant/jolly-sagan-nzolwc`

**Created**: 2026-09-27

**Status**: Implemented (0.50.0)

**Input**: From the list of what a professional system lacks: lock recordings as evidence and export them signed
(police, insurer); the user: "áno, poupravuj všetko".

**Related**: specs 010, 013, 014; URS P-20; constitution I (audit, integrity), II.

## Requirements
- FR-001 Hold a range of a camera's recordings: never deleted by retention days or the disk limit until released
  with a reason; held events kept too; audit.
- FR-002 SHA-256 of every recorded file computed while it is written and stored.
- FR-003 Evidence package: remuxed video (no re-encoding), manifest with hashes then/now, events, exporter, reason,
  Ed25519 signature by a server key sealed with the configuration secret; audit with the video hash.
- FR-004 `verify-evidence` command; any change of the video or manifest detected; key fingerprint shown in the UI.

## Success Criteria
- SC-001 e2e `evidence` passes (tampering detected); unit test of the package; held recordings survive retention.
