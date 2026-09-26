# Stav práce (aktualizované 26. 9. 2026)

## Hotové (na vetve `assistant/jolly-sagan-nzolwc`, otestované)

| Verzia | Čo | Testy |
|---|---|---|
| 0.37.0 | SCADA procesné obrazovky a editor (spec 001) | e2e + headless |
| 0.38.0 | Widgety dashboardov s nastaveniami ako ThingsBoard, filtre v grafoch, plná nastaviteľnosť (spec 002) | e2e 81 + headless |
| 0.39.0 | Redundantná databáza: kópia s bezpečným odpojením, priebežná replika, hot standby s failoverom (spec 003) | e2e 46 + headless |
| 0.40.0 | Toky v štýle Node-RED: vlastný engine v Go, 28 uzlov, editor, ladenie, verzie, štyri oči, prevod pravidiel, metriky, sk/cs (spec 004) | e2e 69 + headless, ~30 000 správ/s |

Celá sada testov (`tests/run-all.sh`) prešla pre 0.40.0 bez chyby.

## Rozpracované: spec 005 — provisioning cez MQTT a smart home (ako Home Assistant)

Hotové a overené naživo (simulátory zigbee2mqtt, Tasmota, ThingsBoard zariadenie, cudzí broker):
- profily provisioningu, registrácia zariadení cez MQTT (`/provision/request`) aj HTTP, schvaľovanie, limity
- ThingsBoard zariadenia: prihlásenie tokenom, telemetria, atribúty, RPC príkazy s potvrdením
- automatické datastreamy z kľúčov telemetrie
- MQTT účty s oddeleným priestorom tém (organizácie sa navzájom nevidia)
- mosty na existujúci broker (napr. Mosquitto zo zigbee2mqtt), inbox objavených zariadení
- Home Assistant discovery → zariadenia, miestnosti, entity, stavy s históriou
- ovládanie entít (vypínače, stmievanie, žalúzie, termostaty, zámky, …) ako auditované príkazy s potvrdením stavu
- stránka **Domov** a stránka **Zariadenia a pripojenia** (sprievodca, inbox, účty, mosty, provisioning)
- uzly tokov: Entita, Akcia s entitou, Výsledok príkazu
- zdieľanie hodnôt ctrl32 do Home Assistant cez most

Zostáva:
1. e2e testy `provisioning` a `smarthome`, headless test stránky Domov (T018, T019)
2. preklady sk/cs, `docs/smarthome.md`, URS, JOURNAL, README, celá sada testov (T020)

## Poznámka k prostrediu

- Lokálna testovacia DB: PostgreSQL 16 na porte 5433 (rola `ctrl32`), po reštarte kontajnera ho treba znova spustiť
  (`pg_ctl … -p 5433 -k /tmp/pgsock`). Povolenie od používateľa: 26. 9. 2026.

## Čaká na používateľa

- nasadenie na telemetry.digital (v produkcii je 0.36.3): `deploy/publish.sh 0.40.0 root@2.29.5.17 -i <kľúč>` alebo
  GitHub workflow (secrets `DEPLOY_SSH_KEY`, `DEPLOY_KNOWN_HOSTS` a workflow na `main`)
- zlúčenie vetvy do `main` (pull request na požiadanie)
- potvrdenie ADR-0015 (SCADA editor v čistom JS nad SVG)
