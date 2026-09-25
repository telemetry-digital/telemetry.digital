# ADR-0006: Zariadenia — jeden protokol, C jadro + porty, ctrl32 ako no-code cesta

**Stav:** rozhodnuté 23. 9. 2026

## Rozhodnutie

- **Hardvér je ľubovoľný.** Vývojár nedodáva hardvér, kity ani kalibrácie.
- **Jeden zdokumentovaný, verziovaný protokol** (`docs/protocol/device-protocol-v0.1.md`): MQTT 3.1.1/5 cez TLS alebo HTTPS; token alebo klientsky certifikát per zariadenie; provisioning claim kódom; JSON (predvolene) alebo CBOR (obmedzené zariadenia); telemetria s dávkami a backfillom, atribúty, príkazy s ack, konzola, FOTA.
- **Knižnice:** spoločné jadro `ctrl32-telemetry-c` (C99, bez dynamickej alokácie, platformové rozhrania) + porty:
  - **Arduino** (ESP32, ESP8266, RP2040 W, Ethernet dosky),
  - **ESP-IDF** (komponent; esp-mqtt, esp-tls, NVS),
  - **STM32** — **obidve cesty:** (a) FreeRTOS + LwIP (Ethernet) + mbedTLS + coreMQTT; (b) modem cez AT príkazy (LTE-M/NB-IoT/2G moduly Quectel, SIMCom, u-blox) s TLS v modeme alebo v mbedTLS; referenčný **bootloader pre FOTA** (dva sloty, overenie podpisu, rollback),
  - **ctrl32 editor**: používateľ programuje čipy a zariadenia v ctrl32 bez Arduina; knižnica je vstavaný komponent generovaného firmvéru; **provisioning** zariadení priamo z editora cez provisioning API platformy (vytvorenie zariadenia, token/claim, stav).
- Knižnice budú licencované permisívne (Apache-2.0 pri zverejnení), aby sa dali vložiť aj do uzavretého firmvéru.
- Cudzie zariadenia (LoRaWAN senzory, PLC, merače) sa pripájajú cez konektory platformy, nie cez knižnicu.

## Dôsledky

- Testy protokolu bežia v CI proti serveru pre všetky porty (emulované zariadenie).
- FOTA klient, konzola a príkazy sú súčasť jadra knižnice, nie portu.
- ctrl32 editor potrebuje stabilné provisioning API od verzie 1.0 platformy.
