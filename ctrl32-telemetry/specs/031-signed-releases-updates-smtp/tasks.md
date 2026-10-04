# Tasks: Signed releases, update modes, offline updates, SMTP in the UI

- [x] T001 Manifest format, parse/verify, trusted keys with rotation, version comparison (`internal/release`) + unit tests (sign/verify, tamper, rotation, refusals, compare)
- [x] T002 `tools/relsign`: keygen (outside repositories, restricted permissions), sign, verify, package, fetch-detector
- [x] T003 Generate the release key pair on the release PC; embed `release-2026.pub`
- [x] T004 Agent: verified installation from HTTP and staged sources, backup, migrations, rollback (binary; database after failed migrations), health guard, history + unit tests
- [x] T005 Agent: offline package verification/staging, size limit, CLI direct install + unit tests (refusals, limit)
- [x] T006 Detector parts from the staged package and a mirror (pins decide) + unit test
- [x] T007 CLI `update --file|verify|mirror|guard`
- [x] T008 Migration 0054: `core.server_setting`, notification event `system`
- [x] T009 Server: update modes, scheduler (daily jittered check, notices, automatic window), banner, upload endpoint (step-up, not MCP), result audit + unit tests (windows with time zones, auto rules)
- [x] T010 SMTP settings (encrypted password, precedence, security rules, test with transcript, delivery log, audit) + unit tests
- [x] T011 UI: System → Updates, System → Server → E-mail (SMTP), banner; i18n in 19 languages
- [x] T012 OpenAPI, clients, MCP exclusions
- [x] T013 `deploy/publish.sh`: signed manifest, offline package, detector parts, upload order
- [x] T014 e2e `updates.sh`, `smtp.sh`; headless `updates.js`; run-all with a per-run test key
- [x] T015 Real Linux test (Docker): 0.75 → 0.76 online, signed update, rollbacks, offline upload, CLI
- [x] T016 ADR-0024, threat model, docs (server administration, system reference, getting started, object counting)
