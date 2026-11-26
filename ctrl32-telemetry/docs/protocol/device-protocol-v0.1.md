# Protokol zariadení ctrl32 telemetry — v0.1 (návrh)

**Dátum:** 23. 9. 2026 · **Stav:** návrh na diskusiu; implementuje sa vo fáze 0 (PoC) a stabilizuje v 1.0 · **Súvisí:** ADR-0006, ADR-0007

Cieľ: jeden protokol pre všetky zariadenia s knižnicou (Arduino, ESP-IDF, STM32, firmvér z ctrl32 editora) aj pre ručne napísaných MQTT/HTTP klientov. Jednoduchý na implementáciu v C bez dynamickej alokácie, bezpečný predvolene, auditovateľný.

---

## 1. Transport a spojenie

| Vlastnosť | Hodnota |
|---|---|
| Primárny transport | MQTT 3.1.1 alebo MQTT 5 cez **TLS 1.2+** (port 8883); broker beží v procese platformy |
| Alternatíva | HTTPS (REST ingest) pre zariadenia bez MQTT alebo za HTTP proxy |
| Vývojový režim | MQTT bez TLS len v profile `intranet` a len ak to správca výslovne zapne s dôvodom (auditované); profil `internet` odmietne |
| Identita klienta | `clientId = deviceId` (UUID alebo `org-prefix:hwid`), `username = deviceId`, `password = token` **alebo** mTLS s klientskym certifikátom (CN = deviceId) |
| Keep-alive | 60 s predvolene (konfigurovateľné shared atribútom), LWT správa na `d/{id}/status` s `{"online": false}` |
| QoS | telemetria, atribúty, príkazy, FOTA: **QoS 1**; konzola: QoS 0 |
| Retained | server publikuje shared atribúty a `fw.target` ako retained; zariadenie nepublikuje retained |
| Veľkosti | payload telemetrie ≤ 16 KiB; dávka ≤ 64 KiB; príkaz ≤ 4 KiB; konzolový riadok ≤ 512 B (server vynucuje, prekročenie = odmietnutie + záznam) |
| Časový zdroj | zariadenie s RTC/NTP posiela `ts`; bez neho `ts` vynechá a server doplní `received_at` (`time_source = server`) |

## 2. Témy

Všetky témy zariadenia sú pod prefixom `d/{deviceId}/`. ACL: zariadenie smie publikovať len do vlastných `…/telemetry|attributes|status|cmd/ack|console/out|fota/status`, odoberať len vlastné `…/attributes/shared|cmd|console/in|fota/target`. Wildcard subscribe zariadeniam nie je dovolený.

| Téma | Smer | Účel |
|---|---|---|
| `d/{id}/telemetry` | Z → S | merania (jednotlivé alebo dávka) |
| `d/{id}/attributes` | Z → S | client atribúty (firmvér, model, hw id, IP…) |
| `d/{id}/attributes/shared` | S → Z (retained) | konfigurácia pre zariadenie (interval, limity, `console.enabled`, `fw.target`…) |
| `d/{id}/attributes/request` / `…/response` | Z ↔ S | vyžiadanie aktuálnych shared atribútov po štarte |
| `d/{id}/status` | Z → S | heartbeat: online, batéria, RSSI, uptime, voľná pamäť, verzia fw; LWT `online:false` |
| `d/{id}/cmd` | S → Z | príkaz (RPC) |
| `d/{id}/cmd/ack` | Z → S | potvrdenie/odpoveď na príkaz |
| `d/{id}/console/in` | S → Z | riadok pre konzolu zariadenia (len ak `console.enabled`) |
| `d/{id}/console/out` | Z → S | výstup konzoly / logy zariadenia |
| `d/{id}/fota/target` | S → Z (retained) | oznámenie cieľového firmvéru |
| `d/{id}/fota/status` | Z → S | priebeh a výsledok aktualizácie |
| `d/{id}/fota/chunk/req` / `…/chunk` | Z ↔ S | prenos firmvéru po blokoch cez MQTT (ak zariadenie nemá HTTPS) |
| `v1/devices/me/telemetry`, `v1/devices/me/attributes` | Z → S | **alias v štýle ThingsBoard** pre migráciu existujúceho kódu (server mapuje na `d/{id}/…` podľa identity klienta) |

