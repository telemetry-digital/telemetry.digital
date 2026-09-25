# Technický research pre ctrl32 telemetry — zhrnutie a závery

**Dátum:** 23. 9. 2026 · Plné reporty (EN, s URL ku každému faktu) sú v `01-technicky-research/`: `platformy-a-licencie.md`, `stack.md`, `hardver.md`. Tento súbor je slovenské zhrnutie toho, čo z nich plynie pre návrh (`02-navrh-systemu.md`).

Všetky verzie a licencie nižšie boli overené živým načítaním zdrojov 23. 9. 2026. Položky označené ⚠️ sa overiť nepodarilo.

---

## 1. Postaviť sa na ThingsBoard, alebo nie?

| Zistenie | Dôsledok |
|---|---|
| ThingsBoard CE 4.3.1.5 (09/2026), Apache-2.0, Java + Angular; PE od $10/mesiac po $499/mesiac (1 000 zariadení), perpetual od $4 999 | licenčne dostupný, ale ťažký (Java, Kafka/Cassandra pri raste) |
| PE EULA zakazuje „installations of the Software as a service" a nemá OEM/white-label klauzulu | **ďalší predaj PE pod vlastnou značkou bez osobitnej zmluvy nie je možný** |
| Telemetria **nie je append-only**: druhý zápis s rovnakým (entity, key, ts) prepíše hodnotu („last wins"), TTL job fyzicky maže, REST API maže rozsahy histórie | v rozpore s ALCOA+ „Original"; nedá sa opraviť konfiguráciou |
| Audit log zaznamenáva CRUD entít a prihlásenia, **nie zápisy telemetrie**, bez hash-reťazca, bez ochrany proti DB-level výmazu | GxP audit trail by sa musel dorobiť mimo TB |
| White-label, reporty, scheduler, LoRaWAN integrácie, 400+ konvertorov, CSV export = **len PE** | to, čo lekáreň potrebuje, je platené a nepredajné ďalej |
| Žiadne GxP / 21 CFR Part 11 pozicionovanie, žiadny e-podpis ⚠️ (negatívny nález) | |

**Záver (R1):** stavať vlastné jadro; z ThingsBoardu prevziať **koncepty** (tenant/customer/device/asset, attributes vs. telemetry, device profile, rule chain, dashboard/widget), nie kód. Alternatívy (Magistrala 1.0 Apache-2.0, OpenRemote AGPL, Kaa mŕtvy, Thinger uzavretý, Eclipse Ditto/Hono ťažké, FROST-Server LGPL) to potvrdzujú — žiadna nemá append-only podpísané jadro a najkompletnejšia (OpenRemote) je AGPL. FROST-Server je ale užitočný ako **referenčný dátový model** (OGC SensorThings: Thing / Datastream / Observation / Sensor), ktorý preberáme do návrhu.

## 2. LoRaWAN network server

- **ChirpStack v4.19.2** (09/2026), **MIT**, Rust, jeden binár (NS + AS), **SQLite režim bez PostgreSQL**, Redis ≥ 6.2 a MQTT v5 broker so zdieľanými subscriptions; tenanti; integrácie MQTT/HTTP/gRPC; JS kodeky v QuickJS. Dokumentovaná inštalácia: Debian/Ubuntu balíky a Docker — **natívne Windows binárky sa v dokumentácii nenašli** ⚠️.
- The Things Stack open source: podľa TTI „not suitable for production".
- ChirpStack Gateway OS beží celý NS priamo na gatewayi (licencia repozitára ⚠️).

**Záver:** ChirpStack len pre Linux prevádzky s viac gatewayami; **default pre lekárne a Windows prevádzky je gateway so vstavaným NS** (Milesight, RAK, Dragino LPS8v2 má vstavaný ChirpStack), ktorý posiela uplinky MQTT/HTTP do jadra. Dekodéry držíme v jadre (Go, verziované), nie v QuickJS.

## 3. MQTT broker

| Broker | Licencia | Vhodný do produktu? |
|---|---|---|
| EMQX ≥ 5.9 (6.3.1) | **BSL 1.1** — integrácia do produktu pre tretie strany vyžaduje komerčnú licenciu | **nie** |
| Mosquitto 2.1.2 | EPL-2.0 / EDL-1.0 | áno (sidecar) |
| NanoMQ 0.25.6 | MIT | áno |
| HiveMQ CE 2026.5 | Apache-2.0 | áno, ale JVM |
| **mochi-mqtt v2** (Go) | **MIT**; MQTT v5 + 3.1.1, TLS, WebSocket, **zdieľané subscriptions** (overené 23. 9.), persistence hooky (Badger/Pebble/Redis) | **áno — v procese, bez samostatnej služby** |

**Záver:** mochi-mqtt v binári: zariadenia (ESP32/ctrl32), ChirpStack aj prehliadač (MQTT-over-WS) na jednom brokeri.

## 4. Databáza a časové rady

- **PostgreSQL 18.6**, podpora do **14. 11. 2030**; PG 19 ešte v beta (GA odsunuté na koniec 10/2026). PG 18 zlepšuje plánovanie nad mnohými partíciami. Licencia dovoľuje pribaliť EDB zip binárky do Windows inštalátora (`initdb`, `pg_ctl register`).
- **pg_partman 5.5.0** (PostgreSQL License, PG 18) — mesačné partície, retencia odpojením partície (nie výmazom).
- **TimescaleDB 2.30.1**: Apache edícia bez kompresie; TSL edícia dovoľuje embedding, ale zákazník musí mať **zakázané meniť schému** a DBaaS je zakázané → do produktu nepatrí.
- InfluxDB 3 Core limituje dotazy na ~72 h histórie — nepoužiteľné; QuestDB/VictoriaMetrics = ďalšia DB bez prínosu pri našich objemoch.
- Zálohy: **pgBackRest MIT**, WAL-G Apache-2.0, Barman GPLv3. Šifrovanie v pokoji: PostgreSQL nemá TDE → BitLocker/LUKS + TLS + `pgcrypto` na tajomstvá + šifrované zálohy.
- Multi-tenant RLS je k dispozícii, ale pri „jedna inštalácia = jedna firma" ju nepotrebujeme.

**Záver:** čistý PostgreSQL 18 s partíciami, BRIN, bez UPDATE/DELETE pre aplikačnú rolu. Objem firmy (10⁷–10⁹ riadkov/rok) je pre PG malý.

## 5. Audit trail a PDF

- **immudb** je od v1.9 **BSL 1.1** („embedded basis" sporné) a druhá DB — nie.
- Audit trail **v PostgreSQL**: `REVOKE UPDATE, DELETE, TRUNCATE`, BEFORE triggery (vrátane statement triggeru na TRUNCATE), `hash = sha256(payload ‖ prev_hash)` v triggeri, gapless `seq`, periodický **kontrolný bod** podpísaný Ed25519 nad `(seq, head_hash)`, ukotvenie mimo DB (hash v mesačnom PDF, e-mail na QA, voliteľne RFC 3161), príkaz `audit verify`. ~300 riadkov Go. Trillian/Tessera/Rekor sú referenčné dizajny, nie závislosti.
- **Typst 0.15.1**, Apache-2.0, jeden statický binár (~30 MB), PDF/A-1b…A-4, plná diakritika, deterministický výstup; z Go cez go-typst (MIT). Gotenberg/Chromium len hostované; wkhtmltopdf archivované s CVE 9.8; QuestPDF (.NET) má licenčnú pascu pre verejný sektor.

## 6. Backend, frontend, knižnice

| Backend | Skóre | Prečo |
|---|---|---|
| **Go 1.27** | 9/10 | jeden statický binár pre Linux aj Windows z jedného CI, Go 1 kompatibilitný sľub, reprodukovateľný toolchain, minimálne závislosti; mochi-mqtt, pgx 5.9, pquerna/otp, x/crypto/argon2, go-webauthn, go-mail, gocron, go-i18n, kardianos/service (Windows služba + systemd); OPC UA `gopcua/opcua` MIT (push 09/2026), Modbus `simonvetter/modbus` MIT (push 07/2026) |
| .NET 10 LTS | 8/10 | výborný na Windows, single-file publish; LTS každé 2 roky (~3 migrácie za 10 rokov), natívne knižnice sa rozbaľujú na disk, QuestPDF licencia |
| Rust 1.98 | 7/10 | korektnosť, natívny Typst; pomalý vývoj pre jedného, sqlx 0.9 lámal API, cross-compile cez cargo-xwin |
| Java/Kotlin | 6/10 | JRE, Spring Boot minor ~13 mesiacov podpory, GraalVM vyňatý z Java SE |
| Node/Bun | 5/10 | npm churn, Node SEA experimentálne, aedes len MQTT 3.1.1 |
| Elixir | 5/10 | bez zrelého embedded brokera, Windows služba nezvyčajná |

Frontend: **Lit 3.3.3** (BSD-3, minimálne breaking changes od 2.x), **TypeScript 7.0**, **Vite 8.0**; grafy **uPlot 1.6.32** (MIT, 166 650 bodov za 25 ms), ECharts 6.1.0 (Apache-2.0) len na gauge/heatmapy; Svelte 5 produktívnejší, ale kompilátor je závislosť; Solid 2 v beta.

Notifikácie z SR: SMS **GoSMS od €0,0145**, **BulkGate €0,026** (Orange/Telekom) za SMS bez DPH; Twilio $0,0883/SMS a hlas $0,1158/min na SK mobil; Web Push na iOS len pre aplikácie pridané na plochu; e-mail Postmark $15/10 000, SES $0,10/1 000. Bezpečnosť: Argon2id m = 19 MiB, t = 2, p = 1 (OWASP); relácie server-side, re-autentifikácia pred citlivými operáciami.

## 7. Hardvér (senzory, gatewaye, kalibrácia)

> Poznámka k rozsahu (23. 9. 2026): systém je bezplatný a hardvér je ľubovoľný — my hardvér nepredávame ani nekalibrujeme. Táto kapitola slúži ako **referenčný zoznam** pre katalóg profilov (ktoré komoditné senzory majú overené bufferovanie a backfill) a ako podklad pre implementátorov; ceny a „kity" sú orientačné, nie ponuka.

**Právny rámec, ktorý vyberá hardvér:** vyhláška 129/2012 § 30 ods. 8 odkazuje pri „kalibrovaného meradla" poznámkou 7a na **§ 17 zákona 157/2018 Z. z. o metrológii** (v znení 58/2022; poznámka aj § 17 overené doslovne 23. 9. 2026). § 17: používateľ je povinný viesť **evidenciu každého povinne kalibrovaného meradla** s miestom používania, intervalom kalibrácie a dátumami kalibrácie a uchovávať doklad (ods. 1 písm. d–e); interval si určuje používateľ, ak osobitný predpis neurčí inak (písm. c); **kalibráciu vykonáva SMÚ alebo akreditované kalibračné laboratórium** (ods. 3). To presne popisuje náš register kalibrácií a znamená, že neakreditovaný „kalibračný list" pre lekáreň nestačí.

**Senzory LoRaWAN s pamäťou a backfillom (S&F):**

| Použitie | Odporúčaný model | Presnosť | Cena bez DPH |
|---|---|---|---|
| Chladnička 2–8 °C, sonda dnu, jednotka s LCD vonku | **Milesight TS301/TS302** + sonda SP-AF10 (PT100 trieda A ±0,4; 10 000 záznamov, retransmisia + vyžiadanie) | ±0,4 | €77,90–88,90 + €17,90/sonda |
| Chladnička, celá jednotka dnu (IP67, potravinársky kryt) | **Milesight EM320-TH** (3 000 záznamov) | ±0,2 (0..60) | €50,90 |
| Miestnosť T + RH | **Milesight EM300-TH** (2 800 záznamov) | ±0,3 | €46,90 |
| Lacná alternatíva s externou sondou | Dragino LHT65N-E3 (3 328 záznamov, nutné PNACKMD=1 + SYNCMOD=1) / -NE117 (TMP117 ±0,1) | ±0,5 / ±0,1 | €30,80–44,95 |
| Mraznička / PT100 | Milesight TS302 + AF10, alebo Dragino LTC2-LB + DR-LT sonda | ±0,4 / ±0,6 | €124,70 / €76,79 |
| Vyššia trieda s výrobným certifikátom | Comet W0911 (Pt1000, ±0,2; backfill ⚠️) | ±0,2 | €204 + sonda |

Zamietnuté: Elsys ELT-2 (buffer len 10 vzoriek), Tektelic TUNDRA (sonda ±2 °C), Decentlab (€772, bez backfillu), MClimate (nezakóduje < 0 °C), staré Dragino LSN50v2 (bez datalogu), DS18B20 sondy pod −10 °C (±2 °C). Milesight časové značky vyžadujú LoRaWAN 1.0.3 a NS s DeviceTimeReq.

**Gatewaye:** Milesight UG63 (mini, vstavaný NS do 20 zariadení, LTE variant) €117,80/€157,80; **RAK7268CV2** (LTE Cat 4, PoE, vstavaný NS, UPS €69) od €187; **Dragino LPS8v2** (vstavaný ChirpStack + Node-RED, bez PoE) €126,90–159; Milesight UG56 €207,50 (UPS kit), UG65 €322,50. MikroTik LtAP LR8 LTE **ukončený**, wAP LR8G bez NS.

**Dvere a napájanie:** Dragino LDS02 (počet otvorení, trvanie, alarm „dlho otvorené") €18,92; Milesight WS523 zásuvka 16 A s **hlásením výpadku** + kWh €63,45; CT svorky Netvox R718N1x €115,90+.

**Alternatívy bez gatewaya:** Efento NB-IoT (40 000 meraní s backfillom, CoAP do „ľubovoľného cloudu", ceny v EUR ⚠️), ELPRO 1TGi €202 + ~€65/rok, Comet W0711 Wi-Fi €285 (len zo siete), Elitech RCW-800W €76, **Elso.sk sada TFA pre lekárne (gateway + 3 senzory + 2 sondy + ISO 17025 certifikáty) €413** = priamy cenový benchmark. NB-IoT/LTE-M pokrytie SR: Orange „viac ako 99 % Slovenska" (€1,23/mesiac), Telekom NB-IoT aj LTE-M po celom Slovensku (od €1,29/mesiac), O2 NB-IoT > 94,6 % populácie.

**Konkurencia hardvérovo:** LogHub = plne proprietárny stack (2,4 GHz rádio → Observer LITE centrála), **prenájom od €20/mesiac** bez DPH na miestnosť, rekalibrácia výmenou zariadenia, 400+ prevádzok. Efento = vlastné BLE a NB-IoT senzory, 5-ročná licencia na senzor. PharmaControl.sk = ISO 17025, 24-mesačná platnosť.

**Kalibrácia (ISO 17025):** SK: ZTS Elektronika SKS (SNAS K-004, aj na mieste), ChiranaLAB, SLM (€36,59 + €6,50/bod), SMÚ (€41/bod), Metershop (**€28 / 3 body**, +€4/bod), ProTechnika (€25 / 3 body 2/6/8 °C); CZ: KALIST AKL (ČIA 2394, do 2030), ČMI (1 960 CZK / 3 body, −40 % od 10 ks), Exatherm (850 CZK / 3 body). Otvorené a dôležité ⚠️: podľa DIS-15 §3.2 a Cometu má certifikát pokrývať snímač **spolu s vyhodnocovacou jednotkou**; žiadne SK/CZ laboratórium verejne neponúka kalibráciu s odčítaním LoRaWAN uplinku — **overiť telefonicky (ZTS Elektronika, ChiranaLAB, KALIST)**; a či ŠÚKL akceptuje český (ČIA) certifikát podľa § 17 ods. 3.

**Odporúčané sady (bez DPH):** lekáreň = UG63-4G + 2× TS301/AF10 + EM300-TH ≈ **€480–530** (+ dvere a zásuvka ≈ €565–615; najlacnejšia variant LPS8v2 + 2× EM320-TH + EM300-TH ≈ €400 s kalibráciou); sklad (10 zón, 2 chladiace boxy, 1 mraznička, 2 gatewaye, dvere, napájanie, 17 kalibrácií) ≈ **€2 150–2 420**.

## 8. Ďalšie overenia vykonané v tejto relácii (mimo agentov)

| Položka | Zdroj | Výsledok |
|---|---|---|
| `gopcua/opcua` | GitHub API | MIT, push 17. 9. 2026, 1 061 ★, aktívny |
| `simonvetter/modbus` | GitHub API | MIT, push 28. 7. 2026, klient aj server, TCP + RTU |
| mochi-mqtt zdieľané subscriptions, WS, TLS, persistence | README | áno (MQTT v5 shared subscriptions dokumentované) |
| ChirpStack požiadavky | chirpstack.io | MQTT v5 so shared subscriptions, PostgreSQL ≥ 13 alebo SQLite, Redis ≥ 6.2; inštalácia Debian/Ubuntu/Docker, Windows nespomenuté |
| zákon 157/2018 § 17 (celý) | zakonypreludi.sk | overené doslovne (viď kap. 7) |

## 9. Čo z toho plynie pre návrh (súhrn)

1. Vlastné jadro v **Go 1.27 + PostgreSQL 18 + Lit 3**, jeden binár pre Windows aj Linux, mochi-mqtt a Typst vnútri, ChirpStack voliteľne na Linuxe. (R1, R4)
2. Audit trail a merania append-only v PostgreSQL s hash-reťazcom a podpísanými kontrolnými bodmi; žiadna ďalšia DB. (P-11, P-19)
3. LoRaWAN cez gateway s NS ako default; senzory len s overeným S&F (Milesight, Dragino); dekodéry v jadre. (R3, 9.2)
4. Register kalibrácií = priamo požiadavka § 17 zákona 157/2018 písm. d)–e); certifikáty len z SMÚ / akreditovaných laboratórií; otázka „snímač + jednotka" sa musí vyriešiť s laboratóriom skôr, než sa vyberie finálny model senzora. (P-06)
5. Cenový strop lekárne je jasný: Elso €413 jednorazovo, LogHub €20–50/mesiac; náš kit ≈ €480–530 + softvér musí byť ospravedlnený reportom, registrom a on-prem dátami, nie „povinnosťou".
