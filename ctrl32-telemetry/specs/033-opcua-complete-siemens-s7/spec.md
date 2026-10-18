# Feature Specification: Complete OPC UA client and Siemens S7 connector

**Feature Branch**: `feature/0.83-opcua-s7`

**Created**: 2026-10-18

**Status**: Implemented (0.83.0)

**Input**: The owner asked for "complete OPC UA, so that several devices can be read as in FUXA, and S7comm Simatic —
complete reading and writing". Until 0.82 the OPC UA connector (0.13.0, writes 0.21.0) polled a typed list of node ids
(at most 500) into one virtual device, opened a new session for every write and had no way to look into the server;
Siemens PLCs were reachable only through an OPC UA server or a Modbus gateway.

**Related**: URS E-* (OPC UA), A-11/A-12 (reading and writing registers), ADR-0008 (SCADA in stages: writes are
commands, never a control loop), ADR-0009 (few third-party dependencies), spec 004 (flows), threat model.

## Clarifications (defaults)

- One connector reads **one** OPC UA server or **one** PLC; several servers or PLCs = several connectors (as FUXA's
  devices). New: a node or tag may name another virtual **device** (`device`), so one server — a gateway, a line
  controller, Kepware — feeds several devices; keys are unique per device. Writes resolve the key on the device the
  command is sent to.
- OPC UA reading: **polling** (default, as before) or **subscription** (`mode: subscribe`): one subscription, one
  monitored item per node, publishing and sampling interval `publishing_ms` 100–60 000 ms (default 1000), an optional
  absolute **deadband** per node. The values of one data change notification are ingested together. A lost connection
  (state check every 5 s or an error from the publish loop) reconnects with the same backoff as polling and creates
  the subscription again.
- **Browsing** (`POST /connectors/opcua/browse`): hierarchical references from a node (default Objects, i=85), objects
  and folders first, variables with data type, user access level (readable/writable) and current value; continuation
  points are followed up to 2000 children. Nothing is stored. The UI adds picked variables read-only (SAFETY FIRST:
  `rw` is always a deliberate edit).
- **Endpoint discovery** (`POST /connectors/opcua/endpoints`): GetEndpoints without a session — policy, mode, login
  types, server name, certificate thumbprint, whether this client supports the policy.
- **Server certificate pin**: optional SHA-1 thumbprint (as OPC UA tools show it). With a pin, a server presenting
  another certificate is refused before a session is opened; the refusal and the test show the presented thumbprint.
  Choosing a secured endpoint in the UI fills the pin.
- Writes use the connector's **open session** (falls back to a separate connection while reconnecting); limits, type
  coercion, read back, audit unchanged. Node limit 500 → 2000; reads are split into requests of 250 nodes.
- Float (OPC UA Float, S7 REAL) values are stored as the shortest decimal of the float32 (21.5, not 21.500000953…).

### Siemens S7

- Own implementation of S7comm (TPKT RFC 1006, COTP, S7 setup communication, Read Var, Write Var, SZL identification)
  without a third-party library (ADR-0009); a separate simulator implementation (`internal/s7/s7sim`,
  `tools/s7sim`) is used by unit tests, e2e and the bench.
- Connection: host, port 102, PLC family → default rack/slot (S7-300 0/2, S7-400 0/3, S7-1200/1500/ET 200SP 0/1) or
  TSAPs (LOGO! 01.00 → 02.00), explicit rack/slot/TSAPs override, connection type PG/OP/S7 basic, timeout, polling
  interval. PDU negotiated (960 requested); jobs of at most 20 items are split to fit request and response into the PDU;
  long strings are read and written in chunks.
- Addresses: Siemens notation (DBX/DBB/DBW/DBD/DBS, I/E, Q/A, M with bit, byte, word, double word), nodes7 notation
  (`DB1,REAL4` …) and LOGO! V memory (= DB1). Types bool, byte, char, sint, usint, int, uint, word, dint, udint, dword,
  real, lreal, string; an explicit type must match the size. Absolute addressing only: S7-1200/1500 need PUT/GET and
  non-optimized data blocks; the "function not available" answer is turned into a message that says so.
- Per-item errors (no such DB, out of range, access denied) are reported per tag and do not stop the others. A
  refused job (PUT/GET) keeps the connection; an I/O error reconnects.
- Writes: command `write {key, value}` on the device, tag must be `rw` (never inputs), value type and range checked and
  encoded **before** the command is recorded (e.g. 40000 into an INT is refused), written over the open connection,
  read back, acked; audit and four-eyes approval as for Modbus and OPC UA.
- Migration 0058 adds the connector kind `s7` and the device transport `s7`.

## Not in scope

- OPC UA: events and alarms & conditions, method calls, history read, arrays and structures (reported as unsupported),
  user certificates as login, an OPC UA server of our own (planned with the C++ runtime, ADR-0008).
- S7: symbolic access to optimized blocks (S7-1200/1500 "S7comm plus"), timers and counters, peripheral areas (PI/PQ),
  arrays, DT/DTL/WSTRING, writing several tags in one job, a TIA project import.

## Tests

- Unit `internal/opcua` (against the in-process OPC UA simulator): validation, multi-device reads, browsing (Objects →
  Simulation → Plant → Line1, types, access, values), endpoints, Basic256Sha256 SignAndEncrypt with the right and a
  wrong pin, subscription with notifications on two devices, write over the live session.
- Unit `internal/s7`: address parsing (valid and invalid), encode/decode of all types, multi-item reads over PDU 240,
  item errors, writes with read back, PUT/GET off, timeouts; connector settings and defaults, poll + write with scale,
  string and bool.
- E2E `tests/e2e/s7.sh` (55) and `tests/e2e/opcua.sh` (70: endpoints, browsing, a subscription feeding three devices,
  writes on a routed device); `modbus.sh`, `commands.sh`, `rules.sh`, `mcp.sh` unchanged and green.
- Headless `tests/headless/connectors_industrial.js`: discover endpoints, browse and add nodes, subscription form,
  S7 form with the test read, the list with both connectors, edit round trip; `DOCS=1` crops the documentation images.
