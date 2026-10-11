# Tasks: Offsite backups, retention and the automatic restore test

- [x] T001 `internal/offsite`: folder, SFTP with host key pinning, S3 with SigV4; validation, redaction, merge
- [x] T002 Unit tests of the targets against servers in the test process; the AWS SigV4 example
- [x] T003 Encryption in Go in the openssl `enc` format; tests both ways against openssl
- [x] T004 Manifest in the bundle, checksum file, streaming bundle writer, `extractBundle` with the manifest check
- [x] T005 GFS retention locally and at the target (own files only)
- [x] T006 Offsite copy with read-back, catch-up of missing backups, listing, handlers (set, test, sync)
- [x] T007 Restore test into `ctrl32_restoretest` with comparison; schedule (monthly) and run now
- [x] T008 History of runs with steps; acknowledgement of audited runs
- [x] T009 Web service: session + step-up for the target, reasons, audit of every result, incidents (K-03)
- [x] T010 OpenAPI, `tools/apigen`
- [x] T011 Backups page; 85 strings in 19 languages
- [x] T012 Agent simulation serves backups; e2e `backups`; headless `backups`; `run-all.sh`
- [x] T013 Real check in Docker (Debian 13, PostgreSQL 18, SFTP, S3)
- [x] T014 URS, threat model, README, JOURNAL, STAV; public documentation with screenshots
