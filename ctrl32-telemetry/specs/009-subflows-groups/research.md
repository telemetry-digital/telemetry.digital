# Research: Subflows, Split and groups

## R1 Expansion instead of a runtime call
- **Decision**: copy the subflow's nodes into the flow (ids `<node>.<id>`) before it starts.
- **Rationale**: the engine stays one goroutine and one queue per flow; debug, counters, state and limits work
  unchanged; no cross-flow calls.
- **Alternatives**: a subflow as a separately running flow called per message (queues, ordering and error
  propagation between flows).

## R2 Versions
- **Decision**: pin the subflow version in the node at save (validation), load exactly that version at start.
- **Rationale**: deployed behaviour changes only through a deploy (four eyes, audit); restarts are reproducible;
  loops are impossible because a pin always points to an existing, earlier version.

## R3 Groups
- **Decision**: rectangles in the spec, dragged with the nodes inside them at drag start.
- **Alternatives**: node membership lists (more bookkeeping when nodes move or are deleted).
