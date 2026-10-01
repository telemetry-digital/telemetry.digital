# Implementation Plan: Subflows, Split and groups

**Branch**: `main` | **Date**: 2026-09-26 | **Spec**: [spec.md](spec.md)

## Summary
Subflows are stored as flows of kind `subflow`; the engine expands subflow nodes into prefixed copies of the pinned
subflow version before a flow runs. Split is a logic node. Groups are part of the stored spec and only drawn by the
editor.

## Technical Context
Go 1.27 (internal/flows), plain JS editor; no migration (spec JSON gains `kind` and `groups`) · Tests: unit
`internal/flows` (expansion, nesting, recursion, validation, split), e2e `subflows`.

## Constitution Check
| Principle | Status |
|---|---|
| I Record integrity | subflow versions are flow versions (history, reason); pinned versions keep deploys auditable |
| III Secure by default | commands inside subflows keep the four-eyes rule; limits on depth, groups, split size |
| V Tested end to end | e2e `subflows` |
| VII Plain web UI, translated | editor groups and subflow picker, sk/cs |

## Structure
```
internal/flows/subflow.go      Expand, SubflowInfo, Loader
internal/flows/{catalog,validate,engine,nodes}.go   kind, groups, split, sub_in/sub_out/subflow nodes
internal/httpapi/flows.go      flowRefs.Subflow, kind/outputs in the list, deploy checks
internal/httpapi/static/flow-editor.js, flows.html
tests/e2e/subflows.sh
```
