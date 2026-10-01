# Feature Specification: Subflows, Split and groups in flows

**Feature Branch**: `main`

**Created**: 2026-09-26

**Status**: Implemented (0.44.0)

**Input**: User description: "Ďalej" (next) — the flow items of the phase-2 list: splitting messages, groups on the
canvas, subflows.

**Related**: spec 004 (flows), URS C-12…C-17, C-18; constitution I (audit), III, VII.

## Clarifications

### Session 2026-09-26
Defaults chosen (the user asked to continue):
- Q: How do subflow changes reach flows? → A: The subflow version is pinned in the node when the flow is saved; a new
  subflow version takes effect when the using flow is saved (version cleared) and deployed — the four-eyes rule and the
  audit of deploys cover subflow changes.
- Q: Are subflows deployed? → A: No; they run only inside flows (deploy refused).
- Q: Parameters per subflow instance? → A: Not in this version; the subflow reads the message (fields can be set
  before it with a Change node).

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Reuse a piece of logic (Priority: P1)
An engineer builds "Notify and open an incident" once as a subflow (input → template → notify → incident → output)
and uses it in ten flows as one node; outputs lead on in each flow.

**Independent Test**: a subflow doubling the value with two outputs (all values; values above 10) used in a flow:
7 → both outputs 14; 3 → first output 6 only.

### User Story 2 - Controlled changes (Priority: P1)
Changing the subflow does not change running flows until they are saved and deployed again.

### User Story 3 - Lists (Priority: P2)
A sensor gateway sends `[21.5, 22.0, 19.8]`; Split sends three messages with index and count.

### User Story 4 - Readable canvas (Priority: P2)
Nodes are framed in named, coloured groups that move with their nodes.

### Edge Cases
- Subflows inside subflows (up to 4 levels); a subflow cannot use itself (pinned versions make loops impossible,
  expansion also checks).
- A subflow with commands makes the using flow commanding (four eyes for its deploy).
- Split with more elements than the limit sends the first ones and logs how many were skipped.

## Requirements *(mandatory)*
- **FR-001**: Flow kind `subflow` with exactly one Subflow input and Subflow outputs 1–4; no triggers; not deployable.
- **FR-002**: Node Subflow (flow, version) with outputs derived from the subflow; version pinned at save.
- **FR-003**: Expansion of subflow nodes before a flow starts (and for dry runs), nested up to 4 levels.
- **FR-004**: Commands inside (nested) subflows count for the flow's four-eyes deploy rule.
- **FR-005**: Node Split (expression, separator, maximum) with msg.index, msg.count, msg.key.
- **FR-006**: Groups (name, colour, rectangle) stored with the flow, drawn behind nodes, dragged with their nodes,
  resizable; validated (≤ 50, colour #rrggbb).
- **FR-007**: Editor: subflow palette in subflows, subflow picker, new subflow from the flow list.

## Success Criteria *(mandatory)*
- **SC-001**: A running flow behaves identically before and after its subflow changes, until it is redeployed.
- **SC-002**: A subflow used in N flows is edited once.
