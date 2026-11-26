# 037 — Tag import from TIA Portal and STEP 7 for the S7 connector (0.91.0)

## Why

An S7 connector reads tags by absolute address, one line per tag: `key | address | type`. A plant has hundreds of
them, and they already exist in the PLC project — as the PLC tag table, the symbol table and the declarations of
data blocks. Typing them again is slow and a wrong offset shows a wrong value without any error. WinCC takes the tags
from the project; this spec lets the connector take them from the project's exports. Third step of "SCADA as
professional as WinCC" (035 alarms, 036 operator view).

## What

`POST /api/v1/connectors/s7/import` (multipart, `config.write`) reads one export file and answers with **proposed
tags**; the user picks which ones go into the connector's tag list. Nothing is stored, nothing is read from or
written to a PLC.

### Formats

| Source | File | What is read |
|---|---|---|
| TIA Portal PLC tag table | `.xlsx` (*Export tags*) | sheet with *Name*, *Data Type*, *Logical Address*, *Comment*; constants are ignored |
| TIA Portal PLC tag table | `.xml` (Openness, `SW.Tags.PlcTag`) | name, data type, logical address, comment |
| TIA Portal data block | `.db` / `.scl` source (*Generate source from blocks*), with `TYPE … END_TYPE` of its data types in the same file | declarations; start values are skipped |
| TIA Portal data block | `.xml` (Openness, `SW.Blocks.GlobalDB`) | members, block number, memory layout; data types are embedded |
| STEP 7 V5 symbol table | `.sdf`, `.asc`, `.seq` | symbol, address, data type, comment |
| STEP 7 V5 data block | `.awl` source | declarations; the block number is in the source |
| any tool | `.csv` / `.txt` with name, address, type, comment (comma, semicolon or tab; header optional) | rows |

Text files that are not UTF-8 are decoded with the chosen code page (Windows-1250/1251/1252/1253/1257); the UI
preselects it from the language.

### Tags with an address in the file

`%` and spaces are removed, English and German mnemonics are accepted, the address and type are checked by the
connector's own parser (`s7.ParseAddress`), so every proposed tag is one the connector accepts. Left out, each with a
reason: peripheral addresses (`PEW`, `%IW64:P`), symbols of blocks, timers and counters, data types the connector
cannot read (S5TIME, DTL, DATE_AND_TIME, LINT …), strings outside data blocks, addresses whose size does not fit the
type. TIME is read as DINT (ms), TIME_OF_DAY as UDINT (ms), DATE as UINT (days since 1990-01-01) — with a warning.

### Data blocks: calculated addresses

A block source carries no offsets, so they are calculated by the layout rules of blocks with **standard access**:

- BOOL one bit, consecutive BOOLs fill a byte; BYTE/CHAR/SINT/USINT one byte at the next free byte;
- every type of two bytes or more starts at an even byte; STRING[n] takes n + 2 bytes, WSTRING[n] 2n + 4;
- ARRAY, STRUCT and user data types start at an even byte, and so does the variable after them; array elements follow
  the same rules (BOOLs packed), every row of a multi-dimensional array starts at an even byte.

Structures and data types are expanded (`Motor.Current`), arrays element by element (`Temps[3]`, at most 1024 elements
of one array are proposed; the rest still takes its place). Unsupported types keep their size, so the addresses after
them stay right.

Safety of the result:

- **Optimized blocks are refused** (`S7_Optimized_Access := 'TRUE'`, `MemoryLayout Optimized`): they have no absolute
  addresses. The message says what to change.
- An **unknown data type** (not in the file, a function block instance, bounds given by constants) **stops the block**:
  the variables after it are left out, because their addresses depend on its size. Never a guess.
- A TIA Portal source does not carry the **block number**: the address is `DB{db}.…` until the user gives the number;
  such a tag cannot be added.
- Every data block import warns to compare a few addresses with the *Offset* column; odd-length strings and
  multi-dimensional arrays of bits or bytes are named.
- **No proposed tag is writable.** `rw` stays a deliberate edit of the tag line.

### Keys

From the symbol: diacritics removed, other characters to `_`, lower case, at most 32 characters (the beginning and the
end are kept), a digit first gets `t_`. An optional prefix (`m1_`) goes in front. Keys are unique within the result and
against the keys the connector already has (`_2`, `_3` …).

### UI

Settings → Connectors → Siemens S7 → **Import tags from TIA Portal / STEP 7**: file, data block number, key prefix,
text encoding, *Read file*; then the proposal with key, symbol, address, type, comment, a filter (text, area), *Select
shown*, *Clear selection*, the list *Left out* with reasons, the warnings, and *Add selected tags*. Addresses that are
already in the tag list are not added twice; a connector holds at most 2000 tags.

## Not in scope

- Reading a TIA Portal project file (`.ap1x`) or a STEP 7 project directly; symbolic access to optimized blocks
  (S7CommPlus); instance data blocks of function blocks; timers and counters.
- Scaling, units, alarm limits and datastream names from the project (the comment is shown, not stored).
- The same import for Modbus and OPC UA (OPC UA has browsing).

## Tests

- Unit `internal/s7/tagimport`: layout of a STEP 7 block with every rule (offsets as the DB editor shows them), a
  TIA Portal source with data types, arrays of data types, a two-dimensional array, LREAL, STRING, WSTRING; optimized
  block; missing data type; instance block; symbol tables (.sdf in Windows-1250, .asc, CSV); Excel tag table; both
  Openness XML forms; keys; limits and refusals. Every proposed address is parsed by the connector.
- E2E `tests/e2e/s7_import.sh` (37): formats, placeholders, refusals, and **the proposed addresses read from the S7
  simulator** — set point 21.5 at the calculated offset 8, the string at offset 20, MW10 from the symbol table.
- Headless `tests/headless/s7_import.js`: the dialog with a real file input, filter, selection, adding, the test read
  with the imported tags, creating the connector.
