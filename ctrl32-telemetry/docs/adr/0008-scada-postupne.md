# ADR-0008: SCADA postupne — HMI obrazovky, riadenie, real-time runtime

**Stav:** **prijaté 23. 9. 2026** (používateľ: „malo by to mať aj SCADU postupne"; potvrdené spolu s ADR-0001)

## Kontext

Platforma je od začiatku historian + alarmy + príkazy do zariadení. SCADA k tomu pridáva: procesné obrazovky (mimiky) v reálnom čase, zápis/riadenie s oprávneniami a auditom, vysoké počty tagov a rýchle cykly, prípadne redundanciu. Skoršie rozhodnutia používateľa pre webový HMI editor (25. 8. 2026) platia: **JSON formát obrazoviek ako trvalé aktívum**, rendering SVG, widgety ako Web Components (Lit), dáta cez MQTT-over-WebSocket, knižnica podľa ISA-101 (šedé podklady, farba pre abnormálne stavy).

## Rozhodnutie

SCADA vrstvy sa pridávajú v tomto poradí, každá na hotovom predchádzajúcom stupni:

| Stupeň | Fáza | Čo pribudne |
|---|---|---|
| **S1 — Historian + alarmy + príkazy** | 1 | už v jadre: datastreamy, observácie, alarmy s eskaláciou, príkazy s ack, shared atribúty |
| **S2 — HMI viewer** | 2 | **JSON formát obrazoviek v1** (strom widgetov + geometria + väzby na datastreamy + akcie), runtime viewer v Lit/SVG, živé hodnoty cez MQTT-over-WS, základná ISA-101 knižnica widgetov (hodnota, stav, trend, alarm, tlačidlo, vstup), zápis hodnôt = príkaz cez platformu s oprávnením a auditom |
| **S3 — HMI editor + riadenie** | 3 | webový editor nad formátom v1 (engine s čistou licenciou: Konva/Fabric.js/maxGraph — tldraw vylúčený), zápis do OPC UA/Modbus cez konektory, setpointy so štyrmi očami a dôvodom (GxP), receptúry/limity ako verziovaná konfigurácia, mapa medzier a kvalita signálu v obrazovke |
| **S4 — Real-time runtime** | 3+ | `ctrl32-runtime` (C++, ADR-0001): OPC UA server + klient (open62541), S7, Modbus s ms cyklami, deterministické skenovanie, lokálna logika, **redundancia** hot standby, edge; platforma je nad ním „tenká webová vrstva" |
| **S5 — Rozšírenia** | podľa dopytu | trendové analýzy, batch/šaržové záznamy, receptúrne riadenie, viacserverová architektúra (PG replika + runtime pár) |

Zásady: obrazovky sú **dáta v DB s verziami a auditom** (zmena obrazovky = konfiguračná zmena); riadiaci zápis je vždy príkaz s identitou, časom, ack a dôvodom; **platforma nikdy nie je v riadiacej slučke** — bezpečnostné blokády ostávajú v PLC/runtime.

## Dôsledky

- Formát obrazoviek sa navrhne vo fáze 1 (aj bez viewera), aby naň mohli odkazovať alarmy a pravidlá.
- Rozhranie platforma ↔ runtime (MQTT/gRPC schéma tagov, kvalít, časov, príkazov) sa definuje spolu s protokolom zariadení.
- Hybrid Go + C++ (ADR-0001) je dôsledok tejto postupnosti: webová a dátová vrstva teraz v Go, real-time jadro v C++ vtedy, keď ho implementácia potrebuje.
- **Stav 24. 9. 2026 (0.16.0):** formát obrazoviek v1 je nasadený ako *dashboardy* (`kind: "dashboard"`, `layout.type: "grid"`, 12 stĺpcov, widgety s geometriou, väzbami na datastreamy/zariadenia a parametrami; verzie s dôvodom v `core.dashboard`). Mimiky (S2/S3) použijú ten istý formát s `layout.type: "canvas"` (absolútne súradnice, SVG symboly, ISA-101), živé hodnoty cez MQTT-over-WS a akcie zostávajú fázou 2–3.