## 3. Formát správ

Predvolene **JSON**; voliteľne **CBOR** s rovnakou štruktúrou (kľúče ako reťazce; content-type sa určuje prvým bajtom alebo MQTT 5 vlastnosťou `Content-Type: application/cbor`). Každá správa má voliteľné pole `"p": 1` (verzia protokolu); chýbajúce = 1.

### 3.1 Telemetria

Jedno meranie:
```json
{"ts": 1758620000, "v": {"temp": 4.2, "rh": 61.0, "door": 0}}
```
Dávka / backfill (pole; poradie ľubovoľné, server triedi podľa `ts`):
```json
[{"ts": 1758619400, "v": {"temp": 4.1}, "bf": 1},
 {"ts": 1758620000, "v": {"temp": 4.2}}]
```
- `ts` = Unix čas v sekundách (celé číslo) alebo milisekundách (`"tsm"`); bez `ts` = čas prijatia.
- `v` = mapa kľúč → hodnota; kľúč je názov datastreamu podľa profilu (`^[a-z][a-z0-9_]{0,31}$`); hodnota číslo, bool, reťazec (≤ 256 B) alebo objekt (stav/GPS).
- **Ďalšie prijímané tvary (od 0.36.0, pre cudzie zariadenia):** ThingsBoard `{"ts": <ms>, "values": {…}}` a plochý objekt `{"temp": 4.2, "rh": 61}` (každý nerezervovaný kľúč je hodnota; rezervované: `p ts tsm dt bf seq v values`), aj v poli a zmiešane. `ts` nad 10 000 000 000 sa berie ako milisekundy. Kľúče sa normalizujú (malé písmená, `-`/`.`/medzera → `_`); kľúč, ktorý sa normalizovať nedá, odmietne správu.
- `bf: 1` označuje doplnenie z bufferu (server nastaví `quality = backfilled`, medzeru uzavrie a vyhodnotí retrospektívne alarmy).
- **Porucha snímača (od 0.67.0):** `fault` vo vzorke označí hodnoty z chybného snímača — zoznam kľúčov `"fault": ["temp"]`, objekt `{"temp": "open circuit"}` (`false`/`0`/`null` = bez poruchy) alebo vedľa `v`/`values` `"fault": true` pre všetky hodnoty vzorky. Server ich uloží s `quality = sensor_fault`. V plochom objekte je `fault` príznakom len ako zoznam alebo objekt; číslo, bool či text ostáva obyčajnou hodnotou s kľúčom `fault`. `sensor_fault` dostane aj číslo mimo fyzikálneho rozsahu datastreamu (`attributes.range_min`/`range_max`, nastaviteľné v UI). Takú hodnotu preskočia hodnotové alarmové pravidlá (high/low/…), vyhodnotí ju pravidlo `sensor_fault`.
- `seq` (voliteľné, monotónne číslo správy) umožňuje serveru deduplikovať opakované odoslanie po strate PUBACK.
- Neznámy kľúč: server uloží surovú správu, meranie zahodí s chybou viditeľnou v prehliadači správ (nevytvára datastreamy automaticky, pokiaľ to profil nedovolí — `auto_create: true` je vedomá voľba implementátora).

### 3.2 Atribúty

Client → server: `{"fw": "1.4.2", "hw": "esp32-s3-devkitc", "model": "ctrl32-tlog", "ip": "10.0.0.12"}`
Server → zariadenie (retained): `{"interval_s": 300, "temp_hi": 8.0, "console.enabled": false, "tz": "Europe/Bratislava"}`
Po pripojení zariadenie pošle `attributes/request` s `{"keys": ["interval_s","temp_hi"]}` alebo prázdne `{}` = všetky; server odpovie na `…/response`.

### 3.3 Stav (heartbeat)

