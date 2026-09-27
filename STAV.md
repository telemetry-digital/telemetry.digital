# Stav práce (aktualizované 27. 9. 2026)

## Hotové (na vetve `assistant/jolly-sagan-nzolwc`, otestované)

| Verzia | Čo | Testy |
|---|---|---|
| 0.37.0 | SCADA procesné obrazovky a editor (spec 001) | e2e + headless |
| 0.38.0 | Widgety dashboardov s nastaveniami ako ThingsBoard, filtre v grafoch, plná nastaviteľnosť (spec 002) | e2e 81 + headless |
| 0.39.0 | Redundantná databáza: kópia s bezpečným odpojením, priebežná replika, hot standby s failoverom (spec 003) | e2e 46 + headless |
| 0.40.0 | Toky v štýle Node-RED: vlastný engine v Go, 28 uzlov, editor, ladenie, verzie, štyri oči, prevod pravidiel, metriky, sk/cs (spec 004) | e2e 69 + headless, ~30 000 správ/s |
| 0.41.0 | Provisioning cez MQTT a smart home ako Home Assistant: profily provisioningu, ThingsBoard API, automatické datastreamy, MQTT účty s oddeleným priestorom tém, mosty, HA discovery, entity, stránky Domov a Zariadenia a pripojenia, uzly tokov, export do HA (spec 005) | e2e 33 + 39, headless, unit |
| 0.42.0 | Smart home, fáza 2: Shelly Gen2+/Gen3 natívne (RPC), Tasmota s vlastným formátom discovery, výber farby a teploty farby na karte svetla, uzly tokov MQTT vstup/výstup, incident pri výpadku mosta alebo účtu, premenovaná entita si drží meno (spec 006) | e2e 36, headless, unit |
| 0.43.0 | Energia (spotreba, výroba, náklady po hodinách/dňoch/mesiacoch, podiel meračov, príkon) a scény (zachytenie stavu, spustenie jedným klikom, uzol toku) (spec 008) | e2e 37, unit |
| 0.44.0 | Toky: podtoky (znovupoužiteľné časti, pripnuté verzie), uzol Rozdelenie, farebné skupiny na plátne (spec 009) | e2e 23, unit |
| 0.45.0 | Kamery: RTSP (H.264/H.265/MJPEG) a HTTP MJPEG, nepretržité nahrávanie s uchovaním a limitom disku, živý obraz a záznamy v prehliadači, časová os, export MP4, videosteny na viac monitorov (2 × 20), audit prezerania (spec 010) | e2e 44, unit |
| 0.46.0 | Kamery: widget, prehrávač s ovládaním a priblížením, timeshift (nahrávanie nezávislé od prehrávania), vzdialené kamery cez šifrovaný prenášač `camera-relay`, PWA aj pre iPhone, test 128 kamier (spec 011) | e2e 61, unit, záťaž |
| 0.47.0 | Grafický editor úvodnej stránky pred prihlásením: bloky, úprava textu priamo v náhľade, počítač/tablet/mobil, verzie, zverejnenie, obrázky (SVG sanitizované), prísna validácia, prihlásenie sa nedá odstrániť (spec 012) | e2e 33, unit |
| 0.48.0 | Kamery: pohyb a udalosti z ONVIF bez dekódovania, nahrávanie pri udalosti s predstihom, značky, PTZ s predvoľbami, vyhľadanie kamier v sieti, udalosti kamier v tokoch, všetky nastavenia videa v rozhraní (spec 013) | e2e 99, unit |
| 0.49.0 | Obrazovky (kiosky) pre dashboardy, SCADA aj videosteny: párovanie kódom, striedanie obsahu, zväčšenie kamier pri udalosti, ovládanie z velína (poslať kameru, identifikovať); skupiny kamier pre používateľov; incidenty pri výpadku kamery, obrazovky a plnom disku; umiestnenie obrazoviek a hromadné správy, výrobné obrazovky (rozsahy 8 h/12 h/dnes, prispôsobenie výške); bezpečnostná kontrola, 7 opráv (spec 014) | e2e 76, unit |
| 0.50.0 | Dôkazy z kamier: zamknutie úseku (nezmaže sa), SHA-256 súborov pri nahrávaní, podpísaný dôkazový balík a `verify-evidence`; obrazovky a widgety po slovensky (dátum, čas, popisy) (spec 015); maskovanie zón GDPR (spec 016) | e2e 29 + 20, unit |
| 0.51.0 | Kamery na pôdoryse: značka so zorným uhlom a stavom (v prevádzke, výpadok, udalosť), klik otvorí živý obraz, rešpektuje skupiny kamier, aj na kioskoch (spec 017) | e2e 78, unit |
| 0.52.0 | Prenášač vzdialených kamier ako služba: inštalátory pre Linux/Raspberry Pi a Windows, samokontrola pred spustením, stiahnutie relay.toml z rozhrania (spec 018) | e2e 101, unit |
| 0.53.0 | Rozpoznanie pohybu na serveri pre kamery bez ONVIF (MJPEG alebo JPEG snímka, citlivosť, zóny); oprava ukladania masiek z formulára (spec 019) | e2e 120, unit |
| 0.54.0 | Zvuk z kamier (AAC/Opus, predvolene vypnutý, oprávnenie `video.audio`, audit počúvania, samostatný prúd v prehliadači) (spec 020) | e2e 132, unit |
| 0.55.0 | Zvuk G.711 (TP-Link Tapo a väčšina kamier) ako PCM cez WebAudio; predvoľby výrobcov kamier vo formulári (Tapo, Hikvision, Dahua, Reolink, Axis, Uniview, EZVIZ) | e2e 137, unit |
| 0.56.0 | Zvuk len naživo — počúvanie bez nahrávania (šetrí disk), odhad miesta vo formulári | e2e 140 |
| 0.57.0 | Platený white labeling (39 EUR jednorazovo, navždy): trvalé licenčné kľúče overované offline (Ed25519, `licgen` predvolene bez vypršania), produkčný kľúč licencora v binárke, Systém → Licencia („navždy“), vlastný názov produktu, pätička, ikona, názov aplikácie, úvod, e-maily, 2FA, PDF; bez licencie pevná pätička ctrl32; migrácia 0039 (vetva `feature/paid-white-label`, spec 007, ADR-0016) | e2e 34, unit |
| 0.58.0 | Kamery za prenášačom: PTZ, predvoľby, udalosti ONVIF a „Vyplniť z ONVIF“ cez prenášač (tunel cez webový port HTTPS, dlhé dotazovanie, tajomstvo kamery, prenášač dosiahne len `onvif` pôvod z `relay.toml`, cesty `/onvif/`, 1 MiB, 10 s); prenášač posiela aj zvuk G.711 (Tapo); informácie o prúde (kodek, rozmer, fps, kbit/s hlavného aj vedľajšieho prúdu) na stránke kamery, prekrytie v prehrávači, dlaždice, widget; `relay.toml`: `web`, `onvif`; doplnok HA: `web`, `audio`, `onvif` (vetva `feature/relay-onvif`, URS P-27, P-28) | e2e `video` 167 (27 nových; na Windows PC 158/167, 9 zlyhaní nesúvisí — čas +02:00 v URL, `python3`), `displays` 78, `masks` 20, unit (8 nových), headless `camera_relay` |

