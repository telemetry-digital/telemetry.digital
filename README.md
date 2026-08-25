# telemetry.digital

Privátny repozitár projektu **telemetry.digital**, postaveného na zakúpenej distribúcii **legacy HMI Pro v1.3.4-2890** (frangoteam).

legacy HMI je webová SCADA/HMI/dashboard aplikácia — Node.js backend, Angular frontend, vstavaný Node-RED, historizácia do SQLite/InfluxDB/TDengine/QuestDB a drivery pre Modbus, S7, OPC UA, BACnet, EtherNet/IP, MQTT, ODBC a ďalšie.

## Štruktúra

```
legacy HMI_inst/v1.3.4-2890/
  server/          Node.js backend (spúšťa sa odtiaľto)
  server/test/     mocha testy – doplnené z upstreamu, v Pro balíku chýbali
  client/dist/     skompilovaný Angular frontend (zdrojáky Pro verzie nie sú súčasťou dodávky)
  docs/            dokumentácia – doplnená z upstreamu
  node-red/        node-red-contrib-legacy HMI (19 uzlov)
  app/electron/    balenie do desktopovej aplikácie
  odbc/            inštalácia ODBC driverov pre Docker
PRO-DELTA.diff     presný rozdiel Pro verzie oproti open-source legacy HMI
```

## Spustenie

```bash
cd legacy HMI_inst/v1.3.4-2890/server
npm install
npm start          # http://localhost:1881
```

Odporúčaný runtime je Node.js 18.20.4 (testovaný baseline, funkčné až po Node 22). `sqlite3` a `serialport` sú natívne moduly — na Windows treba build nástroje. Komunikácia so Siemens S7 vyžaduje `npm install node-snap7` navyše, v `package.json` zámerne nie je.

Testy: `npm test` v priečinku `server`.

## Branding (white-label)

Pro verzia sa prebrandováva **bez rebuildu frontendu** — stačí upraviť jeden JSON:

```
legacy HMI_inst/v1.3.4-2890/client/dist/assets/customization/config.json
```

Kľúče: `appName`, `appTitle`, `favicon`, `serverName`, `appInfo`, `sideMenuTitle`, `logFileName`, `reportHeaderText`, `splash`. V tom istom priečinku sú `favicon.ico` a `logo.svg` na výmenu. Server súbor načíta pri štarte cez `getCustomization()` v `server/main.js`.

## Vzťah k open-source legacy HMI

Upstream: https://github.com/frangoteam/legacy HMI — commit `3616aab43ac80f7f67d4609a1283c820a68b3b82`, verzia `1.3.4-2890` (rovnaká ako táto Pro distribúcia).

Vetva **`vendor-upstream`** obsahuje čistý snapshot open-source verzie. Vetva `main` je z nej odvodená, takže:

```bash
git diff vendor-upstream main -- legacy HMI_inst/v1.3.4-2890/server \
  ':(exclude)*/server/_images' ':(exclude)*/server/_widgets'
```

Výsledok: 29 súborov, +1035 / −83 riadkov — presne to, čo Pro pridáva k serveru. (Bez zúženia na `server/` diff navyše ukáže `client/src` ako zmazaný, keďže Pro dodávka zdrojáky frontendu neobsahuje.)

Rovnaký obsah je pre pohodlie aj v súbore `PRO-DELTA.diff` (29 súborov, ~1 900 riadkov).

**Čo Pro pridáva** (~850 riadkov, ~3 % serverového kódu, čisto aditívne):

- `server/runtime/events_history/` + `server/api/events_history/` — audit log: login, logout, zápis hodnoty, štart/stop zariadenia, skript, alarm, notifikácia, report, systém, pripojenie a odpojenie klienta
- `server/runtime/resources/` — správa zdrojov (adresár `_resources`)
- white-label branding cez `getCustomization()`
- nastavenie `hideNodeRedOptions`
- rozšírené knižnice symbolov v `server/_images` a `server/_widgets`
- licencia MIT → Customized MIT

**Čo open-source má a Pro dodávka nie:** zdrojáky Angular frontendu (`client/src`, 915 súborov, Angular 18.2.14), testy a dokumentáciu. Posledné dve sú v tomto repozitári doplnené.

> **Pozor:** frontend sa nedá rebuildovať z upstream zdrojov bez straty Pro funkcií — v skompilovanom `client/dist` sú obrazovky pre events history a resources, ktoré v open-source zdrojákoch neexistujú.

> **Pozor:** socket.io príkaz na zápis hodnoty používa v Pro verzii `message.variable`, kým upstream `message.var`. Externí WebSocket klienti písaní podľa upstream dokumentácie zápis nevykonajú.

## Aktualizácia na novšiu verziu legacy HMI

1. `git checkout vendor-upstream`
2. nahradiť obsah `legacy HMI_inst/<verzia>/` novým upstreamom, commitnúť
3. `git checkout main && git merge vendor-upstream` — Pro delta sa domerguje automaticky, ručne treba doriešiť len konflikty v 29 dotknutých súboroch

## Poznámka ku klonovaniu na Windows

Upstream frontend má hlboko zanorené cesty, ktoré prekračujú Windows limit 260 znakov. Pred klonovaním treba zapnúť:

```bash
git config --global core.longpaths true
```

## Licencia

Customized MIT (frangoteam). Distribúcia je zakúpená — použitie aj nasadenie u koncových zákazníkov je bez ďalších poplatkov. Ďalší predaj pod menom „legacy HMI Pro" je zakázaný; predaj pod vlastnou značkou vyžaduje dohodu s Frangoteam. Detaily v `legacy HMI_inst/v1.3.4-2890/LICENSE`.
