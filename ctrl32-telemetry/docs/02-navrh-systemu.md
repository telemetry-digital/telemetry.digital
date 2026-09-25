# ctrl32 telemetry — návrh univerzálnej platformy pre zber údajov

**Verzia:** 0.6 (pracovný návrh na diskusiu) · **Dátum:** 23. 9. 2026 · **Stav (25. 9. 2026):** východiskový návrh; realizovaný stav je v `JOURNAL.md` (0.33.1 v produkcii na telemetry.digital) a v stĺpci Zdroj URS; rozhodnutia v `adr/` 0001–0014 prijaté, otvorené body z kap. 15 vyriešené (pozri tam); požiadavky v `urs/URS-v0.1.md`; protokol zariadení v `protocol/`; model hrozieb v `threat-model.md`

**Čo to je.** ctrl32 telemetry je univerzálna platforma na zber, uchovávanie, vyhodnocovanie a reportovanie údajov zo zariadení — systém „podobný ThingsBoard" pre Linux a Windows servery, **bezplatný a navždy bezplatný**. My sme **len vývojár systému**: dodávame serverovú aplikáciu, klientske knižnice pre zariadenia (Arduino, ESP-IDF, ctrl32, STM32) a dokumentáciu. Kto si systém kde nasadí, je jeho vec a **preberá za nasadenie zodpovednosť**; hardvér je ľubovoľný. Príjem vzniká z implementácie, nie zo systému.

**Prečo regulačný research.** Aby bol systém pripravený na najprísnejšie požiadavky (farmácia: Annex 11, EU GDP, PIC/S PI 041-1, vyhláška 129/2012) a spĺňal ich **ako vlastnosť jadra** — integrita údajov, audit trail, bezpečnosť, logy. Lekárne a sklady nie sú cieľový zákazník; sú testovací štandard. Čo obstojí pred ŠÚKL-om, obstojí pred energetikom aj priemyslom.

Technický research (platformy, licencie, stack, hardvér) je v `01-technicky-research.md`; regulačný v `00-regulacny-research/`.

---

## 0. Zhrnutie pre rozhodnutie

**Čo staviame.** Jeden proces (`ctrl32-telemetry`, jediný binár pre Windows aj Linux) + PostgreSQL. V ňom: **konektory a integrácie** (MQTT broker v procese, HTTP, LoRaWAN, OPC UA klient, Modbus klient, MQTT most na cudzí broker, webhooky dnu aj von), profily zariadení a verziované dekodéry, **prehliadač správ** (živý pohľad na surové a dekódované správy každého zariadenia a konektora), append-only časové rady s detekciou medzier, alarmy s eskaláciou, pravidlá a príkazy do zariadení, **FOTA** (podpísaný firmvér, kampane, rollback), **vzdialená konzola zariadenia**, používatelia s rolami a MFA, nemeniteľný audit trail, prístupový a systémový log, dashboardy, reporty a exporty, zálohy s testom obnovy, webové UI a **REST API** (UI je jeho klient). K tomu **klientske knižnice** (spoločné C jadro + porty Arduino, ESP-IDF, STM32 a integrácia do ctrl32) a **inštalátory** (Windows podpísaný certifikátom ctrl32, Linux balíky).

**Čo je mimo systému.** Hardvér (ľubovoľný), prevádzka a hosting (nasadzovateľ), validácia u zákazníka (implementácia), kalibrácie a certifikáty (implementácia/laboratóriá). Systém dodáva technickú dokumentáciu, ktorú implementátor použije.

**Kľúčové rozhodnutia (odporúčanie tučne, detail v kap. 15):**

| # | Rozhodnutie | Odporúčanie |
|---|---|---|
| R1 | Vlastné jadro vs. ThingsBoard CE | **Vlastné jadro.** ThingsBoard prepisuje a maže telemetriu, audit log nepokrýva merania, kľúčové funkcie sú v platenej PE s EULA proti ďalšiemu šíreniu. Preberáme koncepty, nie kód. |
| R2 | Licencia a zdrojový kód | **Apache-2.0 pre server aj knižnice, verejný repozitár**, ochranná známka „ctrl32" mimo licencie. Apache dáva bezplatnosť navždy, patentovú ochranu, vylúčenie záruky (nasadzovateľ nesie zodpovednosť) a bezproblémové vloženie knižníc do cudzieho firmvéru. Alternatíva AGPL-3.0 pre server, ak chceš brániť uzavretým SaaS forkom — za cenu odporu právnikov vo veľkých firmách. |
| R3 | Zariadenia | **Hardvér ľubovoľný; my dodávame protokol + knižnice pre Arduino, ESP-IDF, ctrl32, STM32.** LoRaWAN, OPC UA, Modbus, HTTP ako zberné adaptéry v jadre. Nič nepredávame, nič nekalibrujeme. |
| R4 | Stack | **Platforma v Go 1.27 + PostgreSQL 18 + TypeScript/Lit 3; real-time runtime `ctrl32-runtime` v C++20 ako samostatný voliteľný proces od fázy 3** (OPC UA server/klient cez open62541, S7, ms cykly, redundancia, edge). Dôvody a spúšťače v ADR-0001. Potvrdiť PoC vo fáze 0. |
| R8 | SCADA | **Postupne v piatich stupňoch** (ADR-0008): historian + alarmy + príkazy (1) → HMI viewer s JSON formátom obrazoviek (2) → HMI editor + riadiaci zápis so štyrmi očami (3) → C++ runtime s OPC UA serverom a redundanciou (3+) → rozšírenia. Platforma nikdy nie je v bezpečnostnej slučke. |
| R5 | Nasadenie | **Vždy u toho, kto nasadzuje** — intranet firmy alebo verejne na webe. Jeden inštalátor, profily „intranet" a „internet" (druhý vynucuje tvrdšie predvoľby). Žiadne SaaS prevádzkované nami; voliteľne verejná demo inštancia. |
| R6 | Rozsah a dimenzovanie | **Univerzálny v celom rozsahu ako cieľ, stavaný po vrstvách** (kap. 13); dimenzovaný na organizáciu: jeden server, do ~10 000 datastreamov, bez clustra. Vyššie objemy sú starosť nasadzovateľa (silnejší server, PostgreSQL replika), nie architektúry. |
| R7 | Bezpečnosť, audit, logy | **Od prvého commitu, ako nefunkčné požiadavky s testami**, nie ako modul na neskôr (kap. 7 a 7a). |

---

## 1. Z regulačného researchu → požiadavky jadra

Destilát toho, čo z regulácie priamo vyplýva pre jadro. Platí univerzálne — energetik nepotrebuje e-podpis, ale nemeniteľné merania a audit trail mu neublížia. Každý riadok bude položka URS s ID a testom (kap. 12).