Vetva zlúčená do `main` (PR #1, 27. 9. 2026, CI zelené). Celá sada testov (`tests/run-all.sh`) pre 0.54.0 (27. 9. 2026): všetko prešlo okrem 3 kontrol času NTP v `ops`, ktoré potrebujú internet (v tomto kontajneri nedostupný) — nesúvisí s kódom.

## Ďalšie možnosti (fáza 2)

- kamery: sťahovanie záznamov z prenášača pri výpadku spojenia;
  overiť H.264 + AAC v bežnom Chrome/Edge a inštalátory prenášača na skutočnom Linuxe/Windows
- smart home: efekty svetiel na karte, Shelly Gen1, skupiny entít, plánovanie scén (časovač),
  energia: porovnanie s minulým obdobím, export CSV
- toky: parametre inštancie podtoku, spúšťač pri zmene atribútu
- replika: prihlásenie na replike na prezeranie; Windows: test inštalátora na čistom serveri, podpis binárky

## Poznámka k prostrediu

- Vývoj na PC: `ctrl32-telemetry/deploy/dev/README.md` — `bash deploy/dev/dev.sh up` (PostgreSQL 18 v Dockeri na porte
  5433, databáza `ctrl32_dev`, server na http://127.0.0.1:8080, admin / admin1234), `camsim` (simulované kamery),
  `demo` (40 kamier, 2 steny, výroba, pôdorys). Lokálne dáta v `ctrl32-telemetry/.dev/` (mimo gitu).
- Celá sada testov: `bash tests/run-all.sh` (porty 8080, 1883, 1884 voľné; `SKIP="ops"`, keď je NTP blokované).

## Rozhodnutia

- **27. 9. 2026: white labeling je platený — 39 EUR jednorazovo, navždy** (cena zmenená z 99 EUR 27. 9. 2026 večer: rovnako ako ctrl32 editor) (trvalá licencia na server/inštaláciu,
  overovaná offline; ADR-0016). Všetko ostatné ostáva zadarmo; bez licencie je pätička ctrl32 pevná. (História:
  26. 9. pripravený, v ten istý deň vrátený „nič platené“, 27. 9. obnovený s cenou.)

## Čaká na používateľa

- **Produkcia 27. 9. 2026 22:57: telemetry.digital beží 0.58.0.** Kamery Tapo C210 („Byt“, 192.168.0.21) a C225
  (192.168.0.197) cez prenášač na PC používateľa (`%USERPROFILE%\ctrl32-relay`, naplánovaná úloha „ctrl32 camera
  relay“): obraz, nahrávanie v slučke (limit 15 GB), zvuk G.711 len naživo, PTZ cez tunel (predvoľby C210 načítané).
  Používateľ má ešte vyskúšať PTZ a zvuk v prehliadači. Prenášač presunúť na Raspberry Pi (192.168.0.53) po nahratí
  Raspberry Pi OS Lite (bez Home Assistanta; SSH alias `ctrl32-relay`, kľúč `~/.ssh/telemetry_relay_rpi`).
- white labeling (39 EUR; jedna licencia ctrl32 odomkne buď editor, alebo telemetry): aktivácia číslom objednávky
  cez `lic.ctrl32.com` (vetva `feature/license-activation` + `server/lic-worker` v repe ctrl32, rozpracované);
  súkromné kľúče licencora v `C:\Users\PavolKRNÁČ\.ctrl32-license\` zálohovať offline (nikdy nie do repozitára);
  právna kontrola výnimky v podmienke 2 `LICENSE`
- nasadenie: `deploy/publish.sh VERSION root@2.29.5.17 -i ~/.ssh/telemetry_deploy`, potom na serveri záloha,
  `migrate` novou binárkou a až potom výmena a reštart (poučenie z 27. 9.: 0016 CRLF a výpadok prihlasovania)
- potvrdenie ADR-0015 (SCADA editor v čistom JS nad SVG)
