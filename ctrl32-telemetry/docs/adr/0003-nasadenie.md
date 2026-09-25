# ADR-0003: Nasadenie — výhradne u nasadzovateľa, dva profily, verejná demo inštancia

**Stav:** rozhodnuté 23. 9. 2026

## Rozhodnutie

- Systém sa nasadzuje **vždy u toho, kto ho nasadzuje**: intranet firmy (Windows Server alebo Linux) alebo verejne na webe (VPS, on-prem za reverzným proxy). Vývojár neprevádzkuje SaaS.
- Jeden inštalátor, dva profily: `intranet` (TLS s vlastným certifikátom, MFA pre správcov) a `internet` (ACME alebo certifikát, **MFA pre všetkých**, limity pokusov, prísne hlavičky, MQTT len TLS + autentifikácia). Profil `internet` nedovolí spustiť HTTP ani MQTT bez TLS.
- Jedna inštalácia = jedna **organizácia**; organizácia môže mať **zákazníkov** (podorganizácie so scopovanými rolami), aby implementátor spravoval viac firiem na jednej inštancii. Bez samoobslužnej registrácie.
- Dimenzovanie na firmu: jeden server, do ~10 000 datastreamov, bez clustra; vyššie objemy = silnejší server a PostgreSQL replika (návod), nie iná architektúra.
- Aktualizácie nikdy automaticky: podpísaný release pack, záloha pred migráciou, kontrola integrity.
- **Zodpovednosť nasadzovateľa** je súčasť dokumentácie a licencie: patchovanie OS/DB, zálohy mimo servera, sieť a firewall, prístup z internetu, kalibrácie meradiel, validácia v zamýšľanom použití.
- **Verejná demo inštancia** (rozhodnuté: áno): ukážkové dáta, denný reset, bez reálnych zákazníkov; slúži implementácii a testovaniu profilu `internet` na vlastnej koži.

## Dôsledky

- Bezpečné predvoľby sú povinnosť produktu (ADR-0007), lebo nasadenia nekontrolujeme.
- Systém upozorňuje v UI na zastaranú verziu (lokálna kontrola voči podpísanému zoznamu verzií, bez odosielania dát von).
- Windows prevádzky bez Linuxu: LoRaWAN cez gateway so vstavaným network serverom (ChirpStack je Linux/Docker).
