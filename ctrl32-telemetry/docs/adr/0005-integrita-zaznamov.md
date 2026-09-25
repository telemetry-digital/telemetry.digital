# ADR-0005: Integrita záznamov — append-only, dva časy, verzie, hash-reťazec

**Stav:** rozhodnuté 23. 9. 2026 (odvodené z Annex 11, PIC/S PI 041-1, MHRA; viď `docs/00-regulacny-research/`)

## Rozhodnutie

1. **Append-only** pre všetko, čo je záznam: surové správy, observácie, audit trail, alarmový log, prístupový log, konzolový log, podpisy, reporty, FOTA stavy. DB rola aplikácie bez `UPDATE`/`DELETE`/`TRUNCATE`; BEFORE triggery (vrátane statement triggeru na TRUNCATE) blokujú aj vlastníka. Výmaz existuje len ako autorizovaná, logovaná operácia po uplynutí retencie (fáza 2+).
2. **Dva časy** na každom meraní: `measured_at` (zdroj) a `received_at` (písané databázou `now()`). Nikdy klientsky čas ako `received_at`.
3. **Verzia všetkého, čo mení význam dát:** dekodér (hash), kalibrácia, register map, limity, retencia, odvodenia. Rekalibrácia alebo opätovné dekódovanie vytvára novú `series_version`, pôvodná ostáva.
4. **Audit trail** s gapless `seq`, `prev_hash`, `hash = SHA-256(kanonický JSON ‖ prev_hash)`, `reason NOT NULL` pri zmene/výmaze GMP-relevantných dát; denný **kontrolný bod** podpísaný Ed25519 nad `(seq, head_hash)`, jeho hash v reportoch a voliteľne e-mailom; príkaz `audit verify`.
5. **Žiadny konfiguračný prepínač** na vypnutie audit trailu ani bezpečnostných kontrol.
6. **Medzery sú viditeľné**: záznam `gap` per datastream, uzatvára sa backfillom, nikdy nemizne.

## Dôsledky

- Schéma a triggery sú prvý kód fázy 1 (retrofit nemožný).
- Objemy: mesačné partície (pg_partman), BRIN; pri PLC frekvenciách deadband/agregácia v konektore je vedomé rozhodnutie zákazníka v URS.
- Bez ďalšej databázy (immudb, Timescale TSL) — všetko v PostgreSQL.
