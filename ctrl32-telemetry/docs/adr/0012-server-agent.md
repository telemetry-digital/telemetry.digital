# ADR-0012: Správa servera z UI cez privilegovaného agenta

**Stav:** prijaté 24. 9. 2026
**Kontext:** Používateľ chce, aby si nasadzovateľ nastavil všetko v našom UI, nie v príkazovom riadku: parametre servera prehľadne roztriedené, reštart, Caddy (doména, TLS, Cloudflare), aktualizácie a WireGuard server pre pripájanie zariadení. Služba `ctrl32-telemetry` beží zámerne bez privilégií (používateľ `ctrl32`, `ProtectSystem=strict`, `NoNewPrivileges`), konfigurácia je root-only 0640 a Caddy beží v Dockeri. Priame vykonávanie `systemctl`, `wg` alebo zápis do `/opt` z webovej služby by znamenalo dať jej root, čo je v rozpore s ADR-0007 (bezpečnostná základňa) a modelom hrozieb.

## Rozhodnutie

1. **Druhý proces, rovnaký binár.** `ctrl32-telemetry agent` beží ako root v samostatnej systemd jednotke `ctrl32-telemetry-agent.service` a počúva na Unix sockete `/run/ctrl32-telemetry/agent.sock` (0660 `root:ctrl32`). Webová služba je jediný klient (`[agent] socket` v konfigurácii). Bez agenta (Windows, vývoj) obrazovka servera ukazuje len informácie o procese a stav „agent nepripojený“.
2. **Uzavretý zoznam operácií, žiadne ľubovoľné príkazy.** Agent vystavuje HTTP/JSON cez socket len tieto cesty: `GET /status` (host, služby, kontajnery, verzie, certifikát, WireGuard), `POST /restart {unit}` (`app`, `agent`, `caddy`, `wireguard`), `PUT /caddy` (doména, ďalšie domény, e-mail pre ACME, voliteľne Cloudflare DNS-01 token), `GET/PUT /wireguard` (zapnuté, port, adresa, endpoint), `POST/DELETE /wireguard/peers` (názov → kľúčový pár + predzdieľaný kľúč, konfigurácia klienta vrátená raz), `POST /update {version}` (stiahnutie z `/dl/`, overenie SHA-256, inštalácia, migrácia, reštart). Každý vstup agent validuje sám (regex domény, e-mail, port, CIDR, verzia); nič z UI sa nevkladá do shellu — všetko sú argumenty `exec` bez shellu.
3. **Audit na dvoch miestach.** Webová služba zapisuje každú operáciu do audit trailu (`server.restart`, `server.caddy`, `server.wireguard*`, `server.update`) s dôvodom a používateľom; agent loguje do journalu, čo skutočne vykonal. Oprávnenie v UI: `system.admin`.
4. **Caddy zostáva v Dockeri.** Agent generuje `/opt/ctrl32-telemetry/Caddyfile` zo šablóny, tajomstvá (Cloudflare token) do `/opt/ctrl32-telemetry/caddy.env` (0600), overí `caddy validate` v kontajneri a urobí `caddy reload`. Pre DNS-01 cez Cloudflare sa použije obraz s modulom `caddy-dns/cloudflare` zostavený lokálne z `caddy:2-builder` (bez obrazov tretích strán). Pri zmene domény agent prepíše `base_url` v `config.toml` a reštartuje službu.
5. **WireGuard cez `wireguard-tools`.** Inštalátor nainštaluje balík (best effort), agent spravuje `/etc/wireguard/wg0.conf` a `/etc/ctrl32-telemetry/wireguard/peers.json`, zapína/vypína `wg-quick@wg0`, mení peerov cez `wg syncconf` bez výpadku. Zariadenia dostanú konfiguráciu (text + QR v UI) s vlastným kľúčom a pridelenou IP; súkromný kľúč peera server neukladá.
6. **Aktualizácie z nášho distribučného bodu.** `publish.sh` zverejňuje `SHA256SUMS`; agent stiahne binárku, overí súčet, nainštaluje, spustí `migrate` a reštartuje službu aj seba. Podpis binárok (ADR-0002, certifikát ctrl32) príde s Windows inštalátorom.

## Dôsledky

- Root beží len v malom, samostatne auditovateľnom procese s pevným zoznamom operácií; webová služba zostáva hardened.
- Jeden binár, jedna konfigurácia; inštalátor pridá jednu jednotku a jeden socket. Windows: agent zatiaľ nie je (obrazovka len na čítanie).
- Chyby v agentovi majú vysoký dopad → operácie sú malé, deterministické, s validáciou a s „dry-run“ (`caddy validate`, `wg` syntax) pred aplikovaním.
- Zvážené a zamietnuté: `sudoers` pravidlá (ťažko auditovateľné, shell), D-Bus/polkit pre systemd (nerieši Caddy ani WireGuard), root pre celú službu (proti ADR-0007).
