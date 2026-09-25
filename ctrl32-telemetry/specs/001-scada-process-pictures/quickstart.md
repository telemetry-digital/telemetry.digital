# Quickstart: validate SCADA process pictures

Prerequisites: PostgreSQL 18, Go 1.27, Node 22, Chromium; see `ctrl32-telemetry/README.md` → Development.

```bash
cd ctrl32-telemetry
bash tests/run-all.sh                      # everything, fresh database
bash tests/run-all.sh unit                 # schema + sanitiser unit tests only
```

Manual walk-through on a dev server (`http://127.0.0.1:8080`, admin/admin1234):

1. Dashboards → New → "Process picture — mixing plant (demo)". The picture opens with vessels, pumps, valves,
   pipes and instruments bound to the demo device. Publish values (`go run ./tools/devpub …`): levels, colours,
   flow animation and texts change within 2 s (US1, SC-002).
2. Edit → draw a tank from the library, two pipes, a text `{value} {unit}`; bind tags; add dynamics level and
   blink; align, copy/paste, undo/redo; save with a reason → version +1, audit entry (US2, US3).
3. View → click the pump → faceplate with trend; Start with a reason → command in the audit trail, status acked
   (US4, SC-005). As a viewer without command permission: no controls.
4. New → "Process picture with widgets" → add a trend widget over the drawing (US5).
5. Library → import an SVG containing `<script>` and `onclick` → import succeeds, the response lists what was removed;
   place it; it shows as a static image (US6, FR-024).
6. Share the picture → open the public link in a private window → values visible, no commands, no navigation.

Expected automated evidence: `test-results/e2e-mimics.log`, `test-results/e2e-hmi_objects.log`,
`test-results/headless-scada.log`, `test-results/headless-scada_editor.log`, screenshots in
`test-results/screenshots/`.
