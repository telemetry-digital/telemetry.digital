# Feature Specification: Visual flow editor (Node-RED style automation)

**Feature Branch**: `assistant/jolly-sagan-nzolwc`

**Created**: 2026-09-25

**Status**: Draft

**Input**: User description: "Then it would need something like Node-RED implemented into it. Is there an alternative
to Node-RED — licensed, better, higher quality?"

**Related**: URS automation rules (existing rules engine, change requests, four-eyes), A-14…A-16 (webhooks, MQTT
publish out, generic webhook in), ADR-0013 (canvas editors, superseded in part by ADR-0015), constitution II
(no second runtime) and III (no user scripts without a sandbox decision).

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Build an automation visually from nodes and wires (Priority: P1)

An engineer opens "Flows", creates a flow and drags nodes from a palette onto a canvas: a trigger ("value of
Mixer/temp"), a condition ("> 80 for 2 minutes"), and two actions ("notify shift leader by e-mail", "send command
cooling = on"). Wires connect outputs to inputs. The engineer deploys the flow with a reason; it starts running
immediately, and its version is recorded.

**Why this priority**: This is the Node-RED experience the user asked for; everything else builds on it.

**Independent Test**: Build the example flow, deploy it, publish a temperature of 85 for 2 minutes, verify one e-mail
and one audited command; publish 70 and verify nothing more happens.

**Acceptance Scenarios**:

1. **Given** the palette, **When** the engineer drags nodes and draws wires, **Then** the canvas shows them with
   ports, and invalid wires (output to output, cycles without a delay) are refused.
2. **Given** a flow with an unconfigured node, **When** deploying, **Then** deployment is refused and the node is
   highlighted with the problem.
3. **Given** a deployed flow, **When** its trigger fires, **Then** each node processes the message in order and
   actions happen once per trigger.

---

### User Story 2 - Debug and test a flow (Priority: P1)

While editing, the engineer injects a test message into any node, sees the message passing each wire, and reads a
debug panel with the messages of debug nodes and the errors of any node (time, node, content). A flow can run in
"dry run": actions are shown, not executed.

**Why this priority**: Node-RED's strength is fast feedback; without it flows are guesswork.

**Independent Test**: Inject a message into a flow in dry run, verify the debug panel shows the path and the actions
that would run, and that no command or notification was created.

**Acceptance Scenarios**:

1. **Given** a flow in dry run, **When** a test message is injected, **Then** the debug panel lists each node's output
   and the actions that would run, and nothing is executed.

---

### User Story 3 - Rich node library (Priority: P2)

Nodes available:

- **Triggers**: datastream value, alarm raised/cleared/acknowledged, device connected/disconnected, device attribute
  changed, command acknowledged/failed, schedule (every N, at time of day, cron-like), incoming webhook (with token
  or HMAC), MQTT topic from a foreign broker, manual button.
- **Logic**: condition/filter, switch (route by rules), change (set/rename/delete message fields), math expression
  (safe expression language), delay, debounce, rate limit, deadband, aggregate over a window (min/max/avg/sum/count),
  join/merge, split, template (text with placeholders), counter, state memory (latch/toggle), time window
  (only between times/days).
- **Actions**: store a derived datastream value, send a device command, set a shared attribute, notify (e-mail,
  SMS, push, voice via configured channels), raise/clear an incident, HTTP request out to allowed hosts, MQTT publish
  to a foreign broker, write to OPC UA/Modbus through a connector (command with limits), debug/log.
- **Structure**: sub-flows (a flow used as a node with inputs and outputs), comments, groups.

**Why this priority**: Breadth; US1–US2 work with a small node set.

**Independent Test**: A regression flow per node category proves each node's behaviour with test messages.

**Acceptance Scenarios**:

1. **Given** a deadband node of 0.5, **When** values 20.0, 20.3, 20.6 arrive, **Then** only 20.0 and 20.6 pass.
2. **Given** an HTTP request node to a host not on the organization's allow-list, **When** deploying, **Then**
   deployment is refused.

---

### User Story 4 - Governance: versions, four-eyes, import/export (Priority: P2)

Flows are versioned content: each deploy is a new version with author and reason; the previous version can be
restored; flows that send commands or write to connectors follow the four-eyes policy (a second person approves
the deploy) when the policy is on. Flows can be exported and imported as JSON, and existing automation rules can
be opened as flows.

**Why this priority**: Regulated users (GxP) need traceability of automated actions; it also makes flows safer than
Node-RED's free editing.

**Independent Test**: Deploy two versions, restore the first, check the audit trail; with four-eyes on, verify a
command-sending flow waits for approval by another user.

**Acceptance Scenarios**:

1. **Given** four-eyes for commands is on, **When** a flow containing a command node is deployed, **Then** it stays
   pending until a different user approves it.
2. **Given** an automated command sent by a flow, **Then** its audit entry names the flow, its version and the
   triggering message as the reason.

---

### Edge Cases

- A node fails (for example the e-mail server is down): the error is logged with the message; retries follow the
  node's retry setting; the flow keeps processing other messages.
- A trigger fires faster than the flow can process: messages queue up to a limit, then the oldest are dropped and
  counted, with an alarm.
- Loops: a wire cycle without a delay/rate-limit node is refused at deploy.
- A bound datastream or device is deleted: the flow is marked broken and stops that branch; an alarm is raised.
- Server restart: deployed flows start again automatically; time-based state (delays, windows) resumes from stored
  state where it matters (latches, counters).
- Many flows: up to 500 deployed flows with up to 200 nodes each per organization.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The system MUST provide a visual editor with a node palette, canvas (zoom, pan, grid, multi-select,
  copy/paste, undo/redo), wires, node property dialogs and a debug panel.
- **FR-002**: The system MUST execute deployed flows on the server continuously, independent of any open browser.
- **FR-003**: The system MUST provide the node library of US3.
- **FR-004**: Expressions MUST use a safe, side-effect-free expression language (arithmetic, comparisons, logic,
  math functions, string and time functions) evaluated with time and size limits.
  [NEEDS CLARIFICATION: Should a sandboxed script node (JavaScript-like, like Node-RED's "function" node) be offered
  in addition — more power, but a larger attack surface and harder validation — or only the expression language?]
- **FR-005**: Every automated action MUST carry the flow's identity and version as its origin in the audit trail;
  commands MUST respect permissions of the flow's owner and the four-eyes policy.
- **FR-006**: Flows MUST be versioned content (deploy = new version with reason; restore; export/import JSON), and
  the server MUST validate every flow against a closed schema of node types and settings.
- **FR-007**: Outbound network nodes MUST only reach hosts on the organization's allow-list; secrets for foreign
  systems are stored encrypted and never shown again.
- **FR-008**: The system MUST provide dry run, test injection and a debug panel with at least the last 500 messages
  per flow.
- **FR-009**: The system MUST expose flow metrics: messages in/out, errors, queue length, last run per node.
- **FR-010**: Existing automation rules MUST keep working; the editor MUST be able to open an existing rule as a flow.
- **FR-011**: Everything MUST run inside the ctrl32 server (no separate Node.js or other runtime, constitution II).

### Key Entities

- **Flow**: name, version, state (draft, deployed, disabled, broken), nodes, wires, owner, reason.
- **Node**: type, settings, position, inputs/outputs.
- **Wire**: from node/port to node/port.
- **Message**: payload (value, time, quality, source), topic, metadata; bounded size.
- **Debug entry**: time, flow, node, message excerpt or error.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An engineer builds and deploys the US1 example flow in under 10 minutes on first use.
- **SC-002**: A message passes a 10-node flow in under 50 ms on the reference server; 1 000 messages per second
  across all flows are processed without loss.
- **SC-003**: 100 % of automated actions in the e2e suite are traceable to flow, version and trigger in the audit trail.
- **SC-004**: 0 flows with an invalid configuration can be deployed (regression set of invalid flows).

## Assumptions

- The flow engine is part of the ctrl32 server; Node-RED itself is not embedded (it needs Node.js, against
  constitution II), and paid products (FlowFuse, HighByte, Litmus, Crosser) cannot be bundled in a free product.
- The editor canvas may use one vendored library with a clean licence (candidates: Drawflow MIT, maxGraph
  Apache-2.0, Rete.js MIT) under ADR-0015; the choice is made in the plan.
- Notification channels, incidents, commands, connectors and the four-eyes policy already exist and are reused.
