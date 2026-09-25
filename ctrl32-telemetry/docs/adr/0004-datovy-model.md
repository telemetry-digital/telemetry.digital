# ADR-0004: Dátový model — univerzálny, názvoslovím podľa OGC SensorThings

**Stav:** rozhodnuté 23. 9. 2026

## Rozhodnutie

Hierarchia: **Organization → Customer (voliteľne) → Site → Asset → Datastream**, s **Device** (to, čo komunikuje) a **Sensor** (fyzický snímač s kalibráciami) ako samostatnými entitami priradenými k datastreamom s platnosťou od–do.

- `Datastream.kind ∈ {gauge, counter, state, event}` určuje spracovanie (počítadlá → odvodená spotreba, stavy/udalosti → `value_json`).
- Atribúty zariadenia ako v ThingsBoard: *client*, *server*, *shared* (konfigurácia doručovaná zariadeniu).
- Aplikačné balíky (GxP teploty, Energie, …) = hodnoty v tomto modeli (typy assetov, atribúty, retencie, šablóny alarmov a reportov, matice zhody), nie nové tabuľky ani kód.
- Názvoslovie podľa OGC SensorThings (Thing/Datastream/Observation/Sensor), aby bol model zrozumiteľný mimo farmácie a dal sa neskôr vystaviť ako štandardné API.

## Dôsledky

- Univerzálnosť je v schéme a API od fázy 0; funkcie nad ňou prichádzajú po vrstvách.
- SCADA tagy (ADR-0008) sú datastreamy s rýchlejším cyklom a zápisovou cestou — ten istý model.