| ID | Požiadavka | Zdroj | Dôsledok v návrhu |
|---|---|---|---|
| P-01 | Automatická evidencia v definovanom intervale, záznamy uchované po definovanú lehotu (lekáreň: ≤ 2 h, 12 mesiacov) | vyhl. 129/2012 § 30 ods. 8 (overené doslovne 23. 9. 2026) | Očakávaný interval je vlastnosť datastreamu; medzery sa hlásia; retenčná politika per asset s právnym základom. |
| P-02 | Jedno meracie miesto na každú miestnosť/priestor | vyhl. 419/2025, príloha 1 | Asset je základná jednotka. |
| P-03 | Záznam s menom osoby a nápravným opatrením | usmernenie MZ SR (sekundárne) | Reporty so sign-off poľami; anotácie s identitou. |
| P-04 | „Pravidelne" zaznamenávať; zariadenia indikujúce a signalizujúce nedodržanie; dokumentácia 5 rokov | vyhl. 128/2012 § 15, § 18 | Alarmy sú jadro, nie doplnok; retencia 5 rokov ako predvoľba pre regulované assety. |
| P-05 | Mapovanie pred použitím, monitory na extrémoch | EU GDP 3.2.1 | Pozícia snímača nesie odkaz na dokument; modul mapovania neskôr. |
| P-06 | Kalibrácia v intervaloch, nadväznosť, **evidencia každého povinne kalibrovaného meradla**, kalibrácia len SMÚ/akreditované laboratórium | EU GDP 3.3; 129/2012 pozn. 7a → § 17 zákona 157/2018 (overené) | Register kalibračných certifikátov per snímač (laboratórium, akreditácia, body, neistota, platnosť, miesto) + upozornenia + report. Kto kalibruje, je vec implementácie. |
| P-07 | Zabudované kontroly pri výmene dát | Annex 11 cl. 5 | TLS + autentifikovaný ingest; dedupe; plauzibilita; viditeľné medzery. |
| P-08 | Kontrola kriticky zadávaných dát | Annex 11 cl. 6; draft 10.1 | Rozsahová validácia + voliteľné štyri oči; každá zmena s dôvodom. |
| P-09 | Zálohy pravidelné, obnova testovaná, zálohy oddelené | Annex 11 cl. 7.2; draft 16 | Zálohovací modul s plánom, kontrolou integrity a automatizovaným testom obnovy; cieľ mimo servera. |
| P-10 | Výtlačky jasné; indikácia zmeny | Annex 11 cl. 8 | PDF s hlavičkou, hashom, „koniec reportu". |
| P-11 | Audit trail zmien a výmazov s dôvodom; zapnutý a zamknutý vždy; export nie „flat and locked" | Annex 11 cl. 9; MHRA 6.13; draft 12 | Append-only, hash-reťazec, podpísané kontrolné body, bez prepínača, `reason NOT NULL`, CSV/JSON export, workflow prehľadu. |
| P-12 | Záznam prístupov; identita pri každej zmene | Annex 11 cl. 12; draft 11 | Osobné účty, história rolí, prístupový log, MFA, zamknutie, timeout. |
| P-13 | Incidenty hlásené a posudzované | Annex 11 cl. 13 | Incidentný log v systéme; „alarm nedoručený" je incident. |
| P-14 | E-podpis zviazaný so záznamom; re-autentifikácia, význam, pásmo | Annex 11 cl. 14; draft 13 | Podpis = hash verzie záznamu + používateľ + význam + čas; zmena podpis zneplatní. |
| P-15 | Kontinuita pri výpadku; bufferované dáta s pôvodným časom | Annex 11 cl. 16 | Knižnice bufferujú a dopĺňajú; dva časy na meraní; retrospektívna odchýlka ako samostatná udalosť. |
| P-16 | Archív prístupný a vyhľadávateľný; vyvolateľnosť testovaná | Annex 11 cl. 17; draft 17 | Žiadny automatický výmaz; test vyvolania v release procese. |
| P-17 | Alarmový log oddelený od audit trailu, osobné potvrdenie, logovaná deaktivácia | draft Annex 11 kap. 8 | Alarmový log append-only; potlačenie časovo ohraničené a logované. |
| P-18 | Kontrolované hodiny, časové pásmo | MHRA §5; WHO TRS 996 | `received_at` píše DB; UTC interne; drift zariadení sledovaný. |
| P-19 | Raw data definované; parametre spracovania sledovateľné | GMP Ch. 4; MHRA §6.9 | Surový payload nemeniteľný; verzia dekodéra/kalibrácie/odvodenia na každom zázname. |
| P-20 | Neistota vs. tolerančné pásmo | ISPE (prax) | Datastream nesie požadovanú presnosť; systém varuje pri nesúlade. |
| P-21 | Dodávateľ: zmluva, QMS, audity, dokumentácia, exit strategy, release proces | Annex 11 cl. 3; draft kap. 7 | Vývojár dodáva: verejný QMS-light (SDLC, release, incident), release pack, plný export, verejný zdrojový kód = najsilnejšia exit strategy. Zmluvy sú vec implementácie. |
| P-22 | Žiadny kód špecifický pre zákazníka | GAMP 5 | Všetko zákaznícke je konfigurácia; jeden podpísaný artefakt. |

---

## 2. Kto ho nasadzuje a ako

