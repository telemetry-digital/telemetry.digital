# Research: Energy overview and scenes

## R1 Consumption from cumulative meters
- **Decision**: `lag()` over readings (with a 2-day look-back for the baseline), increases summed per
  `date_trunc(bucket, measured_at AT TIME ZONE tz)` in SQL.
- **Rationale**: one query for any period; buckets follow local days and DST; no pre-aggregation tables needed.
- **Alternatives**: continuous aggregates (TimescaleDB, not used), per-reading processing in Go (slow for a year of
  1-second Shelly data).

## R2 Resets
- **Decision**: drop below 50 % of the previous reading = reset (new reading counts); smaller drop = glitch (0).
- **Rationale**: real resets go to ~0 (daily counters, replaced meters); devices re-reporting a slightly older total
  after a restart would otherwise count the whole total.

## R3 Double counting
- **Decision**: by default one energy meter per device (the one named "total"); the user can change roles.

## R4 Scenes through the command path
- **Decision**: activation calls the entity command handler per action (as the user): permissions, four eyes and
  audit are those of commands; flows use the flow host's entity commands.
- **Alternatives**: a separate scene executor (duplicated governance).

## R5 Capture
- **Decision**: build actions from current states (light: brightness %, colour by color_mode; cover: position;
  climate: mode + temperature; number/select: value).
