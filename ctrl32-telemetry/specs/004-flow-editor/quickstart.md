# Quickstart: validate flows

1. Flows → New flow "Cooling" → drag "Datastream value" (Mixer/temp), "Condition" (`value > 80`, for 120 s),
   "Notify" (shift channel), "Command" (cooling = on) → wire → Deploy with a reason.
2. Publish 85 for 2 minutes → one notification, one command with the audit reason naming flow, version and node.
3. Dry run: inject `{value: 90}` into the trigger → debug panel shows the path and "would send command", nothing sent.
4. Invalid flows (unknown node, missing setting, wire to nowhere, cycle without delay, HTTP to a host not allowed,
   bad expression) are refused with the node highlighted (`POST /api/v1/flows/validate`).
5. Versions: deploy twice, restore v1; with four-eyes for commands, a deploy of a flow with a command node waits for
   a second user.
