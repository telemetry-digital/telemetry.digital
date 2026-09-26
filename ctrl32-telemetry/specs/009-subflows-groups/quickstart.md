# Quickstart: validate spec 009

1. Flows → New flow → Start with: Subflow → add Math `value * 2` between the input and output → save.
2. New flow → Inject → Subflow (choose it) → Debug → save, deploy, press inject: the value is doubled.
3. Change the subflow to `value * 3` and save: the running flow still doubles; clear the version in the flow's
   Subflow node, save and deploy: it triples.
4. Select nodes → Group → rename, colour; drag the group: its nodes move with it.
5. Split: Inject `[5,6,7]` → Split → Debug shows three messages.

Automated: `go test ./internal/flows`, `tests/e2e/subflows.sh`.
