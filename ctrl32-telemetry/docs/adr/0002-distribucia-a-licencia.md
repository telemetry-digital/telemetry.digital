# ADR-0002: Distribúcia a licencia — bezplatné binárky, zdrojový kód po verzii 1.0

**Stav:** rozhodnuté 23. 9. 2026 (používateľ: „stále zdarma, zdrojáky potom, keď to bude 100 % funkčné"); **doplnené 23. 9. 2026 popoludní** (používateľ: „odkaz na licenciu Apache so zákazom ďalšieho predaja, iba zadarmo"; „footer bude nemenný a nebude sa dať nijako zmeniť")

> **Zmenené 27. 9. 2026 (ADR-0016):** white labeling je platený — **39 EUR jednorazovo, navždy** (trvalá licencia na server, overovaná offline). Všetko ostatné ostáva zadarmo; bez licencie platí pevná pätička ctrl32 (ADR-0011) a podmienka 2 v `LICENSE` má výnimku len pre nasadenie s platnou white-label licenciou (návrh na právnu kontrolu). História: 26. 9. 2026 bol platený white labeling pripravený, v ten istý deň na žiadosť vrátený („zostaneme pri free verzii, nič platené“) a 27. 9. 2026 obnovený s cenou.

## Doplnenie: konkrétna licencia (23. 9. 2026)

- Súbor `LICENSE` v koreni repozitára = **Apache License 2.0 + dodatočné podmienky**: (1) zákaz predaja softvéru, jeho kópií, upravených verzií a plateného hostovania (SaaS), pričom vlastné implementačné, integračné, školiace a podporné služby dovolené sú; (2) pätička produktu („ctrl32 · telemetry", GitHub, licencia) musí ostať viditeľná a nezmenená; (3) upozornenie, že pre podmienku (1) nejde o OSI open-source licenciu. Ide o vzor „Apache-2.0 + Commons Clause", zúžený tak, aby nezakazoval služby tretích strán.
- Text licencie je vložený do binárky (`embed.go`) a zobrazený na `/license` (HTML aj `?format=txt`); odkaz je v pevnej pätičke každej stránky.
- **Potvrdené používateľom 23. 9. 2026:** zúžené znenie podmienky (1) je v poriadku — zakázaný je predaj a platené hostovanie softvéru, platené implementačné a servisné služby tretích strán sú dovolené. Právna kontrola presného textu ostáva pred vydaním zdrojových kódov (1.0).

## Rozhodnutie

1. Systém je **bezplatný a navždy bezplatný** — server, inštalátory, knižnice zariadení, dokumentácia. Jedinou platenou voliteľnou funkciou je white labeling (39 EUR jednorazovo, navždy; ADR-0016, doplnené 27. 9. 2026).
2. **Do verzie 1.0** sa distribuujú podpísané binárky pod bezplatnou licenciou (EULA) s týmito vlastnosťami: bez poplatku, bez záruky („AS IS"), bez obmedzenia počtu zariadení/používateľov, zákaz odstraňovania podpisov a bezpečnostných mechanizmov, **zodpovednosť za nasadenie a prevádzku nesie nasadzovateľ**, ochranná známka „ctrl32" a „ctrl32 telemetry" ostáva vývojárovi. Text EULA je návrh na právnu kontrolu (`LICENSE-NOTICE.md`, doplní sa).
3. **Po 1.0 („100 % funkčné")** sa zverejní zdrojový kód. Odporúčaná licencia **Apache-2.0** (patentová ochrana, vylúčenie záruky, bez copyleftu — knižnice zariadení sa dajú vložiť aj do uzavretého firmvéru). Alternatíva AGPL-3.0 pre server, ak bude treba brániť uzavretým SaaS forkom.
4. Knižnice zariadení (`ctrl32-telemetry-c`) môžu byť zverejnené skôr ako server (sú malé, potrebujú ich implementátori a ctrl32 editor) — rozhodne sa pri vydaní 1.0 knižníc.

## Dôsledky

- Pre GxP implementácie do zverejnenia zdrojákov: ponuka **escrow** zdrojového kódu ako služba implementácie (exit strategy podľa draft Annex 11 7.5 viii); po zverejnení odpadá.
- Release proces od začiatku počíta so zverejnením: žiadne tajomstvá v repozitári, čisté licencie závislostí (SBOM), história commitov publikovateľná.
- Verejná demo inštancia (ADR-0003) môže bežať už pred 1.0.