```json
{"online": true, "bat": 3.61, "bat_pct": 78, "rssi": -71, "up_s": 86400, "free": 120000, "fw": "1.4.2", "buf": 12}
```
`buf` = počet záznamov čakajúcich v offline bufferi (server z toho odvodzuje stav spojenia a očakávaný backfill). Interval heartbeatu shared atribútom `hb_s` (predvolene 300 s).

### 3.4 Príkazy (RPC)

Server → `d/{id}/cmd`:
```json
{"id": "c-01J9…", "m": "set_interval", "a": {"s": 60}, "exp": 1758620600}
```
Zariadenie → `d/{id}/cmd/ack`:
```json
{"id": "c-01J9…", "ok": true, "r": {"s": 60}}
```
alebo `{"id": "…", "ok": false, "err": "unsupported"}`. `exp` = čas, po ktorom sa príkaz nemá vykonať (zariadenie porovná s vlastným časom, ak ho má). Server eviduje stavy `queued → sent → acked/failed/expired`, každý príkaz v audit traile s identitou používateľa alebo pravidla, ktoré ho vyvolalo. Rezervované metódy knižnice: `ping`, `reboot`, `get_attrs`, `set_interval`, `fota_check`, `console_on`, `console_off`; ostatné definuje firmvér.

### 3.5 Konzola

Aktivácia: shared atribút `console.enabled: true` (server ho nastaví pri otvorení relácie a zruší pri zatvorení/timeoute). Kým je `false`, knižnica konzolu neinicializuje a `console/in` ignoruje.
- `console/in`: `{"s": "sess-…", "l": "wifi status"}` (jeden riadok; ≤ 512 B; rate limit 5/s).
- `console/out`: `{"s": "sess-…", "t": 1758620001, "l": "RSSI -71 dBm, ch 6"}`; logy firmvéru mimo relácie sa **neposielajú** (šetrenie linky), iba ak `console.log_level` ≥ nastavená úroveň.
- Server ukladá každý `in` aj `out` riadok do `console_log` (append-only) s identitou operátora; výstup filtruje na kontrolné znaky pred zobrazením.

### 3.6 FOTA

1. Server publikuje retained `d/{id}/fota/target`:
   ```json
   {"fw_id": "f-01J9…", "ver": "1.5.0", "hw": "esp32-s3-devkitc", "size": 1439232,
    "sha256": "…", "sig": "base64(ed25519)", "kid": "k1",
    "url": "https://srv/api/v1/fota/f-01J9…/blob", "min_ver": "1.3.0", "campaign": "cmp-…"}
   ```
2. Zariadenie overí `hw` a `min_ver`, stiahne binár cez HTTPS s `Authorization: Bearer <token>` a `Range` (obnoviteľné), alebo požiada o bloky cez `fota/chunk/req` `{"fw_id": "…", "off": 0, "len": 4096}` → `fota/chunk` (binárny payload s hlavičkou `off`).
3. Počas sťahovania priebežne zapisuje do druhého slotu; po dokončení overí **SHA-256 a Ed25519 podpis** vstavaným verejným kľúčom (`kid` vyberá kľúč; dva kľúče vo firmvéri pre rotáciu).
4. `fota/status`: `{"fw_id": "…", "st": "downloading", "pct": 42}` → `verifying` → `applying` → reštart → nová verzia pošle `{"st": "updated", "ver": "1.5.0"}`; pri zlyhaní `{"st": "failed", "err": "sig_invalid"}`; pri rollbacku `{"st": "rolled_back", "ver": "1.4.2"}`.
   > **Stav implementácie (26. 11. 2026):** knižnica `ctrl32-telemetry-c` (porty ESP-IDF a Arduino) overuje zatiaľ len SHA-256;
   > polia `sig` a `kid` neparsuje a podpis neoveruje. Server posiela `sig` ako 128 hexadecimálnych znakov a `kid` neposiela.
   > Do zosúladenia nepoužívať automatickú FOTA tam, kde serveru alebo ceste k nemu nemožno plne dôverovať.
