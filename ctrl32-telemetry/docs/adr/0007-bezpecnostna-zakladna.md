# ADR-0007: Bezpečnostná a logovacia základňa od prvého commitu

**Stav:** rozhodnuté 23. 9. 2026 (používateľ: „na bezpečnosť, audit, logy treba myslieť od začiatku")

## Rozhodnutie

Bezpečnosť je sada **nefunkčných požiadaviek s testami v regresnej sade** (URS kategória H a I), nie modul.

- **Bezpečné predvoľby:** žiadne predvolené heslá; TLS povinné v profile `internet`; HSTS, CSP, X-Frame-Options, Referrer-Policy; cookies `HttpOnly; Secure; SameSite=Strict`; CSRF; limity veľkosti; rate limiting (prihlásenie, API, MQTT CONNECT); dočasné blokovanie IP; MQTT ACL na vlastné témy, bez anonymného prístupu.
- **Identita:** osobné účty, Argon2id (m = 19 MiB, t = 2, p = 1, pepper mimo DB), TOTP povinné pre správcov/QA a v profile `internet` pre všetkých, WebAuthn ako druhý faktor, zamknutie po N pokusoch, timeout nečinnosti len správcom, rotácia ID relácie, história rolí, API tokeny s rozsahom a expiráciou.
- **Tajomstvá:** šifrované v DB (AES-GCM, kľúč mimo DB); tokeny hashované; žiadne tajomstvá v logoch.
- **Najmenšie oprávnenia:** služba pod vlastným OS účtom; DB rola bez DDL a bez UPDATE/DELETE na záznamoch.
- **Vstupy:** validácia schémou na API aj ingeste; JS dekodéry (ak budú) v sandboxe s limitmi a bez I/O.
- **Dodávateľský reťazec:** minimálne pinované závislosti; `govulncheck` a SBOM (CycloneDX) v každom release; podpísané artefakty (Windows certifikátom ctrl32, ostatné cosign/Ed25519); reprodukovateľný build.
- **Proces:** `SECURITY.md` a `security.txt` (hlásenie zraniteľností, lehoty, bulletiny); model hrozieb v repe (`docs/threat-model.md`) aktualizovaný pri každej zmene povrchu; externý penetračný test pred 1.0 a 2.0 so zverejneným súhrnom.
- **FOTA:** firmvér podpisuje autor mimo servera; zariadenie overuje podpis aj hash; postupné nasadenie s automatickým zastavením.
- **Konzola:** predvolene vypnutá; zapnutie per zariadenie; MFA pri otvorení; plný append-only log; žiadny shell na server.
- **Logy v troch vrstvách:** (1) GxP záznamy — append-only, bez rotácie, hash-reťazec; (2) systémový log — štruktúrovaný JSON, korelačné ID, bez tajomstiev/PII, rotácia, export syslog/OTLP/súbor; (3) bezpečnostné udalosti — v (1) aj v (2) s príznakom `security=true` a voliteľnou notifikáciou.
- **Zodpovednosť nasadzovateľa:** licencia bez záruky + dokument + sprievodca hardeningom; UI upozorňuje na zastaranú verziu a nedodržané odporúčania.

## Dôsledky

- Žiadny release bez zelených bezpečnostných testov, SBOM a podpisu.
- Jazyk platformy je pamäťovo bezpečný (ADR-0001) — je to súčasť tejto základne, nie vedľajší efekt.
