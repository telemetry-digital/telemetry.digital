# Model hrozieb ctrl32 telemetry — v0.1

**Dátum:** 23. 9. 2026 · **Stav:** návrh; aktualizuje sa pri každej zmene povrchu (ADR-0007) · **Metóda:** STRIDE nad vstupnými bodmi, opatrenia mapované na URS

## 1. Rozsah a predpoklady

- V rozsahu: serverová aplikácia (Go), PostgreSQL, web UI, REST API, MQTT broker v procese, HTTP ingest, konektory (LoRaWAN, OPC UA, Modbus, MQTT most, webhooky), FOTA, konzola, prehliadač správ, zálohy, inštalátor a aktualizácie, klientske knižnice.
- Mimo rozsahu (zodpovednosť nasadzovateľa, ADR-0003): bezpečnosť OS a siete hostiteľa, fyzická bezpečnosť, patchovanie OS/PostgreSQL, správa účtov mimo systému, bezpečnosť cudzích zariadení (LoRaWAN senzory, PLC), kalibrácie.
- Predpoklady: nasadzovateľ dodrží sprievodcu hardeningom; certifikáty a kľúče spravuje nasadzovateľ; správca DB je dôveryhodný, ale **jeho zásah musí byť detekovateľný** (hash-reťazec).

## 2. Aktíva

| Aktívum | Dôvernosť | Integrita | Dostupnosť |
|---|---|---|---|
| Merania a odvodené dáta | stredná (obchodné/procesné dáta) | **kritická** (GxP, rozhodnutia) | vysoká |
| Audit trail, alarmový log, prístupový log, podpisy | stredná | **kritická** (dôkazný materiál) | vysoká |
| Účty, heslá, TOTP seedy, API tokeny, tokeny zariadení | **kritická** | kritická | stredná |
| Firmvér a podpisové kľúče (FOTA) | nízka / kľúče kritická | **kritická** (ovládnutie flotily) | stredná |
| Konfigurácia (limity, priradenia, konektory s credentials) | vysoká (credentials) | kritická | vysoká |
| Zálohy | vysoká | vysoká | vysoká |
| Dostupnosť alarmovania | — | — | **kritická** (nedoručený alarm = incident) |

## 3. Aktéri

Anonymný útočník z internetu · kompromitované alebo falošné zariadenie · legitímny používateľ prekračujúci oprávnenia · správca organizácie · implementátor s prístupom ku konzole a FOTA · správca DB/OS hostiteľa · vývojár (my; dodávateľský reťazec) · útočník v sieti zákazníka (MITM) · škodlivý webhook/integrácia tretej strany.

## 4. Vstupné body a hrozby (STRIDE)

