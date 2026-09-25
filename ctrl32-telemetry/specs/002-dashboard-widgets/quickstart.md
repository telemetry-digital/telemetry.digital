# Quickstart: validate dashboard widgets

```bash
cd ctrl32-telemetry && bash tests/run-all.sh
```

Manual: Dashboards → New → "Dashboard — showcase (all widget types)": every type renders with demo data; edit a widget →
tabs Data / Appearance / Series / Time / Actions; set a dark background, hide the title, add a right-axis series,
choose max per hour over 7 days, set ranges → the preview changes immediately; save with a reason; the old showcase
created before the upgrade looks the same.

Evidence: `test-results/e2e-dashboards.log`, `test-results/headless-widgets.log`, screenshots.
