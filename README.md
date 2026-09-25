# telemetry.digital

Private repository of telemetry.digital (Palo Krnáč). It holds the ctrl32 telemetry platform and its device library.

| Folder | Content |
|---|---|
| `ctrl32-telemetry/` | The server: Go 1.27 + PostgreSQL 18, web UI, installers (Linux, Windows), documentation (`docs/`), journal (`JOURNAL.md`), API clients. Production instance: https://telemetry.digital/ |
| `ctrl32-telemetry-c/` | Device client library: portable C core, Arduino and ESP-IDF ports, examples, host tests. |

Each folder is a snapshot of its own development repository; start with `ctrl32-telemetry/README.md`.
The source code stays private until version 1.0 (ADR-0002). Secrets, local configuration, runtime data and build
outputs are never committed.

The previous content of this repository (legacy HMI installation and the previous deployment deployment files) was removed on
25 Sep 2026; it remains in the history of this repository.