| | **Intranet firmy** | **Verejne na webe** |
|---|---|---|
| Kto nasadzuje | IT firmy alebo implementátor | implementátor pre viac klientov, servisná firma, ktokoľvek |
| Kde | Windows Server / Linux, VM, často bez internetu | VPS alebo on-prem za reverzným proxy |
| Profil inštalátora | `intranet`: TLS s vlastným certifikátom, MFA pre správcov, LAN/VPN | `internet`: ACME/Let's Encrypt alebo certifikát, **MFA pre všetkých**, limity pokusov, prísne hlavičky, MQTT len TLS + autentifikácia, voliteľný IP allowlist |
| Viac klientov na jednej inštancii | nie (jedna organizácia) | áno: **Organizácie (tenanti; systémový administrátor ich vytvára a zakazuje, 0.9.0) → Zákazníci** (podorganizácie so scopovanými rolami, ako ThingsBoard tenant → customer), bez samoobslužnej registrácie |
| Aktualizácie | manuálne, release pack | manuálne alebo v UI po stiahnutí podpísaného balíka |
| Zodpovednosť | nasadzovateľ (licencia bez záruky + dokument „Zodpovednosť za nasadenie") | nasadzovateľ |

**Model organizácie.** Jedna inštalácia = jedna *organizácia* (ten, kto nasadil). Organizácia môže mať *zákazníkov* (podorganizácie): implementátor tak spravuje viac firiem na jednej inštancii, každý zákazník vidí len svoje prevádzky. Nie je to SaaS s registráciou; je to hierarchia oprávnení.

---

## 3. Architektúra

```
 zariadenia s knižnicou              LoRaWAN senzory          PLC, BMS, SCADA         merače, meniče        čokoľvek
 (Arduino, ESP-IDF, ctrl32, STM32)       │ EU868                  │ OPC UA                 │ Modbus TCP/RTU       │ HTTP/CSV
      │ MQTT/TLS (JSON alebo CBOR)         ▼                       │                        │                      │
      │                          gateway s NS ──MQTT/HTTP──┐        │                        │                      │
      │                          alebo ChirpStack ─────────┤        │                        │                      │
      ▼                                                    ▼        ▼                        ▼                      ▼
 ┌──────────────────────── ctrl32-telemetry (jeden proces, jeden binár) ───────────────────────────────────────────┐
 │ ZBERNÁ VRSTVA: push adaptéry (MQTT broker v procese, HTTP, LoRaWAN)  ·  poll drivery (OPC UA, Modbus)           │
 │      │ surové správy ─► profily/dekodéry (verziované) ─► observations (append-only) ─► medzery, backfill, drift │
 │ alarm engine ─► eskalácia ─► notifikátor (e-mail, SMS, push, hlas, webhook) ─► alarmový log                    │
 │ pravidlá (podmienka → akcia) · príkazy do zariadení (RPC) · atribúty (client/server/shared)                      │
 │ IAM (organizácia, zákazníci, roly, MFA, relácie) ─► prístupový log · audit trail (hash chain + kontrolné body) │
 │ dashboardy · reporty (Typst → PDF/A) · exporty · e-podpisy · záloha + test obnovy · plánovač · web UI · API      │
 │ systémový log (štruktúrovaný, syslog/OTLP export) · health · metriky                                              │
 └──────────────────────────────────────────┬──────────────────────────────────────────────────────────────────────┘
                                            ▼
                               PostgreSQL 18 (jediná externá závislosť; pribalená v inštalátore)
```

**Zásady pre každý modul:**

1. **Append-only pre všetko, čo je záznam** (merania, surové správy, audit trail, alarmový log, prístupový log, podpisy, reporty). DB rola aplikácie bez `UPDATE`/`DELETE`/`TRUNCATE`; triggery blokujú aj vlastníka; výmaz len ako autorizovaná, logovaná operácia po retencii.
2. **Dva časy na každom meraní:** `measured_at` (zdroj) a `received_at` (databáza).
3. **Verzia všetkého, čo mení význam dát:** dekodér, kalibrácia, register map, limity, retencia, odvodenia.
4. **Žiadna neviditeľná medzera.**
5. **Server je zdroj času;** NTP kontrola.
6. **Konfigurácia, nie kód;** konfiguračné zmeny cez audit trail s dôvodom.
7. **Audit trail a bezpečnostné kontroly sa nedajú vypnúť konfiguráciou.**
8. **Release, nie deploy:** podpísané, číslované verzie s release packom; žiadna automatická aktualizácia.
9. **Malé a jednoduché:** jeden proces, jedna DB, jeden log, jeden inštalátor.
10. **Bezpečné predvoľby:** systém po inštalácii je bezpečný bez ďalšej práce; uvoľňovanie (nie sprísňovanie) vyžaduje vedomé rozhodnutie s dôvodom.

**Moduly (Go balíky):** `connector` (rámec + `mqtt`, `http`, `lorawan`, `opcua`, `modbus`, `mqttbridge`, `webhook`), `decode`, `msgview`, `store`, `rule`, `alarm`, `notify`, `rpc`, `fota`, `console`, `iam`, `audit`, `log`, `dash`, `report`, `export`, `backup`, `sched`, `web`, `api`, `admin`, `installer`.

---

## 4. Doménový model

Univerzálny, názvoslovím podľa OGC SensorThings (Thing / Datastream / Observation / Sensor); ThingsBoard má to isté v inom názvosloví.

```
Organization (ten, kto nasadil)
 └─ Customer (voliteľná podorganizácia so scopovanými rolami)
     └─ Site (prevádzka: adresa, časové pásmo)
         └─ Asset (čokoľvek monitorované: miestnosť, chladnička, stroj, rozvádzač, vozidlo, budova; typ + atribúty)
             └─ Datastream (jedna veličina: kind = gauge | counter | state | event; jednotka; očakávaný interval;
                  │           presnosť; limity; retencia)
                  ▲ priradenie (od–do, auditované)
                  │
             Sensor (fyzický snímač: sériové číslo, typ, kalibrácie s verziou)
                  ▲
             Device (to, čo komunikuje: jednotka s knižnicou, LoRaWAN uzol, OPC UA server, Modbus meter/PLC;
                     transport, adresa, profil, dekodér/register map, firmvér, batéria, stav spojenia,
                     atribúty client/server/shared, fronta príkazov)
```

- `kind` rozhoduje o spracovaní: `gauge` ako je; `counter` monotónne počítadlo → odvodená spotreba za interval (pretočenie, výmena meradla) do `derived_observation` s verziou postupu; `state`/`event` majú `value_json`.
- Atribúty ako v ThingsBoarde: *client* (zariadenie hlási: firmvér, model), *server* (my o zariadení: umiestnenie), *shared* (konfigurácia pre zariadenie: interval, limity — doručuje sa cez MQTT).
- **Aplikačný balík = hodnoty, nie tabuľky:** typy assetov, atribúty, retenčné politiky, šablóny alarmov, šablóny reportov, matice zhody. Balík *GxP teploty* je prvý, lebo je najprísnejší; balík *Energie* druhý.

**Tabuľky jadra (výber; `A` = append-only):** `organization`, `customer`, `site`, `asset`, `datastream`, `device`, `sensor`, `device_profile`, `decoder`, `assignment`; `connector` (typ, konfigurácia s verziou, stav), `connector_log` A; `raw_message` A (vstup aj výstup, s výsledkom dekódovania), `observation` A (partície po mesiacoch), `derived_observation` A, `gap`; `attribute` (client/server/shared s históriou), `command` (RPC fronta, stav, odpoveď) A; `firmware` A (binár, verzia, hardvérové ID, hash, podpis, kľúč), `fota_campaign`, `fota_device_status` A; `console_session`, `console_log` A; `alarm_rule`, `alarm`, `alarm_event` A, `escalation_policy`, `notification`; `rule` (podmienka → akcia, verziované); `dashboard`, `widget`; `user`, `role`, `user_role_history` A, `session`, `access_log` A, `api_token`, `webhook_delivery` A; `audit_trail` A, `audit_checkpoint` A, `audit_review` A, `esignature` A; `calibration_certificate`, `report` A, `report_template`, `retention_policy`, `deletion` A, `config_change` A, `backup_run` A, `restore_test` A, `incident`, `system_log` (rotované, mimo GxP záznamov).

---

## 5. Dátová cesta a integrita

1. **Príjem:** adaptér/driver uloží `raw_message` s `received_at = now()` z DB; duplicity s príznakom.
2. **Dekódovanie:** profil (verzia + hash) → observácie; čas zdroja → `measured_at`, `time_source = device`; inak server; rozdiel nad toleranciou → `clock_suspect`.
3. **Kalibrácia a odvodenie:** kalibrovaná hodnota podľa platnej verzie; odvodenia s verziou postupu; rekalibrácia = nová verzia, spätný prepočet len ako nová `series_version`.
4. **Medzery:** plánovač vs. očakávaný interval → `gap` + alarm; backfill medzeru uzavrie; retrospektívne prekročenie limitu = alarm `retrospective`.
5. **Audit trail:** hash-reťazec (SHA-256, gapless `seq`), denný **kontrolný bod** podpísaný Ed25519, hash v reportoch a voliteľne e-mailom; `ctrl32-telemetry audit verify`.
6. **Čas:** NTP, `timestamptz`, rendering s explicitným pásmom.

---

## 6. Alarmy, pravidlá, príkazy

- **Alarmy:** vysoká/nízka hodnota (výstraha + akcia), rýchlosť zmeny, porucha snímača, strata komunikácie (datastream/zariadenie/gateway/server), slabá batéria, výpadok napájania, stav, retrospektívna odchýlka, exspirujúca kalibrácia; oneskorenie, hysteréza; potvrdenie po prihlásení s dôvodom; potlačenie časovo ohraničené; eskalačný rebríček; alarmový log ako samostatná obrazovka a report.
- **Pravidlá (fáza 2):** deklaratívne „podmienka → akcia" nad datastreamami (prah, kombinácia, časové okno) s akciami alarm/notifikácia/webhook/príkaz zariadeniu/odvodený datastream; verziované a auditované; plnohodnotný grafický rule-chain až vo fáze 3, ak vôbec.
- **Príkazy (RPC, fáza 2):** server → zariadenie (nastav interval, prepni výstup), fronta s potvrdením a timeoutom, každý príkaz v audit traile s identitou.
- **Kanály:** SMTP (primárny), SMS brána cez HTTP (konfigurovateľný poskytovateľ), Web Push, hlas cez HTTP poskytovateľa, webhook. Poskytovateľov nevyberáme za nasadzovateľa; dodávame adaptéry.

---

## 7. Identita a prístup

- **Roly:** Čitateľ · Operátor · QA recenzent · Správca prevádzky · Správca zákazníka · Správca organizácie · voliteľné vlastné roly zložené z oprávnení. Oddelenie povinností: administratívna rola nemá QA práva v tej istej prevádzke (predvolené).
- **Účty:** osobné; Argon2id (m = 19 MiB, t = 2, p = 1, pepper mimo DB); zmena hesla pri prvom prihlásení; zložitosť vynútená; **TOTP** povinné pre správcov a QA, v profile `internet` pre všetkých; WebAuthn/passkey ako druhý faktor; zamknutie po N pokusoch; timeout nečinnosti len správcom; rotácia ID relácie; história rolí; prístupový log; API tokeny s rozsahom a expiráciou; neskôr OIDC/LDAP.
- **Štyri oči** pre limity, koeficienty, priradenia — voliteľné per organizácia.
- **E-podpis (fáza 2):** re-autentifikácia pri podpise; význam; manifestácia; hash verzie záznamu.

## 7a. Bezpečnostná a logovacia základňa (od prvého commitu)

Toto je odpoveď na „na bezpečnosť, audit, logy treba myslieť od začiatku". Každý bod je nefunkčná požiadavka s testom v regresnej sade.

**Bezpečné predvoľby a povrch:**
- Žiadne predvolené heslá; prvý správca sa vytvára v sprievodcovi; TLS povinné (profil `internet` bez TLS nespustí HTTP ani MQTT); HTTP → HTTPS presmerovanie; HSTS, CSP, X-Frame-Options, Referrer-Policy; cookies `HttpOnly; Secure; SameSite=Strict`; CSRF ochrana; limity veľkosti požiadaviek a payloadov; rate limiting na prihlásenie, API a MQTT CONNECT; dočasné blokovanie IP po sérii zlyhaní.
- MQTT: každé zariadenie vlastný token alebo certifikát, ACL na vlastné témy, žiadny anonymný prístup, žiadny wildcard subscribe pre zariadenia.
- Tajomstvá (SMTP/SMS kľúče, TOTP seedy, tokeny zariadení) šifrované v DB (AES-GCM, kľúč mimo DB); heslá Argon2id; tokeny hashované.
- Minimálne oprávnenia OS: služba beží pod vlastným účtom, DB rola aplikácie bez DDL a bez UPDATE/DELETE na záznamoch; nič nepotrebuje root/Administrator po inštalácii.
- Vstupy: validácia schémou na API aj ingeste; dekodéry JS (ak sú) bežia v sandboxe s limitmi CPU/pamäte/času a bez I/O.
- Závislosti: minimálne, pinované; `govulncheck` a SBOM (CycloneDX) v každom release; podpísané artefakty (cosign alebo podpis certifikátom ctrl32 pre Windows); reprodukovateľný build.
- Zverejnená **bezpečnostná politika** (`SECURITY.md`, `security.txt`): ako hlásiť zraniteľnosť, lehoty opráv, bezpečnostné bulletiny; **model hrozieb** ako dokument v repe; pred v1.0 externý penetračný test a jeho súhrn zverejnený.
- **Zodpovednosť za nasadenie:** licencia bez záruky + dokument „Zodpovednosť nasadzovateľa" (patchovanie OS/DB, zálohy, sieť, prístup z internetu, kalibrácie) + **sprievodca hardeningom** (reverzné proxy, firewall, VPN, BitLocker/LUKS, oddelené zálohy). Systém sám periodicky upozorní v UI na zastaranú verziu a na nedodržané odporúčania (bez odosielania dát von, len lokálna kontrola voči podpísanému zoznamu verzií, ktorý si správca stiahne).

**Logy — tri oddelené vrstvy:**
1. **GxP záznamy** (append-only tabuľky: audit trail, alarmový log, prístupový log, podpisy) — nikdy sa nerotujú, retencia podľa politiky, hash-reťazec, export CSV/JSON.
2. **Systémový log** (aplikácia, adaptéry, drivery, plánovač): štruktúrovaný JSON, úrovne, korelačné ID požiadavky, **bez hesiel/tokenov/osobných údajov**, rotácia a retencia nastaviteľné, export do syslog/OTLP/súboru pre SIEM nasadzovateľa; zapisuje sa aj do DB tabuľky `system_log` s krátkou retenciou pre UI.
3. **Bezpečnostné udalosti** (zlyhané prihlásenia, blokovania, zmeny oprávnení, zmeny konfigurácie, spustenie/zastavenie služby, zlyhania záloh, porušenie hash-reťazca) — v prístupovom logu/audit traile **a** v systémovom logu s príznakom `security=true`, s voliteľnou notifikáciou správcovi.

**Prevádzková pozorovateľnosť:** health endpoint, metriky (úplnosť dát v %, latencia alarmov, fronty notifikácií, stav záloh, drift hodín), stavová stránka správcu.

---

## 8. Dashboardy, reporty, exporty

- **Dashboardy:** fáza 1 pevné (prehľad prevádzky, trend datastreamu, alarmy, stav zariadení, mapa medzier); fáza 2 jednoduchý builder (mriežka widgetov: trend, hodnota, gauge, tabuľka, stav, mapa) nad datastreamami; živé hodnoty cez MQTT-over-WebSocket z brokera v procese.
- **Reporty (Typst → PDF/A):** periodický report assetu (min/max/priemer, medzery, alarmy, sign-off polia, kotva audit trailu, hash, „koniec reportu"), report odchýlky s MKT, alarmový log, audit trail, stav kalibrácií, dátový balík pre periodický prehľad, mesačná spotreba (energie). Šablóny verziované; balík GxP prináša slovenskú lekárenskú šablónu ako referenčnú.
- **Exporty:** CSV a JSON pre všetko; plný export organizácie jedným kliknutím; import CSV s kontrolou integrity (Annex 11 cl. 4.8). **Každý export aj ako PDF** (`format=pdf`, ADR-0010): renderuje sa cez Typst z verziovanej šablóny organizácie (vstavané šablóny `observations`, `alarms`, `audit`, `generic`), výsledok je záznam `core.report` (hash dát, hash súboru, kotva audit trailu, verzia šablóny), sťahovanie overuje hash. Limit 3 000 riadkov v PDF, plné dáta v CSV/JSONL.
- **Šablóny ako obsah (content framework, ADR-0010):** `core.report_template` (kind, locale, version, body, checksum, valid_to, reason) — nová verzia = nový riadok; API `GET/PUT/DELETE /api/v1/report-templates/{kind}`, `POST …/preview` (PDF nad vzorovými dátami, nič sa neukladá), `GET …/contract` (vzorový `data.json`). Editor v UI (`/settings`): zdrojový kód + náhľad + uloženie s dôvodom + návrat na vstavanú; blokový dizajnér fáza 2. Oprávnenie `content.write` (engineer, org_admin).
- **Branding a téma (ADR-0011):** organizácia nastaví logo (PNG/SVG/JPEG/WebP/GIF ≤ 256 KiB, uložené v `organization.branding`), hlavnú farbu, písmo, zaoblenie a veľkosť písma; platí v UI (svetlá/tmavá téma podľa systému alebo voľby používateľa) aj v hlavičke PDF. Pätička produktu je pevná (ctrl32 · telemetry, GitHub, licencia) a nedá sa zmeniť.
- **Responzívne UI a PWA (ADR-0011):** mobil a tablet s pevnou šírkou obsahu (bez horizontálneho pretekania, tabuľky rolujú vo vlastnom kontajneri), manifest + ikony + service worker pre statický shell (dáta sa nekešujú). Bez prihlásenia jednoduchá úvodná stránka.
- **Jazyk a lokalizácia (ADR-0009):** všetko v angličtine (kód, API, UI predvolene, logy, dokumentácia); ďalšie jazyky cez i18n (go-i18n + katalógy v UI), SK a CZ ako prvé preklady; regulačné šablóny reportov per locale; formáty dátumu, čísel a pásma per organizácia.

---

## 9. Pripojenie zariadení: protokol a knižnice

### 9.1 Protokol zariadenia (jeden, zdokumentovaný, verziovaný)

- **Transport:** MQTT 3.1.1/5 cez TLS (broker v procese) alebo HTTPS POST pre zariadenia bez MQTT.
- **Autentifikácia:** token zariadenia (Bearer/MQTT password) alebo klientsky certifikát; **provisioning** claim kódom (v UI vytvoríš zariadenie → claim kód/QR → zariadenie ho pri prvom štarte vymení za token) alebo predgenerovaným tokenom.
- **Témy:** `d/{id}/telemetry`, `d/{id}/attributes`, `d/{id}/status`, `d/{id}/cmd` (server → zariadenie), `d/{id}/cmd/ack`; alias `v1/devices/me/telemetry` v štýle ThingsBoardu pre migráciu existujúceho kódu.
- **Payload:** JSON `{"ts": 1758620000, "v": {"temp": 4.2, "rh": 61}}` alebo pole (dávka/backfill); **CBOR** ako voliteľný kompaktný formát pre STM32/LoRa-like obmedzené linky; heartbeat s batériou a RSSI; `ts` je čas zariadenia (NTP), inak server.
- **Sémantika:** QoS 1, idempotencia podľa `(device, seq)`, backfill označený; shared atribúty sa doručujú retained správou; príkazy s ID a povinným ack.

### 9.2 Knižnice (spoločné C jadro + porty)

| Cieľ | Forma | Sieť/TLS | MQTT | Buffer offline |
|---|---|---|---|---|
| **jadro `ctrl32-telemetry-c`** | C99, bez dynamickej alokácie (alebo konfigurovateľný alokátor), platformové rozhrania: `net_send/recv`, `time_now`, `storage_read/write`, `log` | cez port | cez port (adaptér na klienta platformy) | kruhový buffer v RAM/flash cez `storage` rozhranie, backfill po obnove |
| **Arduino** (ESP32, ESP8266, RP2040 W, dosky s Ethernetom) | knižnica v Arduino Library Manager + PlatformIO | WiFiClientSecure / Ethernet + BearSSL | PubSubClient alebo ArduinoMqttClient cez adaptér | LittleFS/EEPROM |
| **ESP-IDF** | komponent (`idf_component.yml`) | esp-tls | esp-mqtt (vstavaný) | NVS/SPIFFS |
| **STM32** | CMSIS-Pack / CMake knižnica; FreeRTOS + LwIP (Ethernet) alebo modem cez AT | mbedTLS | coreMQTT (MIT) ako predvolený prenosný klient | vnútorná flash/externá EEPROM cez `storage` |
| **ctrl32** | integrácia do tvojho systému programovania ESP32: knižnica ako komponent + **provisioning API** (ctrl32 editor vytvorí zariadenie na serveri a vloží token do projektu) | ako ESP-IDF | ako ESP-IDF | ako ESP-IDF |

**Funkcie knižnice (spoločné pre všetky porty):** telemetria (dávky, backfill z bufferu), atribúty client/shared (prijímanie konfigurácie), heartbeat/status, príkazy s ack, **konzola** (registrácia príkazových handlerov, stream logov, zapína sa až shared atribútom), **FOTA klient** (stiahnutie na bloky, overenie SHA-256 + Ed25519 podpisu vstavaným verejným kľúčom, aplikácia, ohlásenie novej verzie, rollback), provisioning claim kódom. Príkladové projekty pre každý port: teplota (DS18B20/Pt1000), impulzný vstup (S0 elektromer), Modbus RTU cez RS485 (ESP32 ako brána k elektromeru), digitálne stavy, príkaz na výstup, FOTA demo. Knižnice sú Apache-2.0, aby sa dali vložiť do akéhokoľvek firmvéru vrátane uzavretého.

### 9.3 Konektory a integrácie

Jeden rámec pre všetko, čo nesie dáta dnu alebo von: **konektor** = typ + konfigurácia s verziou + stav + vlastný log + mapovanie na profily/dekodéry. Konektory sú skompilované v binári (žiadne dynamické pluginy — validovaný artefakt ostáva jeden), konfigurujú sa v UI, každá zmena ide cez audit trail. Rovnaký vzor ako „Integrations + Data converters" v ThingsBoard PE, ale v jadre a zadarmo.

**Vstupné konektory:**

| Konektor | Fáza | Poznámka |
|---|---|---|
| MQTT broker v procese (zariadenia s knižnicou, ľubovoľný MQTT klient) | 1 | token/certifikát per zariadenie, ACL na témy |
| HTTP/HTTPS ingest + CSV import | 1 | pre všetko ostatné a migráciu histórie (kontrola integrity, Annex 11 cl. 4.8) |
| LoRaWAN: gateway so vstavaným NS (MQTT/HTTP), ChirpStack v4 (MQTT/gRPC), The Things Stack (webhook) | 1 | dekodéry v jadre, verziované; katalóg profilov pre bežné senzory (hardvérový research = referenčný zoznam s overeným bufferovaním) |
| MQTT most na cudzí broker (Mosquitto, EMQX, AWS IoT, Azure IoT Hub cez MQTT) | 2 | odber tém, mapovanie tém → zariadenia/datastreamy, TLS, credentials šifrované |
| OPC UA klient (`gopcua/opcua`, MIT) | 2 | subscriptions aj polling, bezpečnostné politiky a certifikáty, prehliadanie adresného priestoru, `SourceTimestamp`/`StatusCode` |
| Modbus TCP/RTU (`simonvetter/modbus`, MIT) | 2 | register mapy ako verziované profily (YAML), plánovač pollingu, RS485 na serveri alebo cez zariadenie s knižnicou |
| Generický webhook dnu (JSON s mapovaním) | 2 | pre cloudy senzorov tretích strán |
| M-Bus/wM-Bus, BACnet, SNMP, S7, Sigfox | 3 | podľa dopytu implementácií |

**Výstupné integrácie:**

| Integrácia | Fáza | Poznámka |
|---|---|---|
| Webhooky von (udalosti: nové meranie/alarm/zmena stavu/príkaz) | 2 | HMAC podpis, opakovanie s exponenciálnym odstupom, `webhook_delivery` log |
| MQTT publish von (na cudzí broker) | 2 | zrkadlenie datastreamov pre SCADA, Node-RED, Home Assistant |
| OPC UA server (vystaviť datastreamy ako uzly) | 3 | most na tvoje C++ SCADA jadro alebo legacy HMI |
| E-mail, SMS, push, hlas | 1 | kanály notifikácií (kap. 6) |
| Export do súboru/S3/SFTP podľa plánu | 2 | CSV/JSON dávky pre BI a archiváciu |

**Stav 23. 9. 2026 (0.5.0):** prvý konektor je **ChirpStack HTTP integrácia** — ChirpStack v4 posiela každú udalosť na `POST /api/v1/ingest/chirpstack/{connector_id}?event=up` s hlavičkou `Authorization: Bearer c32c_…` (tajomstvo per konektor, v DB len hash). ChirpStack už spustil kodek profilu zariadenia, takže dekódovaný `object` sa mapuje na kľúče datastreamov rovnako ako MQTT telemetria; surová udalosť je záznam s `connector = lorawan.chirpstack` a meta (rssi, snr, fcnt, fport, brána). Zariadenia sa podľa DevEUI vytvárajú automaticky (voliteľné), `status` udalosť dopĺňa batériu. Vlastný JS engine na kodeky nie je potrebný do chvíle, kým nepríde konektor bez vlastného dekódovania (MQTT LoRaWAN uplinky bez kodeku, Modbus).

### 9.4 Prehliadač správ (message viewer)

Nástroj implementátora číslo jeden: **živý a historický pohľad na každú správu**, ktorá prešla konektorom — dnu aj von.

- Pre zariadenie alebo konektor: čas prijatia, smer, transport, téma/URL/uzol, surový payload (hex + text + JSON), metadáta (RSSI, SNR, fcnt, OPC UA status, Modbus výnimka), **výsledok dekódovania** (vzniknuté observácie alebo chyba s dôvodom), autentifikačné zlyhania, zahodené duplicity.
- Živý stream cez WebSocket s filtrami (zariadenie, konektor, smer, len chyby), pauza, vyhľadávanie v histórii (nad `raw_message`), export.
- **Opätovné dekódovanie:** po oprave dekodéra sa vybrané surové správy pustia znova → vznikne nová `series_version` (surové dáta sa nikdy nemenia, P-19); akcia je v audit traile.
- **Simulácia:** odoslať testovaciu správu „ako zariadenie" na overenie dekodéra a pravidiel — označená ako simulovaná, nikdy sa nemieša s reálnymi dátami (v GxP prevádzke ju možno zakázať).
- Oprávnenie: rola Správca zariadení/Implementátor; payloady môžu obsahovať citlivé dáta → prístup logovaný.

### 9.5 FOTA (aktualizácia firmvéru cez sieť)

Pre zariadenia s našimi knižnicami; cudzie zariadenia (LoRaWAN) majú vlastnú FUOTA mimo nás.

- **Register firmvéru** (append-only): binár, verzia (semver), hardvérové ID/profil, min. kompatibilná verzia, SHA-256, **podpis Ed25519** a ID kľúča, poznámky k vydaniu, kto nahral a kedy.
- **Podpisovanie:** firmvér podpisuje jeho autor (implementátor) vlastným kľúčom **mimo servera**; server podpis len ukladá a distribuuje; **zariadenie má verejný kľúč vo firmvéri a overuje podpis aj hash pred zápisom**. Server ani jeho správca nedokáže podstrčiť firmvér, ktorý autor nepodpísal. Nepodpísaný firmvér sa dá nahrať len s výslovným príznakom „vývojový" a zariadenia v produkčnom režime ho odmietnu.
- **Kampane:** cieľ = zariadenia / skupina / profil; plán (hneď, okno, po pripojení); **postupné nasadenie** (kanárik → percento → všetci) s automatickým zastavením pri prekročení miery zlyhaní; stav per zariadenie (`queued`, `downloading`, `verifying`, `applying`, `rebooting`, `updated`, `failed:<dôvod>`, `rolled_back`); každá kampaň a zmena v audit traile (v GxP je FOTA zmena validovaného zariadenia — spadá pod change control zákazníka).
- **Prenos:** HTTPS na blokoch s obnovením (`Range`) alebo MQTT bloky pre zariadenia bez HTTP; ohlásenie cez shared atribút `fw.target` + príkaz; rýchlostné limity, aby kampaň nezahltila LoRa-like linky (netýka sa) ani slabé siete.
- **Aplikácia a rollback v knižnici:** ESP-IDF a Arduino-ESP32: OTA partície + `app rollback` (nová verzia sa musí ohlásiť serveru do N s, inak sa vráti stará); STM32: **referenčný bootloader** (dual-bank alebo dva sloty v externej flash) s overením podpisu a rollbackom, dodaný s knižnicou; ctrl32: cez ESP-IDF cestu.
- **Bezpečnosť:** kľúče kampaní a nahrávanie firmvéru = samostatné oprávnenie; súbory firmvéru nemenné; sťahovanie autentifikované tokenom zariadenia; rotácia podpisových kľúčov cez shared atribút s prechodným obdobím (dva kľúče vo firmvéri).

### 9.6 Vzdialená konzola zariadenia (remote console)

**Čo to je:** obojsmerný textový kanál medzi UI a **firmvérom zariadenia** (`d/{id}/console/in` a `/out`): zariadenie streamuje svoje logy/printy, správca posiela riadky, ktoré spracujú **príkazové handlery definované vo firmvéri** (`reboot`, `set interval 60`, `read adc 3`, `wifi status`…). Knižnica dodáva registráciu príkazov, help, históriu a bezpečné predvoľby.

**Čo to nie je:** shell na server ani na OS zariadenia. Server žiadny webový terminál do hostiteľa nemá (správa hostiteľa je vec nasadzovateľa; dodávame CLI `ctrl32-telemetry admin …` spustiteľné lokálne na serveri).

- **Zapnutie per zariadenie** shared atribútom `console.enabled` (predvolené vypnuté); knižnica konzolu bez tohto atribútu ani nezaloží.
- **Relácia:** samostatné oprávnenie (Implementátor/Správca zariadení), MFA re-autentifikácia pri otvorení, časový limit relácie, jeden aktívny operátor, **každý vstupný riadok a každý výstup sa ukladajú do `console_log` (append-only) s identitou a časom** — konzola je auditovaný kanál, nie obchádzka auditu.
- **Ochrany:** rate limit na vstup, maximálna dĺžka riadku, zoznam povolených príkazov v profile zariadenia (voliteľne), v GxP prevádzke možnosť konzolu globálne zakázať konfiguráciou s dôvodom; výstup zariadenia sa filtruje na kontrolné znaky.
- **Použitie:** oživovanie v teréne, diagnostika WiFi/LoRa/Modbus na strane zariadenia, čítanie hodnôt pred pripojením datastreamov.

### 9.8 SCADA postupne (súhrn, detail v ADR-0008)

- **Stupeň 1 (fáza 1):** historian, alarmy s eskaláciou, príkazy s ack, shared atribúty — už v jadre.
- **Stupeň 2 (fáza 2):** **JSON formát HMI obrazoviek v1** (navrhne sa už vo fáze 1), runtime viewer v Lit/SVG so živými hodnotami cez MQTT-over-WS, základná ISA-101 knižnica widgetov; zápis hodnôt z obrazovky = príkaz cez platformu s oprávnením a auditom.
- **Stupeň 3 (fáza 3):** webový HMI editor (engine s čistou licenciou; tldraw vylúčený), riadiaci zápis do OPC UA/Modbus cez konektory, setpointy so štyrmi očami a dôvodom, receptúry/limity ako verziovaná konfigurácia.
- **Stupeň 4 (fáza 3+):** `ctrl32-runtime` v C++ — OPC UA server aj klient (open62541), S7, Modbus s milisekundovými cyklami, deterministické skenovanie, lokálna logika, redundancia hot standby, edge nasadenie; komunikuje s platformou cez MQTT (Sparkplug-B-like) alebo gRPC, platforma bez neho funguje naplno.
- Zásady: obrazovky sú dáta v DB s verziami a auditom; každý riadiaci zápis má identitu, čas, ack a dôvod; bezpečnostné blokády ostávajú v PLC/runtime. legacy HMI ostáva pre súčasných zákazníkov, kým ctrl32 HMI nedozreje.

### 9.7 REST API

- **OpenAPI 3.1**, verziované (`/api/v1`), **UI je jeho klient** — čo vie UI, vie API. Autentifikácia: relácia (UI) alebo **API tokeny s rozsahom** (čítanie/zápis per oblasť, per zákazník) a expiráciou; každý mutujúci volanie v audit traile s identitou tokenu a jeho vlastníka.
- **Oblasti:** organizácia/zákazníci/prevádzky/assety/datastreamy/zariadenia/profily/konektory (CRUD), telemetria (zápis cez HTTP ingest; čítanie surové aj agregované — min/max/avg/last/count za okná, stránkovanie kurzorom, limity rozsahu), atribúty, príkazy, alarmy (zoznam, potvrdenie s dôvodom), pravidlá, dashboardy, FOTA (firmvér, kampane, stavy), konzola (otvorenie relácie), audit trail a prístupový log (čítanie, export), reporty (generovanie, stiahnutie), používatelia a roly, zálohy a stav systému, prehliadač správ (história), webhooky (registrácia).
- **Živé dáta:** WebSocket (JSON) alebo MQTT-over-WebSocket z brokera v procese s rovnakými oprávneniami.
- **Ochrany:** rate limit per token, veľkostné limity, idempotenčné kľúče pre zápisy, CORS len explicitne, chybové odpovede bez interných detailov.
- **Klienti:** generovaný TypeScript klient (pre UI aj tretie strany), neskôr Python; Postman/Bruno kolekcia v repe; **provisioning API** pre ctrl32 editor je časť tohto API.

---

## 10. Technologický stack (overené 23. 9. 2026)

| Vrstva | Voľba | Verzia / licencia | Prečo |
|---|---|---|---|
| Backend | **Go** | 1.27; Go 1 kompatibilitný sľub; reprodukovateľný toolchain | jeden statický binár pre Windows aj Linux, minimum závislostí, embedded broker a UI |
| DB | **PostgreSQL 18** + pg_partman 5.5 + pgBackRest (Linux) | PostgreSQL License (podpora do 11/2030); MIT | jediná závislosť; partície; pribaliteľné do inštalátora |
| MQTT | **mochi-mqtt** v procese | MIT; MQTT v5 + 3.1.1, TLS, WS, shared subscriptions | žiadna samostatná služba |
| OPC UA / Modbus | gopcua/opcua, simonvetter/modbus | MIT / MIT | natívne Go klienty |
| Web Push | SherClockHolmes/webpush-go | MIT | VAPID + RFC 8291 šifrovanie, žiadna externá služba okrem push služby prehliadača |
| LoRaWAN NS | ChirpStack v4 (voliteľné, Linux) | MIT | viac gatewayov |
| Frontend | **TypeScript 7 + Lit 3.3 + Vite 8**; uPlot 1.6 (+ ECharts 6.1) | Apache-2.0 / BSD-3 / MIT | web komponenty na štandardoch, minimálny treadmill |
| PDF | **Typst 0.15** CLI + go-typst | Apache-2.0 / MIT | PDF/A, diakritika, deterministické |
| Bezpečnosť | x/crypto/argon2, pquerna/otp, go-webauthn | BSD / MIT | |
| Ostatné | go-mail, gocron v2, go-i18n, kardianos/service | MIT / zlib | SMTP; cron; SK/EN/CZ; Windows služba + systemd |
| Knižnice zariadení | C99 jadro; coreMQTT (MIT) pre STM32; esp-mqtt; PubSubClient/ArduinoMqttClient | MIT | |
| Ikony / UI kit | **Tabler Icons + Tabler flags**; Tabler UI CSS (Bootstrap 5) ako kandidát dizajnového systému v light DOM s Lit — potvrdí PoC | MIT | jednotná ikonová sada, ADR-0009 |

**Licenčná politika tretích strán (ADR-0009):** len permisívne licencie triedy MIT (MIT, BSD, ISC, zlib, Apache-2.0, PostgreSQL License); žiadny copyleft (GPL/AGPL/LGPL), BSL/SSPL ani proprietárne; výnimka len cez ADR (kandidát: open62541, MPL-2.0, pre C++ runtime); kontrola licencií v CI + licencie v SBOM ako brána releasu.

**Vedome nepoužijeme:** EMQX ≥ 5.9 (BSL), immudb (BSL, druhá DB), TimescaleDB TSL (zákaz úpravy schémy zákazníkom), InfluxDB 3 Core (72 h limit), Node/npm na backend, wkhtmltopdf (archivované, CVE 9.8), QuestPDF (licenčná pasca), headless Chromium v produkte.

**Alternatívy k Go:** .NET 10 LTS (8/10), Rust (7/10) — dôvody v `01-technicky-research.md`. **Prečo nie C++:** webová aplikácia s databázou; tvoje C++ SCADA jadro ostáva samostatné a pripojí sa cez OPC UA/MQTT.

---

## 11. Inštalácia a prevádzka (čo dodávame, čo nie)

**Dodávame:** inštalátor pre Windows Server (EXE/MSI podpísaný certifikátom ctrl32; pribalený PostgreSQL 18 alebo existujúci; Typst; služba; sprievodca prvým spustením: správca, TLS, NTP kontrola, SMTP, cieľ záloh, profil intranet/internet), Linux balíky (`.deb`/`.rpm`/tarball + systemd; PostgreSQL z PGDG), Docker compose ako alternatívu, zálohovací modul (pg_basebackup + WAL alebo pgBackRest, GFS, cieľ mimo servera, šifrovanie, **mesačný automatizovaný test obnovy**), release pack pri každej verzii (kap. 12), sprievodcu hardeningom, dokument o zodpovednosti nasadzovateľa, dimenzovanie (typicky 2 vCPU / 4 GB / 100 GB).

**Nedodávame:** hosting, prevádzku, monitoring cudzích inštancií, kalibrácie, hardvér. Voliteľne prevádzkujeme **verejnú demo inštanciu** s ukážkovými dátami (pravidelne resetovanú) — to je marketing implementácie, nie služba.

---

## 12. Dokumentácia a validačná pripravenosť ako súčasť systému

Generuje sa z repozitára, aby ju implementátor a QA zákazníka mohli použiť bez nás:

- **URS** ako štruktúrovaný súbor (P-xx → URS-xxx, riziková trieda, GMP kritickosť); šablóna URS pre zákazníka sa generuje.
- **RTM** generovaná z testov s tagmi `URS-xxx`: požiadavka → test → výsledok → verzia.
- **IQ/OQ šablóny** s ID testov (IQ-01…, OQ-A…O), automatizované testy prikladajú výstup a snímky; kombinované IOQ.
- **Release pack** generovaný: release notes, klasifikácia zmien (GxP áno/nie, dotknuté URS), výsledky testov, odporúčaný regresný rozsah, migračné kroky + kontrola integrity, SBOM a CVE vyhlásenie, hash a podpis.
- **Matice zhody:** vyhláška 129/2012 § 30 ods. 8 / § 31 ods. 3 po bodoch; Annex 11 (2011) po klauzulách; EU GDP 3.2.1/3.3; draft Annex 11 kap. 8/11/12/16/17 ako „pripravenosť". Vždy v podpornom registri: *„systém podporuje požiadavku … prostredníctvom …; zodpovednosť za validáciu v zamýšľanom použití zostáva na prevádzkovateľovi."*
- **QMS-light vývojára** (verejné): SDLC, code review (pri jednom vývojárovi: automatizované brány + periodická externá revízia, draft 2.3), release a change proces, incident/CAPA, bezpečnostná politika, model hrozieb.
- **Šablóny pre implementátorov** (nie súčasť systému, ale repa): zmluva o kvalite (9 bodov draft 7.5), SLA/KPI, supplier dossier, kalibračný postup.

---

## 13. Roadmapa (po vrstvách, nie po zákazníkoch)

| Fáza | Trvanie | Obsah | Kritérium |
|---|---|---|---|
| **0 — Návrh a PoC** | 4–6 týždňov | Rozhodnutia R1–R7; licencia a verejný repozitár; URS v0.1 + posúdenie rizík + model hrozieb; ADR; schéma DB v1 (append-only + audit **prvé**); protokol zariadení v0.1; PoC v Go: MQTT z ESP32 (Arduino aj ESP-IDF) + LoRaWAN uplink z gatewaya → observation → graf → alarm → e-mail; beží ako služba na Windows aj Linuxe | end-to-end na reálnom hardvéri; stack potvrdený |
| **1 — Jadro v1.0** | ~4–5 mesiacov | Rámec konektorov + MQTT broker + HTTP + LoRaWAN (gateway NS, ChirpStack MQTT, TTS webhook); profily/dekodéry; **prehliadač správ** (živý + história + opätovné dekódovanie); observácie, medzery, backfill, drift; pevné dashboardy a trendy; alarmy + notifikácie + eskalácia + alarmový log; príkazy do zariadení (základ) a shared atribúty; **vzdialená konzola zariadenia** (v1: stream logov + príkazy s plným logovaním); IAM (organizácia, zákazníci, roly, TOTP, prístupový log); audit trail s hash chain a kontrolnými bodmi; systémový log s exportom; **REST API v1 (OpenAPI, tokeny s rozsahom, UI ako klient)**; exporty/import; zálohy + test obnovy; inštalátory Win/Linux + Docker; profily intranet/internet; i18n SK/EN/CZ; **knižnice Arduino + ESP-IDF v1.0 (telemetria, atribúty, príkazy, konzola, buffer)**; balík GxP teploty; RTM z testov; SECURITY.md, hardening guide, dokument o zodpovednosti | verejné vydanie 1.0; regresná sada; matica zhody 129/2012 a Annex 11 |
| **2 — FOTA, priemysel, energie, HMI viewer, GxP naplno** | ~4–5 mesiacov | **FOTA** (register podpísaného firmvéru, kampane s postupným nasadením, rollback; ESP-IDF/Arduino-ESP32 cesta); OPC UA klient, Modbus TCP/RTU s katalógom register máp, MQTT most, generický webhook dnu; **výstupné integrácie** (webhooky von s HMAC, MQTT publish von, plánované exporty); `counter` odvodenia a 15-min profily; pravidlá (podmienka → akcia); dashboard builder; **HMI viewer nad JSON formátom obrazoviek v1 + ISA-101 widgety**; e-podpis; štyri oči; MKT + report odchýlky; periodický balík; IOQ šablóny + release pack generátor; **knižnice STM32 (FreeRTOS + LwIP + coreMQTT aj modem cez AT) s referenčným bootloaderom pre FOTA + integrácia ctrl32 (provisioning API)**; balík Energie; OIDC | 2.0; externý penetračný test a zverejnený súhrn |
| **3 — SCADA a rozšírenia** | priebežne | **HMI editor**, riadiaci zápis do OPC UA/Modbus so štyrmi očami, receptúry/setpointy; **`ctrl32-runtime` (C++)**: OPC UA server + klient (open62541), S7, ms cykly, redundancia, edge; M-Bus/BACnet/SNMP/Sigfox podľa dopytu, mapovací modul, white-label pre implementátorov, grafický rule-chain (ak bude dopyt), LDAP, HA návod (PG replika + runtime pár), Python klient API | podľa implementácií |

Poradie vo fáze 1: **schéma a append-only/audit → bezpečnostná základňa → zber → UI.**

---

## 14. Riziká

| Riziko | Dopad | Opatrenie |
|---|---|---|
| „Univerzálny v celom rozsahu" naraz (ThingsBoard pasca) | roky bez v1.0 | univerzálny model a API od začiatku; funkcie po vrstvách; v1.0 má jasný rozsah |
| Bezplatný systém = nikto nezodpovedá za nasadenia | incidenty na cudzích inštanciách, reputácia | bezpečné predvoľby, hardening guide, dokument o zodpovednosti, licencia bez záruky, bezpečnostná politika a bulletiny, upozornenie na zastaranú verziu |
| Bus factor 1 | dôvera veľkých firiem | verejný zdrojový kód, jednoduchý stack, dokumentácia z repa, QMS-light verejný |
| Windows prevádzky bez ChirpStacku | obmedzenie LoRaWAN | gateway NS ako default |
| Štyri porty knižnice | údržba | jedno C jadro + tenké porty; testy protokolu proti serveru v CI; STM32 len s FreeRTOS+LwIP a coreMQTT ako referencia |
| FOTA ako vektor útoku (podstrčený firmvér) | ovládnutie flotily zariadení | podpis autora mimo servera, overenie podpisu aj hashu na zariadení, rollback, postupné nasadenie s automatickým zastavením, samostatné oprávnenie, audit |
| Vzdialená konzola ako obchádzka auditu alebo backdoor | únik dát, neauditované zmeny | predvolene vypnutá, zapnutie per zariadenie, MFA pri otvorení, plný append-only log vstupov aj výstupov, časový limit, zoznam povolených príkazov, možnosť globálne zakázať |
| Prehliadač správ ukazuje citlivé payloady | únik dát | samostatné oprávnenie, logovaný prístup, retencia surových správ podľa politiky |
| Regulačné tvrdenia | strata dôvery u QA | len overené citácie (`00-regulacny-research/README.md`), podporný register |
| Dependency churn frontendu | validovaný stav | pinované verzie, vendorovaný build, `npm audit` mimo release cesty (vstup do change control) |

---

## 15. Otvorené rozhodnutia (potrebujem od teba)

**Vyriešené (stav 25. 9. 2026):** 1 → binárky zdarma pod Apache-2.0 s dodatočnými podmienkami (zákaz predaja a plateného hostingu, pevná pätička), zdrojový kód po 1.0 (ADR-0002, `LICENSE`). 2 → Go 1.27 + PostgreSQL 18; UI zatiaľ bez frameworku, Lit + Tabler po 1.0 (ADR-0013). 3 → repozitáre `ctrl32-telemetry` (server, UI, inštalátory, dokumentácia v `docs/`) a `ctrl32-telemetry-c` (C jadro, Arduino a ESP-IDF porty). 4 → STM32 otvorené (bez hardvéru). 5 → zatiaľ knižnica + provisioning API; správa zariadení z editora ctrl32 otvorená. 6 → áno: `https://telemetry.digital/` je od 24. 9. produkčná inštancia (nie resetovaná). 7 → OPC UA (0.13.0) pred Modbus (0.20.0). Pôvodné znenie otázok ostáva nižšie.

1. **Licencia a zverejnenie:** Apache-2.0 pre server aj knižnice + verejný repozitár (odporúčam), alebo AGPL-3.0 pre server, alebo „zdarma, ale bez zdrojákov"? Verejný kód je zároveň najsilnejšia exit strategy a validačný argument.
2. **Stack:** Go + PostgreSQL 18 + Lit 3 (odporúčam), alebo .NET?
3. **Repozitár:** nový verejný `ctrl32-telemetry` (server) + `ctrl32-telemetry-c` (knižnice)? Kam presunúť tieto dokumenty (odporúčam do `docs/` nového repa).
4. **STM32 referenčná platforma:** FreeRTOS + LwIP (Ethernet) s coreMQTT ako jediná podporovaná kombinácia v 2.0, alebo aj bare-metal/modem cez AT?
5. **ctrl32 integrácia:** stačí knižnica ako komponent + provisioning API, alebo má ctrl32 editor vedieť aj spravovať zariadenia priamo (vytváranie, tokeny, stav)?
6. **Verejná demo inštancia:** áno/nie (odporúčam áno, resetovaná denne).
7. **Poradie vo fáze 2:** OPC UA pred Modbusom, alebo naopak, podľa toho, kde budú prvé implementácie.

---

## 16. Ako budeme postupovať (proces)

- **Dokumenty pred kódom:** URS, posúdenie rizík, model hrozieb, protokol zariadení a ADR sú prvé commity.
- **Kritická cesta test-first:** zber → observation → medzera → alarm → notifikácia → audit trail → bezpečnostné kontroly majú automatizované testy s URS tagmi od prvého dňa.
- **Bezpečnosť ako brána:** žiadny release bez `govulncheck`, SBOM, testov bezpečnostnej základne a podpisu artefaktu.
- **PoC pred záväzkom:** fáza 0 končí funkčným tokom na reálnom hardvéri na Windows aj Linuxe.
- **Týždenný rytmus:** ciele v `JOURNAL.md`, na konci týždňa stav a rozhodnutia.
- **Žiadne tvrdenie bez zdroja.**
