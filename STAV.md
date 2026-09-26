# Stav práce (aktualizované 26. 9. 2026)

## Hotové (na vetve `assistant/jolly-sagan-nzolwc`, otestované)

| Verzia | Čo | Testy |
|---|---|---|
| 0.37.0 | SCADA procesné obrazovky a editor (spec 001) | e2e + headless |
| 0.38.0 | Widgety dashboardov s nastaveniami ako ThingsBoard, filtre v grafoch, plná nastaviteľnosť (spec 002) | e2e 81 + headless |
| 0.39.0 | Redundantná databáza: kópia s bezpečným odpojením, priebežná replika, hot standby s failoverom (spec 003) | e2e 46 + headless |
| 0.40.0 | Toky v štýle Node-RED: vlastný engine v Go, 28 uzlov, editor, ladenie, verzie, štyri oči, prevod pravidiel, metriky, sk/cs (spec 004) | e2e 69 + headless, ~30 000 správ/s |
| 0.41.0 | Provisioning cez MQTT a smart home ako Home Assistant: profily provisioningu, ThingsBoard API, automatické datastreamy, MQTT účty s oddeleným priestorom tém, mosty, HA discovery, entity, stránky Domov a Zariadenia a pripojenia, uzly tokov, export do HA (spec 005) | e2e 33 + 39, headless, unit |

Celá sada testov (`tests/run-all.sh`) prešla pre 0.41.0 bez chyby (26. 9. 2026).

## Ďalšie možnosti (fáza 2)

- smart home: Shelly Gen2 (natívne RPC), natívny formát discovery Tasmoty, kamery, výber farby na karte, incident pri
  výpadku mosta
- toky: MQTT cudzieho brokera ako spúšťač, rozdelenie správ, podtoky, skupiny na plátne
- replika: prihlásenie na replike na prezeranie; Windows: test inštalátora na čistom serveri, podpis binárky

## Poznámka k prostrediu

- Lokálna testovacia DB: PostgreSQL 16 na porte 5433 (rola `ctrl32`), po reštarte kontajnera ho treba znova spustiť
  (`pg_ctl … -p 5433 -k /tmp/pgsock`). Povolenie od používateľa: 26. 9. 2026.

## Čaká na používateľa

- nasadenie na telemetry.digital (v produkcii je 0.36.3): `deploy/publish.sh 0.41.0 root@2.29.5.17 -i <kľúč>` alebo
  GitHub workflow (secrets `DEPLOY_SSH_KEY`, `DEPLOY_KNOWN_HOSTS` a workflow na `main`)
- zlúčenie vetvy do `main` (pull request na požiadanie)
- potvrdenie ADR-0015 (SCADA editor v čistom JS nad SVG)
