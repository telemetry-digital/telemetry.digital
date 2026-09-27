---
description: "Tasks for subflows, Split and groups"
---

# Tasks: Subflows, Split and groups

**Input**: `specs/009-subflows-groups/` · **Tests**: required · **Paths**: relative to `ctrl32-telemetry/`

## Phase 1: Engine
- [x] T001 Spec kind and groups; nodes split, subflow, sub_in, sub_out in internal/flows/catalog.go
- [x] T002 Validation (subflow rules, pinning, groups, commands through subflows) in internal/flows/validate.go
- [x] T003 Expansion with nesting and recursion checks in internal/flows/subflow.go; engine loader, sub_out outputs
- [x] T004 Split processing in internal/flows/nodes.go; unit tests subflow_test.go

## Phase 2: Server and editor
- [x] T005 flowRefs.Subflow, kind/outputs, deploy refusal and expansion check in internal/httpapi/flows.go
- [x] T006 Editor: subflow palette, picker with derived outputs, groups (create, drag, resize, name, colour, delete)
- [x] T007 Flow list: new subflow, badges; no deploy for subflows

## Phase 3: Polish
- [x] T008 e2e tests/e2e/subflows.sh; sk/cs; docs/flows.md; URS C-18; JOURNAL, README; run-all
