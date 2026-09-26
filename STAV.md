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

Hotové (unit testy, bez databázy):
- špecifikácia, plán, research, dátový model, API, 20 úloh (`ctrl32-telemetry/specs/005-mqtt-provisioning-smarthome/`)
- migrácia 0024 (profily provisioningu, entity, inbox objavených zariadení, MQTT účty a mosty) — ešte nespustená na DB
- Home Assistant discovery: skratky, `~`, zariadenie, dostupnosť, discovery celého zariadenia
- šablóny `value_template` (bezpečná podmnožina Jinja)
- stavy a príkazy entít: switch, light (aj JSON), cover, fan, lock, climate, number, select, text, button, scene, …
- broker: MQTT účty s oddeleným priestorom tém, ThingsBoard prihlásenie tokenom, RPC len pre dané zariadenie,
  MQTT provisioning s odpoveďou len žiadateľovi

Zostáva:
1. profily provisioningu + HTTP/MQTT provisioning + schvaľovanie zariadení (T005, T008)
2. automatické datastreamy z kľúčov telemetrie, ThingsBoard RPC a atribúty v ingeste (T007)
3. správca smart home: objavovanie, inbox, prijatie zariadení, miestnosti, entity, ukladanie stavov (T010)
4. mosty na cudzí broker (napr. Mosquitto zo zigbee2mqtt) (T011)
5. API pre zdroje, inbox, entity, stránku Domov (T012)
6. ovládanie entít cez príkazy s auditom a potvrdením stavom (T013)
7. stránka **Domov**: miestnosti, karty, vypínače, stmievanie, žalúzie, termostaty, živé stavy (T014)
8. sprievodca pridaním zariadenia (Tasmota, zigbee2mqtt, ESPHome, ThingsBoard, mosty) (T015)
9. uzly tokov: entita, akcia entity, výsledok príkazu (T016)
10. zdieľanie hodnôt ctrl32 do Home Assistant (T017)
11. simulátor `tools/mqttsim`, e2e testy, headless test, preklady, dokumentácia, JOURNAL/URS/README (T018–T020)

## Blokované

- **Lokálna testovacia databáza**: kontajner sa obnovil a PostgreSQL prišiel o rolu `ctrl32`. Jej vytvorenie
  (superužívateľ v lokálnej testovacej DB) klasifikátor oprávnení zamietol. Treba povolenie alebo doplniť
  inicializačný skript prostredia (návod v poslednej správe v chate). Bez toho nejdú e2e testy ani snímky obrazovky.

## Čaká na používateľa

- nasadenie na telemetry.digital (v produkcii je 0.36.3): `deploy/publish.sh 0.40.0 root@2.29.5.17 -i <kľúč>` alebo
  GitHub workflow (secrets `DEPLOY_SSH_KEY`, `DEPLOY_KNOWN_HOSTS` a workflow na `main`)
- zlúčenie vetvy do `main` (pull request na požiadanie)
- potvrdenie ADR-0015 (SCADA editor v čistom JS nad SVG)
