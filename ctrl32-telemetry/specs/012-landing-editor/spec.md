# Feature Specification: Graphical editor of the front page

**Feature Branch**: `main`

**Created**: 2026-09-26

**Status**: Implemented (0.47.0)

**Input**: User description: "A treba pridať grafický editor hlavnej stránky pred prihlásením" (a graphical editor
of the main page shown before sign-in).

**Related**: URS J-33; constitution I (audit), VII; spec 001 (SVG sanitiser).

## Clarifications (defaults)
- Scope: one front page per server (visitors are not yet in an organization); permission `system.admin`.
- Content model: blocks, no free HTML or scripts (safety); drafts and versions; explicit publish.

## User Scenarios
1. (P1) An administrator replaces the product text with the company's own: hero with logo and photo, three features,
   contact; checks it on the phone preview; publishes.
2. (P1) A wrong change is undone by publishing the previous version.
3. (P1) A malicious or careless edit cannot inject script, load foreign resources or hide sign-in.

## Requirements
- FR-001 Block types hero, features, text, image, split, stats, cta, contact, spacer; page theme.
- FR-002 Editor with add, reorder (drag), duplicate, delete, in-place text editing, properties, device widths, undo.
- FR-003 Versions, publish, unpublish, audit with reason.
- FR-004 Images: PNG/JPEG/WebP by content, SVG sanitised, ≤ 2 MiB, sandbox CSP, nosniff.
- FR-005 Strict server validation; text rendered as text; sign-in always in the header.

## Success Criteria
- SC-001 e2e `landing` passes; unit test of unsafe content.
