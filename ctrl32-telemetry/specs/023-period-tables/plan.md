# Implementation Plan: Period tables with export

**Spec**: `specs/023-period-tables/spec.md` · **Version**: 0.63.0

## Design

| Part | Where | Notes |
|---|---|---|
| Query and periods | `internal/httpapi/period_table.go` | `parsePeriodTable`, `periodRange` (named periods, shift), `ptPeriodsOf`/`truncLocal`/`nextLocal` (local calendar), `rangeLabel` |
| Aggregation | `computePeriodTable` | one SQL over `core.observation` with `lag()` for consumption (energy rule of spec 008), `date_trunc(interval, measured_at AT TIME ZONE tz)`, first/last via ordered `array_agg`, `count(*) FILTER` for limits; the organization's zone from `orgLoc` |
| Totals and grid | `ptTotal`, `ptGrid` | one grid for XLSX, CSV and PDF, both orientations, translated headers |
| Exports | `periodTableExport` | CSV (BOM), XLSX (`internal/xlsx`), PDF (`report.Data` kind generic → `renderReport` → `servePDF`); `exportLanguage` from `locale` or the user's language; audit `export.read` |
| XLSX writer | `internal/xlsx` | zip of 6 XML parts; styles for 0–6 decimals and bold |
| Public | `publicPeriodTable` | ids checked against the shared dashboard's bindings, then the ordinary handler under the public principal |
| Widget | `widget_catalog_more.go`, `static/widgets-more.js` (`ptRender`), `static/dashboards.html` (`OPT_LABEL`) | series labels passed as `label` parameters; ◀ ▶ keep `el._shift` |
| OpenAPI | `openapi.go` | 3 operations, schemas `PeriodTable`, `PeriodCell`; `go run ./tools/apigen` |
