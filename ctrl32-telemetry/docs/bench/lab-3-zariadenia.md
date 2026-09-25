# Skúšobný stôl: 3 zariadenia, 2× MQTT + 2× OPC UA, priamo po sieti

Stav: 24. 9. 2026, ctrl32 telemetry 0.12.x. Cieľ: na stole mať tri fyzické zariadenia, ktoré posielajú
skutočné údaje do ctrl32 telemetry bez WireGuardu — MQTT zariadenia priamo cez internet (TLS), OPC UA servery
po lokálnej sieti.

## 1. Čo dnes systém vie a čo ešte dorobíme

| Cesta | Stav | Poznámka |
|---|---|---|
| MQTT zariadenie → server (TLS 8883, token) | hotové | knižnica `ctrl32-telemetry-c` (Arduino/ESP-IDF), príkazy s ack, zdieľané atribúty, konzola, FOTA |
| MQTT zariadenie → server (plain 1883) | hotové | len v intranete alebo lokálnej inštancii; na demo je 1883 viazaný na localhost |
| OPC UA server → ctrl32 (klient v serveri, polling) | **dorábame (0.13.0)** | konektor `opcua`: endpoint, bezpečnosť, uzly → kľúče datastreamov, interval |
| Modbus TCP → ctrl32 | plán (0.14.0) | konektor `modbus_tcp` |

Dôležité pre OPC UA: ctrl32 je *klient*, OPC UA zariadenie je *server*. Klient sa musí k serveru dostať po
sieti. Zariadenia na stole sú za NAT-om, takže **demo v cloude (telemetry.digital) sa k nim nedostane**. Bez
WireGuardu sú dve možnosti:

1. **Lokálna inštancia ctrl32 na stole** (odporúčané): mini PC / Raspberry Pi 4–5 / notebook s Linuxom, profil
   `intranet`, inštalácia jedným príkazom. OPC UA konektor beží v nej a číta servery v tej istej LAN. MQTT
   zariadenia môžu ísť do nej (1883, bez TLS) alebo rovno do cloudu (8883, TLS).
2. Port-forward OPC UA (4840) z routera na verejnú IP a čítanie z cloudu — technicky ide, ale OPC UA server je
   potom vystavený do internetu; neodporúčam ani na skúšku.

Návrh: **MQTT zariadenia → cloud demo** (skúška reálnej cesty cez internet), **OPC UA → lokálna inštancia** na
stole. Obe inštancie vidíš v tom istom UI (rôzne adresy), export a reporty fungujú rovnako.

## 2. Zostava zariadení

Interpretujem „3 zariadenia, dve MQTT a dve OPC“ takto (ak je to inak, prispôsobíme):

| Ozn. | Zariadenie | Cesta | Meria / robí |
|---|---|---|---|
| A | ESP32-S3-DevKitC-1 + SHT40 (I²C) | MQTT/TLS → telemetry.digital:8883 | teplota, vlhkosť; každých 30 s; buffer pri výpadku |
| B | ESP32-S3-DevKitC-1 + DS18B20 + relé modul | MQTT/TLS → telemetry.digital:8883 | teplota sondy; relé ovládané príkazom (`relay.set`), ack |
| C | OPC UA server 1: PLC (napr. Siemens S7-1200 FW ≥ 4.4 s OPC UA, S7-1500, Beckhoff CX, WAGO PFC200, Unitronics) | OPC UA ← lokálna inštancia | 2–5 tagov: teplota, tlak/stav, počítadlo, boolean |
| C′ | OPC UA server 2: simulátor na PC/Raspberry Pi (Prosys OPC UA Simulation Server – zadarmo, alebo `open62541` ukážkový server, alebo Node-OPCUA) | OPC UA ← lokálna inštancia | simulované signály (sínus, rampa, náhodné) — overí konektor aj bez PLC |

Ak máš len jeden OPC UA server, C′ vynecháš; ak žiadny, simulátor stačí na celý test konektora.

## 3. Nákupný / prípravný zoznam

Hardvér
- 2× ESP32-S3-DevKitC-1 (N8R2 alebo N16R8), 2× USB-C dátový kábel, 2× napájací adaptér 5 V/2 A (alebo USB hub).
- 1× SHT40 (alebo SHT31) modul I²C; 1× DS18B20 sonda (vodotesná) + rezistor 4,7 kΩ; 1× relé modul 1 kanál 3,3 V logika (alebo SSR).
- Breadboard, prepojky, prípadne krabička.
- OPC UA server: PLC podľa tabuľky alebo PC/Raspberry Pi so simulátorom.
- Lokálna inštancia: Raspberry Pi 4/5 (4 GB, microSD 32 GB, napájanie) alebo mini PC/notebook s Ubuntu 24.04 (Docker pre PostgreSQL a Caddy inštalátor nainštaluje sám).
- Sieť: Wi‑Fi **2,4 GHz** SSID pre ESP32 (5 GHz nepodporuje), Ethernet switch pre PLC/Pi, prístup na internet z Wi‑Fi (pre cestu do cloudu).

