# Flows

Visual automation in the style of Node-RED, built into the ctrl32 server: triggers, logic and actions wired on a
canvas, running on the server all the time. Specification: `specs/004-flow-editor/`; URS C-12…C-17.

There is no separate runtime and no user code: every node comes from a closed library, and computation uses a small
expression language with limits. Every action a flow takes is audited with the flow, its version and the node as the
origin.

## Quick start

1. **Flows → New flow**, name it, pick *Example: value above a limit for a while → notify* and a reason.
2. Click the *Datastream value* node and choose the datastream; click *Notify* and choose a channel.
3. Press ▶ on the trigger to send a test message. With *Dry run* checked, the debug panel shows what the actions
   *would* do, and nothing is executed.
4. **Deploy** with a reason. The flow runs until you **Stop** it; the header shows `running vN`.

Editing a running flow saves a new draft version; the deployed version keeps running until you deploy again.

## Messages

A message has `value`, `topic`, `ts` (seconds since 1970), `quality`, `source` and any number of fields
`msg.<name>`. A datastream trigger sets the topic to the datastream name ("Mixer / temp"), the value to the measured
value and `ts` to the measurement time.

## Nodes

| Group | Node | What it does |
|---|---|---|
| Trigger | Datastream value | every stored value of a datastream (optionally only changes, at most one per N s) |
| | Alarm | alarm raised / cleared / acknowledged (filter by severity, datastream) |
| | Device connection | device connected / disconnected |
| | Schedule | every N seconds, or at times of day on chosen weekdays (organization time zone) |
| | Incoming webhook | a POST to the node's URL (`/api/v1/flow-hooks/fh_…`, shown after saving), optional HMAC secret |
| | Manual inject | the ▶ button in the editor or `POST /api/v1/flows/{slug}/inject` |
| Logic | Condition | true → output 1, false → output 2 |
| | Switch | one output per rule plus "otherwise"; first match or all |
| | Change | set, delete or rename `value`, `topic`, `msg.<name>` |
| | Math, Template | an expression or a `{{ expression }}` text into a field |
| | Delay, Debounce, Rate limit | timing; dropped messages are counted |
| | Deadband | passes a value only when it moved by the band (absolute or %) |
| | Aggregate | avg/min/max/sum/count/first/last over a window, per topic |
| | Join | keeps the latest value per topic; once all are present every message carries them as `msg.<topic>` |
| | Counter, Latch | state that survives restarts |
| | Time window | inside → output 1, outside → output 2 |
| | True for | one message when a condition has held for N seconds, one when it stops |
| | Split | one message per element of a list or JSON object (or part of a text by a separator), with `msg.index`, `msg.count`, `msg.key` |
| | Subflow | runs a subflow (below); one output per subflow output |
| Action | Store value | writes a (derived) datastream value |
| | Device command | a command with the permissions of the flow's owner and the four-eyes policy |
| | Device attribute | a shared attribute the device receives over MQTT; governed like a command |
| | Notify | through a notification channel (e-mail, SMS, push, voice, webhook) |
| | Incident | opens (once until resolved) or resolves an incident; open and resolve nodes with the same *incident key* belong together (without a key an open node keys the incident by itself and a resolve node resolves every open incident of the flow) |
| | HTTP request | only to hosts on the allow-list (Flows → Settings); response in `msg.status`, `msg.body` |
| | Debug | shows the message or an expression in the debug panel |
| Other | Comment | a note on the canvas |

`GET /api/v1/flows/catalog` returns the library with the settings schema of every node.

## Subflows

A subflow is a reusable part — for example *notify the shift and open an incident* — built once and used in many flows
as one **Subflow** node. Create it under Flows → New flow → *Start with: Subflow*: it has one **Subflow input** (where
messages enter) and **Subflow outputs** numbered 1–4 (each is an output of the Subflow node); triggers do not belong in
a subflow. Subflows can use other subflows (four levels).

