# Hardware research — commodity EU868 sensors, gateways, alternatives, calibration (verified 23 Sep 2026)

> Rešeršný report (EN) vypracovaný agentom na zadanie z 23. 9. 2026. Ceny **bez DPH**, pokiaľ nie je uvedené inak (iot-shop.de uvádza s 19 % DE DPH → vydelené 1,19). Neoverené položky sú označené **UNVERIFIED**. Slovenské zhrnutie a závery sú v `../01-technicky-research.md`.

**Method / caveats.** 5 parallel research threads + own fetches; ~700 page/PDF fetches of vendor datasheets, user guides, EU shops and SK/CZ lab price lists. WebSearch quota ran out mid-way, so the last third was verified by direct fetches only; testo.com/testo.sk returned HTTP 429 throughout; TME/SOS electronic 403. No SK/CZ distributor exists for Milesight or Dragino (partner pages: https://www.milesight.com/partner/find-channel-partner , https://www.dragino.com/buy.html) — buy from DE/NL/GR shops.

## 0. Legal baseline that drives the hardware choice (SK)
- Vyhláška 419/2025 Z. z. amends 129/2012 Z. z.; temperature provisions effective 1 Jul 2026 (https://www.zakonypreludi.sk/zz/2025-419). Consolidated § 30 ods. 8 (https://www.zakonypreludi.sk/zz/2012-129): *"Teplota sa meria pomocou kalibrovaného meradla,7a) ktoré umožňuje meranie a automatickú evidenciu teploty najmenej v dvojhodinových intervaloch. Denné záznamy o teplote sa uchovávajú 12 mesiacov."* Same sentence in § 31 ods. 3 and Príloha 1 (B.2.1, G.1–G.9). No calibration interval, no word "akreditovaný"; the 2-h automatic duty is for *temperature*.
- Footnote 7a → § 17 zákona 157/2018 o metrológii (https://www.zakonypreludi.sk/zz/2018-157): ods. 3 *"Kalibráciu povinne kalibrovaného meradla vykonáva ústav, alebo akreditované kalibračné laboratórium."* Our reading (interpretation, not official): certificate must come from SMÚ or an ISO/IEC 17025-accredited lab; a plain traceable "kalibračný list" from a non-accredited lab is not enough. Whether a Czech ČIA-accredited lab is accepted under § 17 — UNVERIFIED (EA-MLA argument).
- CZ reference: SÚKL FAQ recommends calibration "minimálně každých 5 let" (https://sukl.gov.cz/lekarny/musi-byt-pro-kontrolu-teploty-uchovavani-lecivych-pripravku); DIS-15 v4 for distributors: ≤24 months, ±0.5 °C max error for thermolabile storage, calibrate sensors "ve vazbě na používanou vyhodnocovací a registrační jednotku" (https://sukl.gov.cz/wp-content/uploads/2025/02/DIS-15_verze4_Sledovani-a-kontrola-teploty-pri-skladovani-a-preprave-leciv.pdf).

## 1. LoRaWAN temperature sensors — shortlist
S&F = onboard log with timestamps + automatic backfill after outage + retrieval by downlink.

| Model | Range / stated accuracy | Probe | S&F | Battery / life | Uplink | LoRaWAN | Price excl. VAT (source) | Cal. cert | Source |
|---|---|---|---|---|---|---|---|---|---|
| **Milesight TS301 / TS302** (LCD, 1/2 ch) | AF10 food PT100 class A ±0.4 °C (−40..125); A03 low-temp class A ±0.5 (−175..50); SP13 T/RH probe ±0.2 (0..65); SP12 DS18B20 ±0.5 | detachable, 1.5–2 m, sold separately | **Yes** 10 000 rec., retransmission + retrieval, CSV export | 2×ER18505; >5.2 y (1 probe, SF7, 10 min) | 10–1440 min | A, 1.0.2/1.0.3 | TS301 €77.90 / TS302 €88.90 (iot-shop), €74.80/€85.90 (invibit); AF10 probe €17.90 | Factory cal report incl.; ISO 17025 via Milesight partner labs on order; EN 12830 | https://www.milesight.com/iot/product/lorawan-sensor/ts30x ; https://resource.milesight.com/milesight/iot/document/ts30x-user-guide-en.pdf |
| **Milesight EM320-TH** (IP67, magnet, food-contact housing) | −30..60 °C; ±0.2 (0..60), ±0.3 (−30..0); RH ±2 % | built-in SHT41 (whole unit inside fridge) | **Yes** 3 000 rec. | 2×ER14505; >10 y | 1–1080 min | A, 1.0.2/1.0.3 | €50.90 (iot-shop) / €49.80 (invibit) | as above; EN 12830 | https://www.milesight.com/iot/product/lorawan-sensor/em320-th |
| **Milesight EM300-TH** | −30..70; ±0.3 (0..70), ±0.6 (−30..0); RH ±3 % | built-in | **Yes** 2 800 rec. | 1×ER18505; >10 y | 1–1080 min | A | €46.90 (iot-shop) / €49.80 (invibit) | as above; EN 12830 | https://resource.milesight.com/milesight/iot/document/em300-th-datasheet-en.pdf |
| **Milesight TS201** (12/2025) | −40..125; ±0.5 (−10..85) | detachable DS18B20 1.5 m | **Yes** 4 000 rec. | 1×ER14505; ~8.2 y | 1–1440 min | A | €53.90 + SP12 probe €8.90 | as above | https://www.milesight.com/iot/product/lorawan-sensor/ts201 |
| Milesight EM500-PT100 | T200: class A ±0.6; T050: class B ±0.5 (−40..40) | 3-wire PT100, 1.5 m | **Yes** 1 000 rec. | ER34615; 10 y | 1–1080 min | A | €139.80–223.80 | cal service only −30..70 | https://resource.milesight.com/milesight/iot/document/em500-pt100-datasheet-en.pdf |
| **Dragino LHT65N-E3 / -NE117** | built-in ±0.3; E3 DS18B20 ±0.5 (−10..85), ±2 below; NE117 TMP117 **±0.1 max (−20..50)** | E3: DS18B20 2 m; NE117 2 m | **Yes** 3 328 rec., Unix ts; needs PNACKMD=1 + SYNCMOD=1 → resend + poll (0x31) | built-in 2 400 mAh; "up to 10 y" | default 20 min | A, 1.0.3 | E3 €30.80 (exp-tech) / €34.90 (iot-shop); NE117 €44.95 (antratek) | LHT65N-NIS "NIST-traceable probe + certificate" $148.50 list; no 17025 | https://wiki.dragino.com/docs/LoRaWAN-End-Node/temperature-humidity-sensors/lht65n/ |
| **Dragino LTC2-LB** | PT100 class A probes, "±0.6 °C"; DR-LT −196..150 | 1–2× 3-wire PT100, 2 m | **Yes** 3 328 rec. | 8 500 mAh Li-SOCl2 | 20 min default | A, 1.0.3 | €59.85 (antratek; €49.46 @10+) + DR-LT probe €16.94 | none | https://wiki.dragino.com/docs/LoRaWAN-End-Node/temperature-humidity-sensors/ltc1-ltc2-lb/ |
| Dragino D20/D22/D23-LB | DS18B20 ±0.5 (max ±2) | 1/2/3× DS18B20 | **Yes** | 8 500 mAh | 20 min | A, 1.0.3 | €49.95 / €54.95 / €59.95 (antratek) | none | https://wiki.dragino.com/docs/LoRaWAN-End-Node/temperature-humidity-sensors/d20-d22-d23-lb/ |
| **Elsys ELT-2 + Dual PT1000 module** | electronics ±0.1 + Pt1000 class tolerance | 1–2× Pt1000 on terminals | **Partial**: queue ≤10 samples, no multi-day buffer | 1×AA 3.6 V; <10 y | configurable | A/C, 1.0.3 | ELT LoRa €133 + module €81 (elsys.se) | none | https://elsys.se/public/manuals/New/Dual%20PT1000%20module_Manual-1.pdf |
| **Adeunis TEMP2S** (IP68) | NTC ±0.2 (0..60), ±0.5 (−30..0) | 2× NTC 2 m | **Partial**: 24 samples/frame + repeats | 4 000 mAh; >10 y | periodic/threshold | A | €222.90 (iot-shop) | none | https://www.adeunis.com/wp-content/uploads/2024/01/Datasheet_TEMP-TEMP2S_LoRa-R_EU868_SIGFOX_RC1_EN.pdf |
| Tektelic TUNDRA + probe | ambient ±0.3; **probe NTC ±2 °C** | 10 kΩ NTC | **Yes** 3 000 entries | 1×C; 15 y | ≥1 min | A, 1.0.2 | €75.90 (iot-shop) | none | https://tektelic.com/wp-content/uploads/TEKTELIC_TUNDRA_UG_v3.2.pdf |
| Sensing Labs Senlab T | ±0.5 (−10..85) | Binder 5-pin | **Yes** + 10-day flush | 20 y | 10 min–24 h | UNVERIFIED | **no EU price found** | none | https://sensing-labs.com/versatile-temperatire-monitoring/ |
| Comet W0911 (LoRaWAN, ext. Pt1000) / W0910 / W0932 | ±0.2 °C / ±0.4 / — | Pt1000 CINCH | UNVERIFIED | AA Li; 3–10 y | 5 min–24 h | EU868 | €204 / €190 / €270 (Didactum DE) | **manufacturer traceable cert incl.**; accredited via external lab €80 | https://www.cometsystem.com/products/iot-wireless-thermometer-for-external-probe-lorawan-network/reg-w0911 |
| Netvox R718B120 (PT1000) | −40..200; ±0.5 | PT1000 | none documented | 2×ER14505 | 15 min | A, 1.0.2 | €146.90 | none | https://iot-shop.de/en/shop/nv-r718b120-netvox-r718b120-lorawan-wireless-temperature-sensor-with-pt1000-round-head-probe-6708 |

Rejected/notes: Decentlab DL-DS18 €772.90, no OTA backfill; MClimate HT cannot encode <0 °C; Enginko LW12TERWP ±1.0 °C; Talkpool OY1110 no S&F; Elsys ERS-2 0..50 °C only; legacy Dragino LSN50v2-D2x has **no** datalog — avoid; DS18B20 probes are ±2 °C below −10 °C → not for freezers. Milesight timestamps need LoRaWAN 1.0.3 + NS answering DeviceTimeReq (ChirpStack does).

## 2. Indoor LoRaWAN gateways
| Model | Backhaul | PoE | Built-in NS | Remote mgmt | Backup | Price excl. VAT | Source |
|---|---|---|---|---|---|---|---|
| **Milesight UG63** (mini) | Eth; opt. LTE Cat 1 dual-SIM | 802.3af splitter / USB-C | yes, max 20 devices; UDP, Basic Station | DeviceHub | UPS kit acc. | €117.80 Eth / €157.80 4G (invibit) | https://www.milesight.com/iot/product/lorawan-gateway/ug63 |
| **Milesight UG56** (industrial indoor) | Eth + Wi-Fi; opt. 4G | 802.3af / USB-C | yes, FUOTA | DeviceHub | 12 000 mAh UPS kit | €207.50 / €256.80 4G | https://www.milesight.com/iot/product/lorawan-gateway/ug56 |
| Milesight UG65 (IP65) | GbE + Wi-Fi; opt. 4G | 802.3af | yes | DeviceHub | UPS kit | €322.50 / €389.80 4G | https://www.milesight.com/iot/product/lorawan-gateway/ug65 |
| **RAK7268V2 / CV2** | Eth + Wi-Fi; CV2 **LTE Cat 4** | 802.3af/at | yes (1.0.3) + UDP + Basic Station | WisDM | RAK UPS PoE-03 €69 | from €187 | https://store.rakwireless.com/products/rak7268-8-channel-indoor-lorawan-gateway |
| **Dragino LPS8v2** | Eth + Wi-Fi; opt. 4G | **no** (USB-C 5 V) | **built-in ChirpStack** + Node-RED | Remote Management | none | €159 (invibit) / €126.90 (iot-shop) | https://www.dragino.com/products/lora-lorawan-gateway/item/228-lps8v2.html |
| Kerlink iFemtoCell / -evolution | Eth / Eth + 4G | 12 VDC | Basic Station; optional Wanesy SPN | Wanesy MC | none | €194.90 / €227 | https://www.kerlink.com/product/wirnet-ifemtocell-evolution/ |
| MikroTik wAP LR8G kit | Eth + Wi-Fi | passive PoE | **no NS**; UDP/LNS/CUPS | RouterOS | none | MSRP $149; EU price UNVERIFIED; LtAP LR8 LTE kit **discontinued** | https://mikrotik.com/product/wap_lr8g_kit |
| Tektelic Kona Micro Gen 2 | Eth + LTE | PoE | no; Basic Station | KonaFT | **built-in Li-ion up to 4 h** | €305.90 (n/a) | https://tektelic.com/products/gateways/kona-micro-iot-gateway/ |
| Browan MiniHub Pro / TTIG | Wi-Fi only | no | no | none | none | £85 / £79.95 | https://connectedthings.store/ |

## 3. Door/contact and power-failure sensors
| Model | Function | Battery | Heartbeat | Price excl. VAT | Source |
|---|---|---|---|---|---|
| **Dragino LDS02** | open/close + open count + last-open duration + open-too-long alarm | 2×AAA | 24 h | €18.92 | https://www.dragino.com/products/lorawan-nb-iot-door-sensor-water-leak/item/181-lds02.html |
| Dragino DS03A-LB (outdoor) | same, weatherproof | 8 500 mAh, ~5 y | 2 h | €51.90 | https://shop.dragino.com/index.php?product_id=264&rt=product%2Fproduct |
| Milesight WS301 | status + tamper | ER14250 | 18 h default | €47.50 | https://www.milesight.com/iot/product/lorawan-sensor/ws301 |
| **Milesight WS523 smart socket 16 A** | capacitor-backed **power-outage alert** + V/I/W/kWh | mains, Class C | 20 min | €63.45 (botland) | https://resource.milesight.com/milesight/iot/document/ws52x-user-guide-en.pdf |
| Netvox R718N1 / R718N17 CT clamp | current on fridge feed | 2×ER14505 | 1 800 s | €115.90–125.90 | https://iot-shop.de/en/shop?search=R718N |
| MC Technologies 163110 | 230 V loss detector | AA ≥10 y | daily | €138 | https://mc-technologies.com/en/produkt/163110/ |
Not suitable: Dragino LT-22222-L (dies with mains), Netvox R809A (no outage report).

## 4. Alternatives for a single pharmacy (2–4 sensors) and SK cellular coverage
| Solution | Accuracy / probe | Buffer & backfill | Radio / gateway | Hardware price | Cloud | Cal. cert | Source |
|---|---|---|---|---|---|---|---|
| **Efento NB-IoT logger, ext. probe** | ±0.5 (−10..85); internal ±0.4 | **40 000 meas., re-sends after outage** | NB-IoT direct, no gateway; 3×AA up to 5 y | starter kit $165; EUR UNVERIFIED | 5-year licence per sensor, price UNVERIFIED | ISO 17025 via external lab, extra | https://getefento.com/product/wireless-temperature-logger-with-external-probe-nb-iot/ |
| ELPRO ECOLOG-PRO 1TGi / 1TGe | ±0.5 int.; 1TGe ext. Pt100 ±0.3 | 31 000 values | LTE-M + NB-IoT, roaming SIM | 1TGi €202 / €267 with 1-yr licence | ~€65–70 / sensor / yr | 3-pt ISO 17025-traceable cert | https://eu.shop.elpro.com/products/ecolog-pro-1tgi |
| Comet W0711 Wi-Fi (Pt1000) / W0710 | ±0.2 / ±0.4 | memory yes, backfill UNVERIFIED | Wi-Fi; **mains only** | €285 / €302 + probe | €21 / device / yr | factory cert incl.; accredited €80 | https://www.cometsystem.com/products/reg-w0711 |
| Testo Saveris 2-T2 | ±0.3 | 10 000 values/channel | Wi-Fi; 12-month battery | $318 + probes | Basic free (3-month retention) | on request | https://testequipmentusa.com/testo-0572-2032-testo-saveris-2-t2-wifi-data-logger/ |
| TFA WeatherHub / TFA.me | ±0.5 | 90-day memory | 868 MHz proprietary → gateway | **Elso.sk pharmacy set (gateway + 3 sensors + 2 probes, ISO 17025 certs) €413** | free | included | https://www.elso.sk/sada-pre-automaticky-zaznam-teploty-a-vlhkosti-pre-lekarne-a-sklady-liekov/ |
| Elitech RCW-800W-TDE (2 probes) | ±0.5 | 20 000 rec. | Wi-Fi; 7-day battery | €76.05 | free / premium ~€12/yr | none | https://www.elitecheu.com/products/elitech-rcw-800w-5m-tde-eu-wireless-temperature-data-logger |
| Papouch PAPAGO 2PT Wi-Fi | 2× Pt100/1000 | timestamped memory | Wi-Fi, mains | €191.80 | none (Modbus/SNMP/HTTP) | none | https://en.papouch.com/papago-2pt-wifi-2x-thermometer-for-pt100-1000-with-wifi-p4844/ |

Approx. 3-year cost per pharmacy (3 sensors, excl. VAT): Efento NB-IoT ~€450–600 + SIM; Elitech ~€230–335; TFA/Elso €413 one-off; Testo T2 ~€1 030; Comet W0711 ~€1 130; ELPRO 1TGi ~€1 190. LogHub subscription €720–1 800 over 3 years.

**SK NB-IoT / LTE-M coverage (Sep 2026):** Orange — NB-IoT "pokrytie viac ako 99 % Slovenska", €1.23/month; IoT Connect €0.26/month incl. LTE-M (https://www.orange.sk/biznis/iot-sluzby/iot-pausaly). Slovak Telekom — NB-IoT a LTE-M "po celom Slovensku", IoT Low Power SIM from €1.29/month excl. VAT (https://www.telekom.sk/biznis/iot-konektivita). O2 — NB-IoT B20 for >94.6 % of population, LTE-M UNVERIFIED (https://business.o2.sk/nb-iot-narrow-band).

## 5. How LogHub and Efento structure their hardware (public facts)
- **LogHub** (R-DAS s.r.o., ISO 9001/27001): fully proprietary stack — Logger PLUS (internal T/RH), external thermistor probe variant, Logger LONG RANGE, Logger SOLO; **2.4 GHz proprietary radio** to the **Observer LITE central unit** (Wi-Fi uplink) (https://www.loghub.sk/zariadenia/). Accuracy ±0.5 °C; records every 15 min, stored 10 years. **Devices leased, "from €20/month" excl. VAT** (room €20–50, warehouse €50–100, hall €100+), calibration by device exchange; claims ISO/IEC 17025 calibration (lab not named), EN 12830:2018, Annex 11 / 21 CFR 11 validation (https://www.loghub.sk/en/pricing/). 400+ sites in SK.
- **Efento** (Kraków): BLE sensors + Efento Gateway, and NB-IoT/LTE-M sensors that need no gateway (CoAP + protobuf to Efento Cloud *or any other cloud*); 40 000-measurement buffer with backfill; no LoRaWAN line (https://getefento.com/products/). One-time 5-year licence per sensor; calibration from external ILAC/PCA lab, recal. every 2 years recommended.
- **PharmaControl.sk**: wireless fridge/room sensors, "ISO/IEC 17025 calibrated, 24-month validity", 10-day memory, price on request (https://www.pharmacontrol.sk/).

## 6. Calibration logistics (SK/CZ ISO 17025)
| Lab | Accreditation | Scope / on-site | Public price (excl. VAT) | Turnaround | Source |
|---|---|---|---|---|---|
| **ZTS Elektronika SKS, Nová Dubnica** | SNAS K-004 | RTD/TC/dataloggers; on-site yes | on request | ~10–14 days | https://www.elektronika.sk/kalibracia-teplomerov-a-meradiel-teploty |
| **ChiranaLAB (metrologia.sk)** | ISO 17025 (SNAS no. UNVERIFIED) | combined T/RH dataloggers; on-site −20..250 °C | on request | n/a | https://metrologia.sk/sluzby/teplomery-a-vlhkomery/ |
| **KALIST AKL** (lab Holešov CZ) | ČIA 2394, valid to 2030 | −196..1100 °C; dataloggers; on-site | on request | n/a | https://www.kalistakl.sk/kalibracia-teplomerov |
| SMÚ Bratislava | national institute | digital thermometers −40..300 | €41/point; RTD €89/pt | n/a | https://www.smu.sk/cennik/ |
| SLM n.o. | SNAS no. UNVERIFIED | dataloggers | €36.59 + €6.50 per extra point | n/a | https://www.slm.sk/cennik.html |
| Metershop.sk | "akreditovaná" UNVERIFIED | dataloggers, nationwide pickup | **€28 / 3 pts**, +€4/pt | 5–10 working days | https://metershop.sk/sluzby/kalibracia-teplomerov/ |
| ProTechnika.sk / Medipharm služby | lab not named | pharmacy thermometers | €44 / 4 pts ; €25 / 3 pts | ~10 days | https://www.protechnika.sk/kaliracie/prvotna-akreditovana-kalibracia-teploty/ |
| ČMI Praha stř. 1033 | national institute | −196..1550 | 1 960 CZK / 3 pts, 10+ pcs −40 % | express +50 % | https://cmi.gov.cz/Cenik_1033 |
| Exatherm | ČIA 2259 | 0..100 °C | **850 CZK / 3 pts** | express 3 d | https://www.exatherm.cz/ |
| Comet System | **not accredited**; accredited via external lab | SLU001 T €80, SLU003 T+RH €112 | n/a | https://www.cometsystem.cz/podpora/kalibrace-a-metrologie |

- Typical accredited 3-point calibration: **€16–45 per device in SK via resellers**, €32–50 SLM, ~€123 SMÚ; CZ 850–1 960 CZK. Turnaround 2–14 days. Register of SNAS labs: https://ais.snas.sk/web-report?type=valid&lang=sk.
- **Probe-only calibration:** labs calibrate a detached Pt100/Pt1000 probe, but per Comet ("z oddělených kalibračních listů na součásti systému nelze vyvodit kalibraci systému jako celku") and SÚKL DIS-15 §3.2, the certificate must cover the sensor *with* the evaluating/recording unit. No SK/CZ lab publicly offers reading the LoRaWAN uplink — **UNVERIFIED, confirm by phone with ZTS Elektronika, ChiranaLAB, KALIST**. Practical options: (1) calibrate the complete unit before installation and recalibrate by **unit swap** (LogHub model); (2) Milesight's own calibration service with ISO 17025 partners (https://www.milesight.com/iot/service/calibration-service); (3) for TS30x/LTC2-LB, calibrate the detachable probe accredited and document the transmitter's electronics check — acceptance by SK inspectors UNVERIFIED.

## 7. Recommendations
**(a) Pharmacy starter kit — 1 gateway + 3 sensors (excl. VAT)**
| Item | Model | Price |
|---|---|---|
| Gateway (Ethernet + LTE Cat 1 fallback) | Milesight UG63-4G (alt.: RAK7268CV2 €187 with PoE; budget: Dragino LPS8v2 €159 with built-in ChirpStack) | €157.80 |
| 2× fridge sensor, probe inside, LCD unit outside | Milesight TS301 + SP-AF10 PT100 class A probe (alt.: EM320-TH €50.90 sealed unit inside fridge) | 2 × €95.80 = €191.60 |
| 1× room T+RH | Milesight EM300-TH (or EM320-TH ±0.2) | €46.90 |
| Optional door | Dragino LDS02 | €18.92 |
| Optional power-loss | Milesight WS523 socket | €63.45 |
| Accredited 3-pt calibration ×3 | €28–44 each | ~€85–130 |
| **Total** | core kit ≈ **€480–530**; with door + power ≈ **€565–615**; + LTE SIM €1.3–2.5/month | |
Cheapest compliant variant: LPS8v2 + 2× EM320-TH + EM300-TH = €307.70 + calibration ≈ €400. Benchmarks: Elso.sk TFA set €413 one-off; LogHub €20–50/month.

**(b) Warehouse (GDP) kit — example 10 ambient zones, 2 cold rooms, 1 freezer (excl. VAT)**
| Item | Model | Price |
|---|---|---|
| Gateway with PoE, LTE Cat 4, fleet mgmt + UPS | RAK7268CV2 + RAK UPS PoE-03 | €256 |
| 2nd gateway for redundancy | Milesight UG56 | €207.50 |
| 10× ambient T/RH | EM300-TH €46.90 (or EM320-TH €50.90) | €469 |
| 2× cold room, 2 probes each | TS302 €88.90 + 2× AF10 €17.90 | €249.40 |
| 1× freezer −25..−15 °C | TS302 + 2× AF10 class A (alt.: LTC2-LB + DR-LT €76.79) | €124.70 |
| 3× doors | Dragino DS03A-LB | €155.70 |
| 3× power/compressor monitoring | WS523 per fridge or R718N1x CT | €190.35 |
| Accredited calibration 17 probes/units | €30–45 each | ~€510–765 |
| **Total** | ≈ **€2 150–2 420** hardware + calibration | |
Design notes: use Milesight/Dragino only for S&F (2 800–10 000 records = weeks at 10 min); set Dragino PNACKMD=1; run the NS with DeviceTimeReq; keep the mapping/qualification docs (EN 12830 units, per-probe certificates) per GDP.

## 8. Not verified / open items
Efento EUR prices, 5-year cloud licence cost and SIM cost; Comet W09xx backfill; Sensing Labs EU price; Elsys shop VAT status; Testo SK/CZ accreditation and Cloud pricing; MikroTik EU street prices; UG65/UG56 LTE category; O2 LTE-M; SNAS numbers of ChiranaLAB, SLM, Metershop's lab; whether SK inspectors accept ČIA (CZ) certificates under § 17 ods. 3; whether any lab calibrates a wireless unit by reading the uplink or accepts probe-only certificates with the radio unit installed.