5. Rollback: nová verzia sa musí do `confirm_s` (predvolene 120 s) ohlásiť serveru heartbeatom, inak bootloader/rollback logika vráti predchádzajúci slot (ESP-IDF app rollback; STM32 referenčný bootloader).

### 3.7 Provisioning

- **Claim kód:** v UI (alebo ctrl32 editor cez API) vznikne zariadenie s jednorazovým claim kódom (platnosť napr. 24 h). Zariadenie pri prvom štarte pošle `POST /api/v1/provision` `{"claim": "…", "hw": "…", "fw": "…"}` → odpoveď `{"device_id": "…", "token": "…", "mqtt": {"host": "…", "port": 8883}, "ca": "-----BEGIN…"}`; claim kód sa zneplatní. Token uloží do NVS/flash.
- **Predgenerovaný token:** implementátor vloží token do konfigurácie firmvéru (ctrl32 editor to robí automaticky).
- **mTLS:** klientsky certifikát vydaný platformou (interná CA) alebo cudzí s registrovaným odtlačkom.
- Rotácia tokenu: príkaz `rotate_token` s novým tokenom v `a`, zariadenie potvrdí ack až po uložení; starý token platí do ack + 60 s.

### 3.8 HTTP alternatíva

- **Implementované v 0.36.0** (`docs/integration/device-integration.md` má príklady): `POST /api/v1/d/telemetry` (Bearer token zariadenia, JSON ako v 3.1), `POST /api/v1/d/attributes`, `POST /api/v1/d/status`, `GET /api/v1/d/attributes/shared`, `GET /api/v1/d/cmd?wait=30` (long-poll do 60 s; zariadenie s transportom `http` dostáva príkazy len takto), `POST /api/v1/d/cmd/{id}/ack`, `POST /api/v1/d/fota/status`, `GET /api/v1/d/fota/{fw_id}/blob`.
- Odpovede: `202 Accepted` s `{"accepted": n, "rejected": [{"i": 3, "err": "unknown_key"}]}`.

## 4. Chyby a odolnosť

- Server nikdy nezahodí správu bez stopy: neplatná správa sa uloží ako `raw_message` s `decode_error` a je viditeľná v prehliadači správ; autentifikačné zlyhania idú do bezpečnostného logu.
- Knižnica: exponenciálny backoff pri reconnecte (1 s → 5 min), offline kruhový buffer (RAM/flash) s konfigurovateľnou kapacitou, po obnove odošle dávky s `bf: 1` v poradí od najstaršej, heartbeat aj počas backfillu.
- Hodiny: ak zariadenie nemá platný čas (pred NTP), posiela bez `ts`; po synchronizácii môže bufferované záznamy označiť relatívne (`"dt": -300` = sekundy pred odoslaním) — server prepočíta a nastaví `time_source = device_relative`.

## 5. Verziovanie

- `p` (verzia protokolu) v správe; server podporuje aspoň dve po sebe idúce verzie; zmeny sú aditívne, odstránenie poľa = nová major verzia.
- Profil zariadenia na serveri deklaruje očakávané kľúče, jednotky, `kind` a limity; zmena profilu je verziovaná (ADR-0005).

## 6. Rozhranie platforma ↔ runtime (výhľad, ADR-0008)

Rovnaký broker, prefix `rt/{runtimeId}/`: tagy s kvalitou a časom zdroja v dávkach (Sparkplug-B-like: `NBIRTH/NDATA/NDEATH` sémantika, metriky s aliasmi), príkazy s ack, stav redundancie. Definuje sa v protokole v0.2 pred fázou 3.

## 7. Otvorené otázky

- CBOR povinne pre STM32 s modemom (úspora dát), alebo voliteľne? (návrh: voliteľne, JSON predvolene)
- Maximálna kapacita bufferu v knižnici pre ESP32 bez externej flash (návrh: 2 000 záznamov v NVS/LittleFS)
- Či server smie `auto_create` datastreamy z neznámych kľúčov vo vývojovom režime (návrh: áno, len v profile `intranet` a len s príznakom v profile zariadenia)