A subflow is not deployed on its own; it runs inside the flows that use it. The Subflow node **pins the subflow
version** when the flow is saved, so changing a subflow does not change running flows: clear the version in the node
(or pick the subflow again), save and deploy the flow to take the new version — the deploy is audited and, when the
subflow sends commands, needs the same four-eyes approval as any commanding flow. In the debug panel, nodes of a
subflow appear as `<subflow node>.<node>`.

## Groups

Select nodes and choose *Group* to frame them in a named, coloured rectangle. Dragging a group moves the nodes inside
it; the corner resizes it; *Delete* removes the group, not its nodes. Groups only organize the canvas.

## Expressions

`value * 1.8 + 32`, `msg.temp > msg.limit + 2 && quality == 'ok'`, `if(value > 80, 'hot', 'ok')`,
`fixed(value, 1)`, `fmtTime(ts, 'HH:mm')`, `hour() >= 22`, `coalesce(msg.x, 0)`.

Names: `value`, `topic`, `ts`, `quality`, `source`, `msg.<field>`, `flow.<name>` (flow memory). Operators: arithmetic,
comparison, `&& || !`, `? :`, string `+`. Functions: `abs min max round floor ceil sqrt pow clamp log log10 exp len
lower upper trim contains startsWith endsWith replace substr str num bool fixed now hour minute weekday day month year
fmtTime if coalesce isNull`.

Limits: 1 000 characters, 300 syntax nodes, 10 000 evaluation steps and 4 096-character strings per expression. No
loops, no I/O, no access to anything outside the message.

## Governance

- Flows are **versioned**: every save is a new version with a reason; Versions lists them, Restore makes an old one
  the newest draft. Export/Import moves a flow between servers as JSON (`ctrl32-flow/1`).
- **Deploy** needs a reason and `content.write`; flows with command or attribute nodes also need `device.command`.
  With the four-eyes policy on, such a deploy becomes a change request that a different user approves.
- A flow with problems (unknown node, bad expression, missing reference, a host not on the allow-list, a loop without
  a delay) can be saved as a draft but **cannot be deployed** (422 with the list).
- Every action is in the audit trail with the origin `flow <name> v<version> node <id> (<trigger>)`; commands carry
  the flow as issuer.

## Converting automation rules

**Flows → New flow → Start with: Converted automation rule** (or `GET /api/v1/rules/{id}/as-flow`) turns a rule into
an equivalent draft flow: one datastream trigger per input, a Join, a *True for* over the rule's conditions (inputs
become `msg.<input>`), a Rate limit for the cooldown, the enter actions on the first output and the exit actions on
the second; an alarm action becomes an incident that opens and resolves. Staleness conditions, webhook secrets and
custom RPC arguments have no flow equivalent; they are listed before the flow is created and in a comment node. The
rule stays as it is: stop it once the flow runs, so the actions do not happen twice.

## Operation

- Each running flow has its own goroutine and a queue of 1 000 messages; when the queue is full, new messages are
  dropped and counted (status, metrics). One slow action does not block other flows; actions of a node run in order.
- A message passes a 10-node flow in well under a millisecond; the engine handles tens of thousands of messages per
  second on a laptop (`TestFlowThroughput`; the requirement is 1 000/s).
- The debug panel keeps the last 500 entries per flow; node badges show messages in/out and errors.
- Counters, latches and flow memory are stored in `core.flow_state` and survive restarts.
- `/metrics`: `ctrl32_flows_running`, `ctrl32_flow_messages_total`, `ctrl32_flow_errors_total`,
  `ctrl32_flow_dropped_total`. Incidents opened by Incident nodes have the kind `flow`.
- Limits: 200 nodes and 600 wires per flow, 500 deployed flows per organization.

## Not yet (phase 2)

Triggers on attribute changes, parameters per subflow instance. Connector writes (OPC UA, Modbus) already work through the Device command node,
since connector devices take commands like any other device.
