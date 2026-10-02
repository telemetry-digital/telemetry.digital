# Tasks: Licence model — free up to 4 cameras, updates included for a period

- [x] T001 Licence claim `updates_until`, online-key limit, `NoExpiry`; unit tests
- [x] T002 Release date in the binary (`internal/release`, `version`, `deploy/publish.sh`); unit tests
- [x] T003 Agent: refuse a release after `updates_until` before replacing anything (402 `updates_ended`); unit tests
- [x] T004 Server: camera limit (transaction, advisory lock, 402 `license_required`), licence/updates JSON, update passes the limit, install/activation refuse a key that does not cover this release
- [x] T005 licgen: `-updates-until`, batch with `paid_at` / `updates_until`
- [x] T006 UI texts and rows, Camera management count, 19 catalogs; OpenAPI and clients regenerated
- [x] T007 Docs: whitelabel.md, README, ADR-0016 amendment, ADR-0002/0017, URS J-16a/J-16b, threat model, constitution 1.0.1, mcp.md, public documentation
- [x] T008 Tests: e2e `video`, `whitelabel` (licfake with `updates_until`), headless `license`, docshots crops
- [ ] T009 Licence worker (lic.ctrl32.com, outside this repository): send `paid_at` in `/telemetry/queue` (optional — without it the year counts from minting)
- [ ] T010 On a real installation: an update refused by the release date (needs two published versions)