| Vstupný bod | Hrozba | Opatrenie | URS |
|---|---|---|---|
| **Web UI / REST API (HTTPS)** | S: krádež relácie/tokenu; T: CSRF, manipulácia požiadavky; R: popretie akcie; I: únik dát cez chybové hlášky/IDOR; D: brute force, zahltenie; E: eskalácia rolí | TLS povinné (profil internet), cookies HttpOnly/Secure/SameSite, CSRF token, rotácia relácie, MFA, RBAC s testami per endpoint (IDOR), audit trail všetkých mutácií s identitou, rate limiting a zamknutie, generické chybové odpovede, CSP/HSTS, validácia schémou, limity veľkosti | H-01..H-14, I-01..I-06, G-05 |
| **MQTT broker (zariadenia, ChirpStack, prehliadač)** | S: falošné zariadenie s ukradnutým tokenom; T: podvrhnuté merania; I: odber cudzích tém; D: CONNECT flood, obrovské payloady; E: publikovanie do `cmd` cudzieho zariadenia | token/mTLS per zariadenie, ACL na vlastný prefix, zákaz wildcard subscribe pre zariadenia, limity veľkosti a rýchlosti, rotácia tokenov, revokácia zariadenia (okamžité odpojenie), dedupe `seq`, plauzibilita hodnôt, surové správy uchované (forenzika) | A-02..A-06, H-15, F-01 |
| **HTTP ingest** | ako MQTT + replay | Bearer token, idempotencia `(device, seq)`, limity | A-04, G-04 |
| **LoRaWAN feed (gateway NS / ChirpStack / TTS webhook)** | S: falošný gateway/webhook; T: zmenené uplinky; R: neexistujúca stopa | autentifikovaný konektor (token/HMAC/mTLS), surové uplinky uchované, dekodéry verziované, `fcnt` dedupe; bezpečnosť samotnej LoRaWAN siete (AppKey) je vec nasadzovateľa | A-07..A-09 |
| **OPC UA / Modbus (odchádzajúce spojenia)** | S: falošný server (MITM); I: únik credentials; T: zápis do PLC neoprávnenou osobou | OPC UA s bezpečnostnou politikou a overením certifikátu servera; credentials šifrované; **zápis (riadenie) len cez príkaz s oprávnením, dôvodom a auditom; platforma nie je v bezpečnostnej slučke** (ADR-0008) | A-10..A-13, C-08 |
| **FOTA** | T: podstrčený firmvér; S: falošný server pri sťahovaní; D: zahltenie linky; R: kto spustil kampaň | podpis autora mimo servera + overenie podpisu a hashu na zariadení, TLS + token pri sťahovaní, min_ver/hw kontrola, postupné nasadenie s auto-stopom, rollback, samostatné oprávnenie, audit | D-01..D-09 |
| **Vzdialená konzola** | E: shell-like prístup k zariadeniu; R: neauditované zmeny; I: únik dát cez logy zariadenia; D: zahltenie | predvolene vypnutá, zapnutie per zariadenie, MFA pri otvorení, plný append-only log vstup/výstup, rate limit, dĺžka riadku, zoznam povolených príkazov, globálny zákaz v GxP profile; **žiadny shell na server** | E-01..E-08 |
| **Prehliadač správ** | I: citlivé payloady; T: „simulovaná" správa v produkčných dátach | samostatné oprávnenie, logovaný prístup, simulácia označená a v GxP profile zakázateľná, opätovné dekódovanie = nová verzia série | F-01..F-06 |
| **Webhooky von / MQTT von** | I: únik dát na cudzí endpoint; S: podvrh príjemcu | HMAC podpis, TLS, allowlist cieľov, žiadne credentials v URL, log doručení | A-14, A-15 |
| **Webhooky dnu (cloudy tretích strán)** | S: falošná správa; D: flood | HMAC/token per konektor, limity, surové správy uchované | A-16 |
| **Zálohy** | I: únik celej DB; T: obnova zmanipulovanej zálohy | šifrované zálohy, kontrola integrity (checksum), test obnovy, cieľ mimo servera s oddelenými credentials; hash-reťazec odhalí manipuláciu po obnove | K-01..K-06 |
| **Inštalátor a aktualizácie** | T: podvrhnutý balík; E: služba s vysokými právami | podpísané artefakty (Windows certifikát ctrl32, cosign), overenie podpisu pred aktualizáciou, služba pod vlastným účtom, DB rola bez DDL, žiadna auto-aktualizácia | K-07..K-11 |
| **Dodávateľský reťazec (naše závislosti)** | T: škodlivá závislosť; I: CVE | minimálne pinované závislosti, `govulncheck`, SBOM, reprodukovateľný build, review zmien `go.sum`, frontend build vendorovaný | H-16..H-18 |
| **Databáza (správca DB/OS)** | T: tichá zmena záznamov; R: výmaz audit trailu | DB rola aplikácie bez UPDATE/DELETE/TRUNCATE + triggery; hash-reťazec s podpísanými kontrolnými bodmi ukotvenými mimo DB (report, e-mail, TSA); `audit verify`; alarm pri porušení reťazca | I-02..I-05 |
| **Notifikačné kanály** | D: nedoručený alarm (výpadok SMTP/SMS); I: obsah alarmu | eskalácia cez nezávislé kanály, log doručení, „alarm nedoručený" = incident s vlastným alarmom, minimálny obsah v SMS | C-05..C-07 |
| **Čas (NTP)** | T: posun času → nesprávne časy záznamov | `received_at` z DB, NTP kontrola pri štarte a periodicky, drift zariadení sledovaný, UTC v DB | B-06..B-08 |
| **Knižnice zariadení** | I: token v plaintexte vo flash; T: falošný server (MITM) | overenie CA/odtlačku servera, token v NVS s možnosťou flash encryption (ESP32) — odporúčanie v dokumentácii; rotácia tokenov; revokácia | L-04..L-07 |

## 5. Zvyškové riziká (prijaté, zdokumentované)

- Kompromitované zariadenie s platným tokenom môže posielať falošné merania — obmedzené plauzibilitou, revokáciou a forenzikou zo surových správ; nedá sa úplne vylúčiť (rovnako u každej platformy).
- Nasadzovateľ, ktorý vypne MFA v profile `intranet` alebo vystaví HTTP bez TLS, nesie riziko sám — systém to loguje a zobrazuje varovanie.
- Správca s prístupom k DB a k podpisovému kľúču kontrolných bodov môže sfalšovať reťazec — preto sa hash kontrolného bodu ukotvuje mimo DB (report/e-mail/TSA) a kľúč sa odporúča držať mimo DB servera.
- Jednoosobový vývojár: kompenzácia automatizovanými bránami, externou revíziou kódu a penetračným testom (draft Annex 11 2.3).

## 6. Overenie

Každé opatrenie má test v regresnej sade (tag `URS-xxx`); penetračný test pred 1.0 a 2.0; model hrozieb sa reviduje pri každom novom konektore, transporte alebo oprávnení.
