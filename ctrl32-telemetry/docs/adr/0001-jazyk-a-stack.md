# ADR-0001: Jazyk a stack — Go pre platformu, C++ pre real-time runtime (postupne)

**Stav:** **prijaté 23. 9. 2026** (používateľ potvrdil hybrid po otázke „nebude lepšie C++?"); PoC vo fáze 0 ostáva kontrolným bodom
**Súvisí:** ADR-0008 (SCADA postupne), `docs/01-technicky-research.md`

## Kontext

- Linux aj Windows Server, jeden inštalátor, jednoduchá prevádzka („pre firmu, nie pre republiku").
- Systém môže byť vystavený do internetu; bezpečnosť, audit a logy sú priorita od začiatku.
- Jeden vývojár; systém je bezplatný, príjem z implementácie → rýchlosť dodania a nízke náklady údržby rozhodujú.
- Postupne má pribudnúť SCADA: obrazovky (HMI), riadenie, vysoké počty tagov, prípadne redundancia.
- Používateľ má skúsenosť a stratégiu s C++ SCADA jadrom („ťažké priemyselné jadro, tenká webová vrstva navrch").
- Research (23. 9. 2026): Go 1.27 (Go 1 kompatibilitný sľub, jeden statický binár pre Win/Linux, reprodukovateľný build), PostgreSQL 18, Lit 3, Typst; knižnice s jednou zjavnou voľbou pre TOTP, Argon2id, WebAuthn, MQTT broker v procese, OPC UA klient, Modbus.

## Rozhodnutie (návrh)

**Dve vrstvy, dva jazyky, jasné rozhranie medzi nimi:**

1. **Platforma `ctrl32-telemetry` = Go 1.27.** Všetko, čo je webové a dátové: HTTP/API, IAM, audit trail, PostgreSQL úložisko, alarmy, pravidlá, notifikácie, reporty, exporty, konektory (MQTT broker v procese, HTTP, LoRaWAN, MQTT most, OPC UA *klient*, Modbus s cyklami v sekundách), FOTA, konzola, prehliadač správ, HMI viewer/editor (frontend TypeScript/Lit 3). Jeden statický binár.
2. **Runtime `ctrl32-runtime` = C++20, samostatný voliteľný proces, od fázy 3.** Real-time zber a riadenie tam, kde Go prestane byť najlepšia voľba: OPC UA **server aj klient** cez open62541 (MPL-2.0, referenčný stack s PubSub), S7/PROFINET, Modbus s milisekundovými cyklami, deterministické skenovacie cykly, lokálna riadiaca logika, **redundancia** (hot standby), edge nasadenie (priemyselné PC, ARM). S platformou komunikuje cez MQTT (Sparkplug-B-like) alebo gRPC; platforma bez neho funguje naplno pre telemetriu.

Frontend: TypeScript 7 + Lit 3 + Vite 8, grafy uPlot, HMI v SVG/Web Components (viď rozhodnutia pre HMI editor). DB: PostgreSQL 18 + pg_partman. PDF: Typst 0.15.

## Prečo nie všetko v C++

- **Bezpečnosť na internetovej hrane.** Webový server, parsovanie JSON/MQTT payloadov od neznámych zariadení, autentifikácia, relácie — to je presne miesto, kde pamäťovo nebezpečný jazyk u jedného vývojára bez bezpečnostného tímu vytvára triedu chýb (buffer overflow, use-after-free), ktoré Go nemá. Systém, za ktorý „nasadzovateľ preberá zodpovednosť", musí byť bezpečný predvolene; jazyk je prvá predvoľba.
- **Rýchlosť dodania.** ~80 % funkcií je CRUD, API, RBAC, audit, reporty, integrácie. V Go je to násobne rýchlejšie ako v C++ (knižnice, testovanie, chýbajúce kompilačné časy, žiadny CMake/vcpkg/Conan ekosystém na údržbu).
- **Reprodukovateľné buildy a validovaný stav.** Go toolchain je reprodukovateľný od 1.21; SBOM a `govulncheck` sú vstavané do procesu. V C++ je správa závislostí a cross-compile Win/Linux samostatná práca.
- **Výkon platformy nie je úzke hrdlo.** Firma s 10 000 datastreamami produkuje desiatky správ za sekundu; Go zvládne rádovo 10⁵ správ/s na jednom jadre. GC pauzy pod 1 ms sú pre SCADA cyklus 100 ms irelevantné.

## Prečo C++ tam, kde je

- **open62541** je najlepší otvorený OPC UA stack (server + klient + PubSub, MPL-2.0); Go má len klienta (`gopcua`), OPC UA server v Go nie je vyzretý (⚠️ overiť aktuálny stav `gopcua/server`).
- **Determinizmus a latencia** pri tisícoch tagov s milisekundovými cyklami, lokálna riadiaca logika, redundancia s prepínaním v stovkách ms — doména C++ (WinCC OA, zenon sú C++ jadrá).
- **Kontinuita s tvojou stratégiou:** C++ jadro ostáva, len sa **nestavia prvé** — stavia sa vtedy, keď ho implementácie potrebujú, a pripája sa na hotovú platformu cez jasné rozhranie.

## Kedy vzniká C++ runtime (spúšťače)

Prvá implementácia, ktorá potrebuje aspoň jedno z: OPC UA server; > ~20 000 tagov alebo cykly < 100 ms; S7/PROFINET; redundancia; edge box bez platformy. Dovtedy stačí Go platforma s konektormi.

## Alternatívy

| Alternatíva | Prečo nie (teraz) |
|---|---|
| Všetko v Go | najjednoduchšie; naráža až pri vysokom SCADA konci (OPC UA server, determinizmus, redundancia) — preto hybrid |
| Všetko v C++ | najvyššie bezpečnostné riziko na internetovej hrane, najpomalší vývoj pre jedného; C++ je správny nástroj pre runtime, nie pre webovú platformu |
| .NET 10 LTS | dobrý na Windows; LTS každé 2 roky, natívne knižnice sa rozbaľujú na disk, QuestPDF licencia; 8/10 |
| Rust ako runtime namiesto C++ | bezpečnostne lepší, ale open62541 je C a tvoja expertíza je C++; možno prehodnotiť pri štarte fázy 3 |
| Node/TypeScript backend | dependency churn nezlučiteľný s validovaným stavom (skúsenosť z predchádzajúcich projektov) |

## Dôsledky

- Dva repozitáre a dve jazykové sady, ale nie súčasne — runtime začína až vo fáze 3.
- Rozhranie platforma ↔ runtime (MQTT/gRPC schéma) sa navrhne už vo fáze 1 spolu s protokolom zariadení, aby runtime nebol retrofit.
- PoC vo fáze 0 potvrdí Go (end-to-end tok na reálnom hardvéri, Windows aj Linux ako služba).
- Ak sa potvrdenie nedostaví alebo PoC ukáže neprijateľnú produktivitu, náhradník je .NET 10 LTS, nie C++.