Údaje, ktoré treba mať po ruke (vyplň pred montážou)
- Wi‑Fi SSID a heslo (2,4 GHz, WPA2).
- IP adresy: PLC, simulátor, lokálna inštancia (ideálne pevné/DHCP rezervácie).
- OPC UA: endpoint URL (`opc.tcp://192.168.x.y:4840/...`), bezpečnostná politika (na skúšku `None`, potom
  `Basic256Sha256` + Sign&Encrypt), spôsob prihlásenia (anonymne / meno+heslo), zoznam uzlov (NodeId, napr.
  `ns=3;s="DB1"."Temp"` alebo `ns=2;i=1001`), typ hodnoty (Float, Int16, Boolean…), jednotka, žiadaný interval.
- Pre každý signál: kľúč datastreamu (napr. `t_c`, `rh`, `p_bar`, `run`), veličina, jednotka, medze pre alarm.

## 4. Príprava v ctrl32 (cloud aj lokálna inštancia rovnako)

1. **Site** „Stôl“ → **Assets**: „Chladnička A“ (A), „Sonda B“ (B), „Linka C“ (OPC UA).
2. **Datastreams** pod assetmi: kľúč, veličina, jednotka, očakávaný interval (pre detekciu výpadkov), medze
   (warning/action) — alarmy sa dajú doplniť neskôr.
3. **Devices → New device** pre A a B: vznikne token `c32d_…` (zobrazí sa raz). Priradiť device → datastreamy
   (kľúče, ktoré zariadenie publikuje: `t_c`, `rh` pre A; `t_c` pre B).
4. **Settings → Connectors → New connector** typu `opcua` (od 0.13.0): endpoint, bezpečnosť, prihlásenie,
   interval, mapovanie uzlov → datastreamy. Jeden konektor na jeden OPC UA server (C a C′ = dva konektory).
5. Lokálna inštancia: `curl -fsSL https://telemetry.digital/install.sh | sudo bash -s --` (bez `--domain` =
   profil intranet, HTTP :8080, MQTT :1883). Prvé heslo admina je v `/etc/ctrl32-telemetry/admin-bootstrap.txt`.

## 5. Firmvér ESP32 (A, B)

- Repo `ctrl32-telemetry-c`, port `ports/arduino`, príklad `examples/arduino/fridge_node` (PlatformIO alebo
  Arduino IDE 2.x, board `esp32-s3-devkitc-1`, knižnica PubSubClient).
- Konfigurácia v kóde: `host = "telemetry.digital"`, `port = 8883`, `tls = true`, `device_id` = external id z UI,
  `token` = `c32d_…`, `ca` = Let's Encrypt ISRG Root X1 (PEM je v príklade), `publish_every = 30 s`.
- Zariadenie B navyše: príkaz `relay.set {"on": true|false}` → knižnica zavolá callback, ten prepne GPIO a pošle
  ack; v UI Devices → B → Commands → Send command.
- Lokálna inštancia namiesto cloudu: `host = "192.168.x.y"`, `port = 1883`, `tls = false`.

## 6. OPC UA server (C, C′)

- PLC: zapnúť OPC UA server, povoliť anonymný prístup alebo vytvoriť používateľa, exportovať (alebo opísať)
  NodeId sledovaných tagov; bezpečnosť na skúšku `None`, potom certifikáty (klient ctrl32 si vygeneruje vlastný
  certifikát, na PLC ho treba raz dôverovať).
- Simulátor (Prosys Simulation Server): po štarte ukazuje endpoint `opc.tcp://<pc>:53530/OPCUA/SimulationServer`
  a uzly `ns=3;s=Sinusoid`, `ns=3;s=Random`, `ns=3;s=Counter` — ideálne na prvý test konektora.
- Firewall na PC so simulátorom: povoliť TCP port servera pre LAN.

## 7. Akceptačné kroky (v tomto poradí)

1. A a B: v Devices sú **online**, v Message viewer prichádzajú telemetry správy, datastreamy majú hodnoty; po
   odpojení Wi‑Fi na 2 minúty príde buffer (backfill) a detekcia výpadku sa uzavrie.
2. B: `relay.set` z UI → relé cvakne → v Commands je stav `acked`.
3. Alarm: nastaviť warning limit pod aktuálnu teplotu → alarm sa spustí, e-mail (ak je SMTP), zobrazí sa v Alarms.
4. C, C′: konektor v stave „connected“, hodnoty uzlov sa objavujú každý interval; zmena hodnoty v PLC/simulátore
   sa prejaví do jedného intervalu; po vypnutí simulátora konektor hlási chybu a po zapnutí sa sám pripojí.
5. Export CSV/JSON Lines a PDF report za posledných 6 hodín z oboch inštancií; audit ukazuje kto čo nastavil.

## 8. Otvorené otázky pre teba

- Máš PLC s OPC UA serverom, alebo ideme na simulátor + PLC neskôr?
- Ktoré signály (aspoň názvy a jednotky) chceš z OPC UA — stačí 3–5 na začiatok.
- Lokálna inštancia: Raspberry Pi, mini PC alebo notebook? (Windows inštalátor príde až po šablónach a blokoch,
  zatiaľ Linux jedným príkazom.)
