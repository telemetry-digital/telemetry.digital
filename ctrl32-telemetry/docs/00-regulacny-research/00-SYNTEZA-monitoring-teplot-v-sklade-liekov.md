# Monitoring teplôt v liekových skladoch: kompletná regulačná analýza

**Stav k 21. septembru 2026.** Toto nie je právne poradenstvo. Každý právny text uvedený nižšie treba pred použitím vo validačnom dokumente, v zmluve alebo v odpovedi inšpektorovi overiť priamo v Slov-Lexe (slovenské predpisy) alebo v EUR-Lexe / na EudraLex Volume 4 (európske predpisy). Časť citácií v tomto materiáli pochádza z nástroja, ktorý dlhé citáty skracoval — sú označené a nesmú sa doslovne prebrať bez kontroly originálu.

---

## Zhrnutie pre rozhodovanie: čo sa zmenilo a čo z toho plynie

Najdôležitejší nález celého výskumu je slovenský, nie európsky. **Vyhláška č. 419/2025 Z. z. novelizovala vyhlášku 129/2012 Z. z. tak, že od 1. júla 2026 musí každá slovenská lekáreň merať teplotu kalibrovaným meradlom, ktoré umožňuje automatickú evidenciu teploty najmenej v dvojhodinových intervaloch, a denné záznamy o teplote uchovávať 12 mesiacov** ([zakonypreludi 129/2012](https://www.zakonypreludi.sk/zz/2012-129); [vyhláška 419/2025](https://www.zakonypreludi.sk/zz/2025-419)). To je prvá prescriptívna, testovateľná teplotná norma v slovenskom liekovom práve — a tento termín **už uplynul**. Ku dňu tohto reportu je po termíne viac ako dva a pol mesiaca. Komerčný dôsledok treba povedať rovno: prvá vlna nákupov je s vysokou pravdepodobnosťou z veľkej časti za nami a kto na nej chcel zarobiť ako prvý, meškal. Ostáva však tri druhy dopytu, ktoré sú reálne: lekárne, ktoré termín nestihli a čakajú, kým na to niekto príde (ŠÚKL vykonal v roku 2024 **478 inšpekcií u poskytovateľov lekárenskej starostlivosti**, [ŠÚKL Výročná správa 2024](https://www.sukl.sk/buxus/docs/rozhodnutia/Vyrocna_sprava_2024_final.pdf)); lekárne, ktoré si v panike kúpili najlacnejší datalogger a teraz zisťujú, že im chýba kalibračný list, archív a mesačný report; a segment veľkodistribúcie, ktorého sa novela netýka vôbec a kde je právna úprava naďalej vágna — čiže tam sa predáva odbornosť, nie povinnosť.

Druhý nález je korekcia východiskového predpokladu. **128/2012 Z. z. je vyhláška o správnej výrobnej praxi a správnej veľkodistribučnej praxi. 129/2012 Z. z. je vyhláška o správnej lekárenskej praxi.** Skoršia úvaha, že 129/2012 pokrýva distribúciu, je nesprávna a treba ju z každého obchodného materiálu odstrániť — pomýliť si tieto dve čísla pred zákazníkom z veľkoskladu je rýchly spôsob, ako stratiť dôveryhodnosť.

Tretí nález mení to, proti akej verzii sa dnes validuje. **Revidovaná príloha 11 (Annex 11) k EU GMP nebola k 21. septembru 2026 publikovaná.** Oficiálny index EudraLex Volume 4 stále uvádza *„Annex 11 | Computerised Systems (revision January 2011)"*, neobsahuje žiadnu položku Annex 22, a zároveň je preukázateľne aktuálny — Annex 19 je na ňom označený ako *„applicable as of 24 September 2026"* ([EudraLex Volume 4](https://health.ec.europa.eu/medicinal-products/eudralex/eudralex-volume-4_en)). Pôvodný plán počítal s publikáciou Komisiou v júni 2026 a prijatím PIC/S v septembri 2026; tento plán je zmeškaný a po ňom sa očakáva ešte prechodné obdobie približne 12–18 mesiacov ([qmsdoc.com](https://qmsdoc.com/2026/01/15/revision-of-pic-s-gmp-annex-11-computerised-systems/)). **Reálny termín povinného súladu je teda 2028, najskôr.** Blogy s názvom „Annex 11:2026" sú marketingové texty o návrhu, nie o platnom štandarde — citovať ich ako dôkaz platného predpisu je chyba, ktorú zákazníkova QA odhalí.

Štvrtý nález je rovnakého druhu na strane distribúcie. **EU GDP 2013/C 343/01 je naďalej platný usmerňovací text** ([EMA, Good distribution practice](https://www.ema.europa.eu/en/human-regulatory-overview/post-authorisation/compliance-post-authorisation/good-distribution-practice)). Dokument Komisie **C(2026) 4135 final z 23. 6. 2026**, ktorý sa v agregátoroch objavuje pri vyhľadávaní „GDP 2026", nie je revíziou GDP — je to **GMP Annex 19: Reference and Retention Samples** ([C(2026) 4135 final](https://health.ec.europa.eu/document/download/831d392e-bcc4-4552-8011-2b724b8655de_en?filename=c2026-4135_en_act_en.pdf)). Tvrdiť zákazníkovi súlad s „GDP 2026" znamená tvrdiť súlad s dokumentom, ktorý neexistuje.

Piaty nález je o presnosti pozicionovania. **Rozsah Annexu 11 je výroba**, nie distribúcia: `[A11-2011]` Principle hovorí *„used as part of a GMP regulated activities"* a návrh z roku 2025 to sprísňuje na *„used in the manufacturing of medicinal products and active substances"*. Čistý distribučný sklad spadá pod **GDP**, nie priamo pod Annex 11. A **lekárne sú mimo rozsahu 2013/C 343/01 úplne** — každé operatívne ustanovenie je adresované „wholesale distributors" a cez článok 85b(3) smernice 2001/83/ES aj sprostredkovateľom. Identická „GDP compliance" správa pre veľkosklad aj pre lekáreň je preto nepresná a pred informovaným zákazníkom aj škodlivá.

Čo sa z toho dá predávať: kalibrované senzory s protokolom z akreditovaného laboratória, ročná rekalibrácia ako služba (nie ako právna povinnosť — takú slovenský predpis neukladá), mapovanie teplôt pre sklady, inštalácia a uvedenie do prevádzky, validačný balík IQ/OQ/PQ (ktorého právnym základom je Annex 15 clause 2.6, viď nižšie) a podporná zmluva. Cenové kotvy, ktoré v Slovensku existujú, sú nízke a treba s nimi počítať: kalibrované dataloggery €66–€154, akreditovaná kalibrácia €19,68–€72,57 za prístroj, Comet S0110E €94,24 bez DPH. Cena mapovacej štúdie nie je zverejnená nikde v EÚ — to je zároveň riziko aj príležitosť.

A napokon realita konkurencie: **LogHub je slovenský výrobca, deklaruje 400+ prevádzok, ISO 9001 a ISO 27001, rekalibráciu výmenou senzora, odkazuje sa na ŠÚKL a už marketingovo využíva „Nová legislatíva 2026 – elektronické meranie teplôt"** ([LogHub.sk](https://www.loghub.sk/)). Pozicionovať sa treba proti LogHubu, nie proti Vaisale.

---

## Legenda označení spoľahlivosti

V celom dokumente platí:

| Značka | Význam |
|---|---|
| ✅ **overené** | Znenie sa vrátilo konzistentne z primárneho zdroja (často opakovane a nezávisle) a zodpovedá publikovanému textu. Použiteľné ako citácia po bežnej kontrole. |
| ⚠️ **skrátené, treba overiť** | Nástroj na načítanie stránok skracoval citácie približne na 125 znakov. Vetu označenú takto **nesmieš doplniť z pamäti** a **nesmieš ju vložiť do validačného dokumentu ani do zákazníckeho súboru bez porovnania s oficiálnym PDF**. Substancia je spoľahlivá, presné znenie nie. |
| ❌ **nepoužívať** | Text, o ktorom je preukázané alebo dôvodne podozrivé, že ho generoval nástroj, nie zdroj. |

**Konkrétne varovanie, ktoré si zapamätaj:** pri práci s PIC/S PI 041-1 nástroj vrátil úhľadne očíslované „clauses 9.6.1 – 9.6.7" o audit trailoch. Následná štruktúrna kontrola toho istého dokumentu ukázala, že **sekcia 9.6 nie je takto číslovaná vôbec** — sekcie 9.6 a 9.7 sú tabuľky typu Item / Expectation / Potential risk, nie desatinné pododseky. Tých sedem „klauzúl" je ❌ **fabrikát a v tomto reporte sa nenachádza.** Ak niekde uvidíš citáciu „PI 041-1 clause 9.6.3", je to buď z rovnakého zdroja omylu, alebo si ju niekto vymyslel. Správny spôsob citovania je **„PI 041-1 §9.6"** alebo **„PI 041-1 §9.6, item N"**.

**Druhé varovanie, slovenské:** Slov-Lex sa nepodarilo prečítať priamo (konsolidované znenia servíruje cez JavaScript a priamy prístup na `static.slov-lex.sk` odmietol proxy). **Slovenské právne texty v tomto reporte pochádzajú zo zakonypreludi.sk / ASPI / EPI a z PDF hosťovaného ŠÚKL-om, nie z oficiálnej Zbierky.** Čísla paragrafov sú krížovo overené aspoň dvoma zdrojmi, doslovné znenie nie. Pred vložením do validačného dokumentu alebo do zmluvy ho skontroluj v Slov-Lexe.

**Tretie varovanie:** k európskym textom bol prístup len cez sumarizačný fetch; priame stiahnutie PDF z `health.ec.europa.eu`, `eur-lex.europa.eu`, `picscheme.org` a `assets.publishing.service.gov.uk` bolo blokované. Preto je časť klauzúl Annexu 11, návrhu Annexu 11 a GDP označená ⚠️.

---

## Ktoré pravidlá platia pre ktorého zákazníka

Toto je tabuľka, ktorú si vytlač a maj pri telefonáte. Odpoveď je pre každý typ zákazníka iná a zamieňanie je najčastejšia chyba dodávateľov.

| | **Výrobca liekov** (povolenie na výrobu) | **Veľkodistribútor** (povolenie na veľkodistribúciu) | **Lekáreň** (povolenie na lekárenskú starostlivosť) |
|---|---|---|---|
| **Slovenský zákon** | zákon 362/2011 Z. z. § 15 ods. 1 písm. a) — *„dodržiavanie správnej výrobnej praxe"* | zákon 362/2011 Z. z. § 18 — povinnosti držiteľa povolenia na veľkodistribúciu | zákon 362/2011 Z. z. § 23 ods. 1 písm. a) — *„dodržiavanie správnej lekárenskej praxe"* |
| **Slovenská vyhláška** | 128/2012 Z. z., PRVÁ ČASŤ (SVP) | 128/2012 Z. z., DRUHÁ ČASŤ, § 12–19 (SVDP) | **129/2012 Z. z. v znení 419/2025 Z. z.** (SLP) |
| **Teplotná požiadavka** | § 5 ods. 4: zariadenia sa *„pravidelne overujú, kalibrujú a validujú"* | § 15 ods. 1: teplota a vlhkosť sa *„pravidelne zaznamenávajú, kontrolujú a vyhodnocujú"*; § 15 ods. 2: indikujúce a signalizujúce zariadenia; § 13: *„kalibrované teplomery a vlhkomery"* | **§ 30 ods. 8 a § 31 ods. 3: kalibrované meradlo s automatickou evidenciou ≤ 2 h, denné záznamy 12 mesiacov, od 1. 7. 2026** |
| **Uchovávanie dokumentácie** | GMP Chapter 4: 1 rok po exspirácii šarže alebo 5 rokov po certifikácii QP, podľa toho, čo je dlhšie | **5 rokov** (vyhl. 128/2012 § 18 ods. 1 aj zákon 362/2011 § 18) | **12 mesiacov** pre denné teplotné záznamy; **3 roky** pre chladový reťazec vakcín podľa odborného usmernenia MZ SR |
| **Európsky rámec** | **EU GMP**, vrátane **Annex 11** (počítačové systémy) a **Annex 15** (kvalifikácia a validácia) | **EU GDP 2013/C 343/01**; Annex 11 iba analogicky a cez očakávanie inšpektora | **žiadny** — 2013/C 343/01 lekárne nepokrýva; rámec je čisto národný + podmienky uchovávania zo SPC |
| **Čo predávať** | plný validačný balík, mapovanie, dodávateľská kvalifikácia, zmluva o kvalite | mapovanie + monitoring + kvalifikácia, dôraz na „pravidelne" a na alarmy | kalibrované meradlo + 12-mesačný archív + mesačný report; alarmy a vzdialený prístup sú **upsell, nie povinnosť** |

Dve poznámky, ktoré z tabuľky priamo vyplývajú a ktoré predávajú lepšie ako brožúra. Po prvé: **požiadavka 129/2012 je napísaná ako vlastnosť *meradla*, nie systému ani softvéru** — *„kalibrované meradlo, ktoré umožňuje meranie a automatickú evidenciu teploty"*. Doslovne čítané to znamená kalibrovaný datalogger s automatickým záznamom a 12-mesačným archívom. Neukladá to cloud, alarmy, audit trail ani validáciu počítačového systému podľa Annexu 11. To limituje, čo môžeš lekárni čestne *požadovať* — a zároveň to jasne oddeľuje, čo je povinnosť a čo je služba navyše. Po druhé: **419/2025 opakuje požiadavku naprieč mnohými bodmi príloh** (body 39, 40, 47, 50, 52, 53, 55–59) pre rôzne oddelenia a typy miestností lekárne ([zakonypreludi 419/2025](https://www.zakonypreludi.sk/zz/2025-419)). Lekáreň s niekoľkými skladovými miestnosťami a chladničkou teda reálne potrebuje **viac meracích miest, nie jedno**. Ponuku kalkuluj na miestnosť, nie na prevádzku — je to presnejšie aj výhodnejšie.

---

## Slovenské právo: čo presne hovorí a čo z toho vyplýva

### Zákon č. 362/2011 Z. z. — rámcové povinnosti

Zákon sám neobsahuje technickú teplotnú špecifikáciu. Stanovuje rámcové povinnosti a technický detail deleguje na dve vyhlášky. To je dôležitejšie, než sa zdá: **povinnosti zákazníka sa môžu zmeniť samotnou vyhláškou ministerstva, bez novely zákona a bez parlamentu** — presne ako sa to stalo v decembri 2025. Regulačný spúšťač dopytu sa teda môže pohnúť rýchlo a bez varovania.

- **§ 15 ods. 1 písm. a)** ukladá držiteľovi povolenia na výrobu liekov *„dodržiavanie správnej výrobnej praxe"* ✅ (číslo § a substancia overené dvoma zdrojmi) — [static.slov-lex.sk 362/2011](https://static.slov-lex.sk/static/SK/ZZ/2011/362/20250131.html)
- **§ 18** je nadpísaný *Povinnosti držiteľa povolenia na veľkodistribúciu liekov*. Ukladá *„dodržiavanie správnej veľkodistribučnej praxe"*, zabezpečenie *„uchovávania liekov v podmienkach podľa predpisu … vrátane teploty"* a evidenčné povinnosti s uchovávaním *„najmenej päť rokov"* ⚠️ — [static.slov-lex.sk 362/2011](https://static.slov-lex.sk/static/SK/ZZ/2011/362/20250131.html); [zakonypreludi 362/2011](https://www.zakonypreludi.sk/zz/2011-362). Seat § 18 potvrdzuje aj [Lewik](https://www.lewik.org/term/2498/povinnosti-drzitela-povolenia-na-velkodistribuciu-liekov-zakon-o-liekoch-a-zdravotnickych-pomockach/).
- **§ 23 ods. 1 písm. a)** ukladá držiteľovi povolenia na poskytovanie lekárenskej starostlivosti *„dodržiavanie správnej lekárenskej praxe"* ✅

⚠️ **Dôležité obmedzenie:** doslovné znenie § 18 ods. 1 na úrovni písmen sa nepodarilo spoľahlivo zachytiť. Fetch naznačoval písm. b) pre SVDP a písm. c) bod 1 pre uchovávanie, ale to sa nepodarilo nezávisle potvrdiť. **Fragmenty § 18 vyššie ber ako presnú parafrázu, nie ako citáciu.** Pred použitím v ponuke ich over v Slov-Lexe.

Praktický dôsledok pre obchod: v ponuke citujte **obe vrstvy** — § 18 (resp. § 23) ako zákonný háčik a príslušnú vyhlášku ako technickú špecifikáciu, proti ktorej bude zákazník reálne kontrolovaný.

### Vyhláška č. 128/2012 Z. z. — výroba a veľkodistribúcia

Plný názov: *Vyhláška Ministerstva zdravotníctva Slovenskej republiky č. 128/2012 Z. z. o požiadavkách na správnu výrobnú prax a požiadavkách na správnu veľkodistribučnú prax* ([Slov-Lex 128/2012](https://slov-lex.sk/pravne-predpisy/SK/ZZ/2012/128); [ASPI](https://www.aspi.sk/products/lawText/1/77336/0/2/vyhlaska-c-128-2012-zz-o-poziadavkach-na-spravnu-vyrobnu-prax-a-poziadavkach-na-spravnu-velkodistribucnu-prax)). Správna veľkodistribučná prax je upravená v **DRUHEJ ČASTI, § 12–19**.

**§ 15 ods. 1** (*Uchovávanie liekov, liečiv, pomocných látok a obalov*) ✅:

> „Lieky, liečivá, pomocné látky a obaly sa uchovávajú oddelene. V tom istom priestore na uchovávanie nemožno uchovávať iný tovar."

a v tom istom odseku:

> „Teplota a vlhkosť sa pravidelne zaznamenávajú, kontrolujú a vyhodnocujú."

— [static.slov-lex.sk 128/2012](https://static.slov-lex.sk/static/SK/ZZ/2012/128/vyhlasene_znenie.html)

**Čo to znamená pre teba:** slovo je **„pravidelne"**, nie „automaticky", nie „najmenej v dvojhodinových intervaloch", nie „kalibrovaným meradlom". Distribučná strana teda nemá prescriptívne číslo. Predávať sa tu nedá povinnosťou; predáva sa tým, že „pravidelne" musí zákazník sám definovať a obhájiť pred inšpektorom — a to je presne to, čo mu dokážeš napísať do dokumentácie.

**§ 15 ods. 2** ✅ — doslovné znenie:

> „Ak sa na uchovávanie liekov vyžadujú osobitné podmienky na teplotu a vlhkosť priestoru na uchovávanie, priestory na uchovávanie sa vybavia zariadeniami indikujúcimi a signalizujúcimi nedodržanie požadovanej teploty a vlhkosti."

— [static.slov-lex.sk 128/2012](https://static.slov-lex.sk/static/SK/ZZ/2012/128/vyhlasene_znenie.html)

**Čo to znamená:** toto je slovenský právny základ pre **alarm**. „Indikujúce a signalizujúce" — teda nielen meranie, ale aj signalizácia prekročenia. Pre priestory s osobitnými podmienkami (chladený sortiment) je to povinnosť, nie doplnok. Toto je najsilnejšia veta pre predaj alarmovej funkcionality veľkodistribútorovi.

**§ 18 ods. 1** (*Dokumentácia*) ✅:

> „O veľkodistribúcii liekov sa vedie dokumentácia tak, aby bolo možné sledovať a kontrolovať všetky dôležité úkony veľkodistribúcie liekov. Dokumentácia sa uchováva najmenej päť rokov."

**§ 18 ods. 5** ✅ — dokumentácia *„sa ukladá tak, aby do nej mohli nazrieť osoby vykonávajúce štátny dozor"* — [static.slov-lex.sk 128/2012](https://static.slov-lex.sk/static/SK/ZZ/2012/128/vyhlasene_znenie.html)

**§ 5 ods. 4** (výrobná strana) ✅:

> „Zariadenia, stroje a prístroje používané pri výrobe lieku a jeho kontrole sa pravidelne overujú, kalibrujú a validujú."

**§ 13** (distribučné priestory) odkazuje na **„kalibrované teplomery a vlhkomery"** ✅ — [zakonypreludi 128/2012](https://www.zakonypreludi.sk/zz/2012-128)

⚠️ **Otvorená otázka, ktorú si over sám:** konsolidované zobrazenie na zakonypreludi ukazuje **iba jednu verziu 128/2012 s účinnosťou od 15. 5. 2012 a žiadne novely**. To je indícia, nie dôkaz — ide o absenciu dôkazu. Keby na distribučnej strane prišla paralelná novela, výrazne by to zmenilo veľkosť príležitosti v segmente veľkodistribúcie. **Over to v Slov-Lexe.**

### Vyhláška č. 129/2012 Z. z. v znení 419/2025 Z. z. — lekárne, a to podstatné

Plný názov: *Vyhláška Ministerstva zdravotníctva Slovenskej republiky z 21. marca 2012 o požiadavkách na správnu lekárenskú prax* ([PDF hosťované ŠÚKL](https://www.sukl.sk/buxus/docs/Inspekcia/Legislativa/vyhlaska_129_2012.pdf)).

**Pôvodné znenie z roku 2012** malo v § 30 ods. 8 aj v § 31 ods. 3 jedinú vetu ✅:

> „Teplota a vlhkosť sa denne počas prevádzky zaznamenávajú, kontrolujú a vyhodnocujú."

— [ŠÚKL PDF 129/2012](https://www.sukl.sk/buxus/docs/Inspekcia/Legislativa/vyhlaska_129_2012.pdf)

**Aktuálne znenie § 30 ods. 8** ⚠️ (znenie zo zakonypreludi, nie zo Zbierky):

> „Lieky, liečivá a pomocné látky sa uchovávajú za podmienok určených držiteľom registrácie lieku, výrobcom liečiv a pomocných látok, Európskym liekopisom alebo Slovenským farmaceutickým kódexom tak, aby sa zabránilo akémukoľvek ich poškodeniu teplom alebo vlhkom. **Teplota sa meria pomocou kalibrovaného meradla, ktoré umožňuje meranie a automatickú evidenciu teploty najmenej v dvojhodinových intervaloch. Denné záznamy o teplote sa uchovávajú 12 mesiacov.**"

— [zakonypreludi 129/2012](https://www.zakonypreludi.sk/zz/2012-129)

**§ 31 ods. 3** (zdravotnícke pomôcky, dietetické potraviny, doplnkový sortiment) nesie **identickú vetu** ⚠️:

> „Teplota sa meria pomocou kalibrovaného meradla, ktoré umožňuje meranie a automatickú evidenciu teploty najmenej v dvojhodinových intervaloch. Denné záznamy o teplote sa uchovávajú 12 mesiacov."

Novelizujúci predpis je **vyhláška č. 419/2025 Z. z.**, *ktorou sa mení a dopĺňa vyhláška Ministerstva zdravotníctva Slovenskej republiky č. 129/2012 Z. z. o požiadavkách na správnu lekárenskú prax*, **publikovaná 15. decembra 2025**, s účinnosťou **1. januára 2026 pre väčšinu bodov a 1. júla 2026 pre teplotné body** ([zakonypreludi 419/2025](https://www.zakonypreludi.sk/zz/2025-419)). Teplotné zmeny sú **bod 32 (§ 30 ods. 8) a bod 33 (§ 31 ods. 3)**, plus opakovanie cez prílohové body 39, 40, 47, 50, 52, 53, 55–59.

Nezávislé potvrdenie z odbornej tlače, že ide o **novú** povinnosť: *„Automatické zaznamenávanie teploty v lekárňach ako nová povinnosť od 1. júla 2026"*, s konštatovaním, že používanie kalibrovaných teplomerov s automatickým záznamom sa stáva povinnou súčasťou prevádzky lekárne a nahrádza spoliehanie sa na manuálne odčítanie jednoduchých teplomerov bez možnosti spätnej kontroly ([mediPRÁVNIK](https://www.medipravnik.sk/pravne-clanky/automaticke-zaznamenavanie-teploty-v-lekarnach-ako-nova-povinnost-od-1-jula-2026-2091/)).

**Analýza, ktorú si nikde inde neprečítaš.** Právna asymetria je celý obchodný príbeh. Lekáreň má teraz pravidlo, ktoré má číslo (2 hodiny), lehotu (12 mesiacov) a vlastnosť prístroja (kalibrované). Proti takému pravidlu sa nedá argumentovať nástenným teplomerom a papierovým hárkom. Veľkodistribútor má naďalej „pravidelne" — a s tým sa dá argumentovať čímkoľvek, čo si zákazník zdôvodní. **Prescriptívne pravidlo sa predáva samo, vágne pravidlo sa predáva odbornosťou.** Podľa toho nastav dve úplne odlišné predajné konverzácie.

⚠️ **Čo nebolo overené a čo si over:** oficiálne konsolidované znenie 129/2012 po novele sa zo Slov-Lexu nepodarilo získať; znenie vyššie pochádza zo zakonypreludi.sk a je krížovo potvrdené samostatnou stránkou 419/2025. Ďalej: celé telo 419/2025 (body 1–31, 34–38) nebolo prezreté — môžu tam byť ďalšie relevantné povinnosti k dokumentácii alebo priestorom. A napokon: **nezistilo sa, či existuje prechodné ustanovenie zmäkčujúce termín 1. 7. 2026 pre existujúce lekárne.** To je pre obchodnú argumentáciu po termíne dosť podstatné.

### Lehoty uchovávania — trojstupňový systém

Toto je detail, ktorý väčšina generických HACCP loggerov nemá premyslený, a preto je to diferenciátor.

| Kto / čo | Lehota | Zdroj |
|---|---|---|
| Veľkodistribútor — dokumentácia o veľkodistribúcii | **najmenej 5 rokov** | vyhl. 128/2012 § 18 ods. 1 ✅ |
| Veľkodistribútor — evidencia nadobudnutia/dodania | **najmenej 5 rokov** | zákon 362/2011 § 18 ⚠️ |
| Lekáreň — denné záznamy o teplote | **12 mesiacov** | vyhl. 129/2012 § 30 ods. 8, § 31 ods. 3 ⚠️ |
| Vakcíny — písomné záznamy chladového reťazca | **3 roky** | odborné usmernenie MZ SR, cit. podľa [Solen](https://www.solen.sk/storage/file/article/Lancova.pdf) ⚠️ |
| EU GDP — akýkoľvek dokument distribútora | *„the period stated in national legislation but at least five years"* | 2013/C 343/01 ch. 4.2 ✅ |
| EU GMP — šaržová dokumentácia | 1 rok po exspirácii šarže **alebo** 5 rokov po certifikácii QP, podľa toho, čo je dlhšie | GMP Chapter 4 ✅ |

Tri praktické závery. **Po prvé:** jeden produkt predávaný obom segmentom musí mať predvolenú najdlhšiu lehotu (5 rokov) alebo konfigurovateľnú lehotu podľa typu zákazníka. Predať veľkodistribútorovi 12-mesačnú retenciu znamená nechať ho v nesúlade. **Po druhé:** trojročné pravidlo pre vakcíny stojí **nad** 12-mesačným minimom lekárne pre tú istú fyzickú chladničku. Lekáreň, ktorá očkuje, teda pre túto chladničku reálne potrebuje 3-ročnú retenciu — to je konkrétny, obhájiteľný dôvod predať vyšší tarifný stupeň. **Po tretie:** keďže sa lehoty líšia, „retencia konfigurovateľná a zdokumentovaná pre každé miesto uloženia" je samo osebe diferenciátor, ktorý sa oplatí pomenovať v produktovej dokumentácii.

⚠️ Nepotvrdené: presný identifikátor, dátum a aktuálna platnosť *Odborného usmernenia MZ SR* o chladovom reťazci vakcín. Článok v Soléne ho cituje genericky ako publikované vo Vestníku MZ SR a môže byť staršie než aktuálne usmernenie. A: **nie je výslovne uvedené, či sa 5-ročná lehota distribútora vzťahuje aj na *teplotné* záznamy** — § 18 ods. 1 hovorí o dokumentácii „všetkých dôležitých úkonov veľkodistribúcie", čo je široké, ale nie explicitné k environmentálnemu monitoringu.

### Metrológia: kalibrované áno, „určené meradlo" nie

Toto je otázka, ktorú ti položí každý druhý zákazník, a odpoveď je pre teba priaznivá.

*Určené meradlá* sú podľa **§ 11 zákona 157/2018 Z. z.** definované ako *„meradlá, ktoré sa bez vykonania povinnej metrologickej kontroly alebo posúdenia zhody nesmú uviesť na trh a používať"*; zaradenie závisí od použitia — platby, ochrana zdravia, bezpečnosti, majetku a životného prostredia, spotrebiteľské balenia a podobné merania vo verejnom záujme ([ÚNMS SR, FAQ metrológia](https://www.normoff.gov.sk/stranka/247/faq-metrologia/)). ÚNMS zároveň rozlišuje ✅:

> „Cieľom kalibrácie je vytvorenie vzťahu medzi hodnotami indikovanými meradlom a hodnotami reprezentovanými etalónom"

— zatiaľ čo overenie zisťuje *„či hodnoty indikované určeným meradlom vyhovujú z hľadiska najväčších dovolených chýb"* ([ÚNMS SR](https://www.normoff.gov.sk/stranka/247/faq-metrologia/)). **Bežné teplomery nie sú v zozname určených meradiel** vo FAQ ÚNMS; merače tepla áno, ak sú osadené pred bytmi. Zoznam určených meradiel a ich metrologickú kontrolu upravuje **vyhláška č. 161/2019 Z. z. o meradlách a metrologickej kontrole** (príloha 1 určuje lehoty overenia) ([ASPI 161/2019](https://www.aspi.sk/products/lawText/1/92023/0/2/vyhlaska-c-161-2019-zz-o-meradlach-a-metrologickej-kontrole)).

Rozhodujúci argument je však jazykový a je v samotných zdravotníckych vyhláškach: **129/2012 § 30 ods. 8 hovorí „kalibrovaného meradla", nie „overeného"**; 128/2012 § 13 hovorí *„kalibrované teplomery a vlhkomery"* a § 5 ods. 4 *„pravidelne overujú, kalibrujú a validujú"*. To je silnejšie než mlčanie FAQ, pretože práve toto číta inšpektor.

**Záver:** nadväznosťou zabezpečená (akreditovaná) kalibrácia takmer isto postačuje; periodické úradné overenie sa takmer isto nevyžaduje. ⚠️ **Príloha 1 vyhlášky 161/2019 nebola čítaná priamo** — pred tvrdením v zákazníckom materiáli to over.

Akreditácia sa dá **prenajať, nie je nutné ju vlastniť**. SNAS akredituje kalibračné laboratóriá na Slovensku ([SNAS](https://www.snas.sk/akreditacia/kalibracne-laboratoria), [rozsah akreditácie PDF](https://www.snas.sk/storage/app/uploads/public/690/b52/44e/690b5244e1cd6236226642.pdf), [MSA-L/07](https://www.snas.sk/storage/app/uploads/public/61a/3a6/ca1/61a3a6ca14371704010175.pdf)), ale ISO/IEC 17025 akreditácia od **ktoréhokoľvek** člena EA je v EÚ vzájomne uznávaná — čo dokazuje KALIST AKL, ktorý predáva na Slovensku na základe **českej akreditácie ČIA**: *„Laboratórium teploty KALIST AKL je akreditované Českým inštitútom pre akreditáciu"*, s rozsahom −196 °C až 1100 °C ([KALIST AKL](https://www.kalistakl.sk/kalibracia-teplomerov)). Metershop deklaruje súlad svojich akreditovaných kalibrácií so **zákonom 157/2018** ([Metershop](https://metershop.sk/sluzby/kalibracia-teplomerov/)).

**To zásadne znižuje bariéru vstupu** — akreditácia je drahá časť tohto biznisu a dá sa subdodať. Zároveň z toho plynie tvrdé obmedzenie: **nesmieš sa vydávať za akreditované kalibračné laboratórium.** Čestná a stále predajná formulácia je: *„senzory dodávané s kalibračným listom z akreditovaného laboratória [meno], s rekalibráciou spravovanou za vás."*

⚠️ **Dôležitá poctivosť:** **žiadny slovenský právny zdroj neurčuje povinný rekalibračný interval** pre teplomery v lekárni alebo v sklade. Bežne predávaný ročný interval je obchodná konvencia, nie zákonná povinnosť. Dodávateľ, ktorý tvrdí „zákon vyžaduje ročnú rekalibráciu", klame — a zákazníkova QA to môže zistiť. Čestná formulácia: *„interval rekalibrácie si stanovuje a zdôvodňuje prevádzkovateľ; odporúčame ročný a takto ho zdôvodňujeme…"*.

### ŠÚKL: čo inšpektori skutočne píšu do protokolov

Toto je najsilnejší predajný materiál, aký vieš mať, pretože to nie sú slová dodávateľa, ale regulátora.

Z **Výročnej správy ŠÚKL za rok 2024**: **73 inšpekcií u veľkodistribútorov** a **478 inšpekcií u poskytovateľov lekárenskej starostlivosti** ⚠️ (čísla boli extrahované sumarizačným fetchom z PDF, pred publikovaním ich over priamo) ([ŠÚKL Výročná správa 2024](https://www.sukl.sk/buxus/docs/rozhodnutia/Vyrocna_sprava_2024_final.pdf)).

Časté nedostatky u **veľkodistribútorov** podľa tej istej správy ⚠️:

> „nedostatočne vykonaný teplotný monitoring priestorov"
> „chýbajúci teplotný monitoring počas prepravy"
> „chýbajúce kalibrácie kľúčových meradiel"

Časté nedostatky u **lekární** ⚠️:

> „nedostatočná kontrola/udržiavanie teplotných podmienok v rámci priestorov lekárne"
> „chýbajúce kalibrované teplomery a vlhkomery, resp. chýbajúca záznamová dokumentácia k monitorovaniu teplôt"
> „nedostatočný dohľad nad prepravnými podmienkami v rámci internetového výdaja"

ŠÚKL navyše prevádzkuje verejnú stránku *Zistené nedostatky pri priebežných inšpekciách v lekárňach* s konkrétnymi nálezmi ⚠️:

> „nie je teplomer v chladničke"
> „teplota a vlhkosť sa neevidujú denne"
> „teplomery nie sú kalibrované"
> „teplomery a vlhkomery nie sú vo všetkých miestnostiach, kde sa skladujú lieky"
> „na chladničke nie je zoznam liekov, nie sú vyznačené doporučené teploty uchovávania"

— [ŠÚKL, Zistené nedostatky](https://www.sukl.sk/hlavna-stranka/slovenska-verzia/inspekcia/lekarenstvo/zistene-nedostatky-pri-priebeznych-inspekciach-v-lekarnach?page_id=1724)

⚠️ **Dôležitá výhrada k tejto stránke:** ako governing predpisy cituje **vyhlášky č. 198/2001 a č. 274/1998**, teda *predchodcov* 129/2012 a 128/2012 ([AUTORA 198/2001](https://www.autora.sk/zz/2001/198/)). Stránka teda zjavne nebola roky revidovaná. Nálezy zostávajú indikatívne pre správanie inšpektora, ale **nesmieš ju prezentovať ako aktuálne regulačné usmernenie.** Zároveň to naznačuje, že slovenské lekárne dostávajú od úradu málo aktualizovanej komunikácie — čo zvyšuje hodnotu dodávateľa, ktorý zmenu z roku 2026 vysvetlí zrozumiteľne.

Ďalšie relevantné ŠÚKL zdroje: [zoznam vydaných osvedčení o dodržiavaní SVDP](https://www.sukl.sk/hlavna-stranka/slovenska-verzia/inspekcia/distribucia/zoznam-vydany-osvedceni-o-dodrziavani-spravnej-velkodistribucnej-praxe), [doklady potrebné pre vstupnú inšpekciu distribútora](https://sukl.sk/hlavna-stranka/slovenska-verzia/inspekcia/distribucia/doklady-potrebne-pre-vykonanie-vstupnej-inspekcie?page_id=1248) (žiadosť TL ODL 025, *Prevádzkový poriadok spoločnosti – odporúčaný obsah*, *Hygienicko-sanitačný poriadok – odporúčaný obsah*; inšpekcia do 30 dní od úplnej žiadosti, nález do 30 dní po inšpekcii — **stránka výslovne neuvádza mapovanie teplôt, kalibráciu, kvalifikáciu ani validáciu medzi požadovanými dokladmi**) a [legislatívna stránka inšpekcie](https://www.sukl.sk/hlavna-stranka/slovenska-verzia/inspekcia/legislativa?page_id=900).

**Zásadná medzera, ktorá je zároveň príležitosťou: nenašlo sa žiadne slovenské usmernenie ŠÚKL k mapovaniu teplôt.** Česká obdoba existuje — **SÚKL ČR, DIS-15 verze 4, *Sledování a kontrola teploty při skladování a přepravě léčiv*, aktualizované vo februári 2025** ([DIS-15 v4 PDF](https://sukl.gov.cz/wp-content/uploads/2025/02/DIS-15_verze4_Sledovani-a-kontrola-teploty-pri-skladovani-a-preprave-leciv.pdf)). Slovenskí zákazníci teda nemajú národný autoritatívny návod. Kto napíše zrozumiteľnú slovenskú metodickú poznámku (zosúladenú s EU GDP 2013/C 343/01 a analogicky s DIS-15), zaplní skutočné dokumentačné vákuum a získa poradenskú kredibilitu. **Musí ju však označiť ako svoj výklad, nie ako usmernenie ŠÚKL.**

⚠️ Nezistené: **žiadny inšpekčný checklist ŠÚKL k teplotnému monitoringu alebo mapovaniu**; žiadna pozícia ŠÚKL, či sa od distribútorov očakáva kontinuálny elektronický monitoring; sankcie za teplotné nedostatky (boli za platobnou bránou); údaje za rok 2025; celkový počet registrovaných lekární a licencovaných veľkodistribútorov v SR, teda veľkosť adresovateľného trhu.

### Lekárne a vakcíny: špecifiká, ktoré generický logger nerieši

Od 1. 7. 2026 musí každé miesto uloženia v lekárni — vrátane chladničiek a každej miestnosti, kde sa skladujú lieky — mať kalibrované meradlo s automatickým záznamom ≤ 2 h a 12-mesačnou retenciou. Na to sa vrství vakcínové usmernenie ⚠️ (všetko z [Solen, *Uchovávanie a distribúcia očkovacích látok*](https://www.solen.sk/storage/file/article/Lancova.pdf), sekundárny odborný zdroj):

- Rozsah 2–8 °C a zákaz zmrazenia: *„Teploty pod bodom mrazu môžu nezvratne redukovať účinnosť vakcín"*; DTP kombinácie sa nesmú skladovať pod +2 °C.
- Frekvencia záznamu: *„Teplota v chladničke musí byť denne písomne zaznamenaná (vrátane dní pracovného voľna)"* — denne, vrátane dní pracovného voľna, ideálne dvakrát denne na začiatku a konci zmeny.
- Umiestnenie a typ senzora: teplomer v strede úložného priestoru; min–max teplomery sú optimálne, pretože *„monitorujú skutočnú teplotu, minimálnu aj maximálnu teplotu"*.
- Obsah záznamu: dátum, čas, odčítaná teplota, **meno pracovníka** a **vykonané nápravné opatrenia**; archivácia tri roky.
- Regulačným vlastníkom chladového reťazca je **ÚVZ SR** ([ÚVZ SR](https://www.uvzsr.sk/web/uvz)), požiadavky sú odvodené z *Odborného usmernenia MZ SR* vo Vestníku.

**Dve veci z toho predávaj prednostne.** Prvá: pravidlo *„vrátane dní pracovného voľna"* sa nedá splniť pracovníkom, ktorý odčíta ručičku — presne preto bol nariadený automatický záznam. To mení nákup z „pekné mať" na „jediný spôsob, ako byť v nedeľu v súlade". Druhá: požiadavka, aby záznam obsahoval **meno zodpovednej osoby a nápravné opatrenie**, nie je splnená holým exportom z loggera. Mesačný report v slovenčine s miestom na podpis a na komentár k odchýlke zodpovedá reálnemu inšpekčnému artefaktu, ktorý lekárnik musí predložiť — malá funkcia s neúmerne vysokou vnímanou hodnotou.

Realistický počet meracích miest na jednu lekáreň je **2–4** (minimálne jedna sonda do chladničky plus jedna na každú miestnosť so skladovaním liekov). To je skutočná veľkosť obchodu, nie predaj jedného senzora.

⚠️ Nevyriešené: či dvakrát denne vykonávaný manuálny záznam podľa vakcínového usmernenia prežíva popri novom pravidle automatického záznamu, alebo je ním nahradený. Obidva nástroje majú odlišný právny status (vyhláška vs odborné usmernenie) a nenašiel sa zdroj, ktorý by ich zosúlaďoval. A: **žiadny slovenský predpis neurčuje požadovanú presnosť (napr. ±0,5 °C) ani alarmové prahy** pre prístroje v lekárni.

---
## EU GDP 2013/C 343/01: doslovné znenie a čo z neho naozaj plynie

Kanonická citácia je **OJ C 343, 23.11.2013, p. 1–14**, CELEX **52013XC1123(01)**, plný názov *Guidelines of 5 November 2013 on Good Distribution Practice of medicinal products for human use* ([EUR-Lex CELEX](https://eur-lex.europa.eu/legal-content/EN/TXT/?uri=CELEX:52013XC1123(01)); [PDF](https://eur-lex.europa.eu/LexUriServ/LexUriServ.do?uri=OJ:C:2013:343:0001:0014:EN:PDF)).

**Právny status, ktorý treba vedieť presne.** Ide o usmernenie Komisie publikované v sérii C (oznámenia a informácie), nie v sérii L (legislatíva). Nie je priamo záväzné tak, ako nariadenie. Jeho praktická sila plynie z dvoch vecí: z **článku 80 písm. d) smernice 2001/83/ES**, ktorý ukladá držiteľom povolenia na veľkodistribúciu dodržiavať zásady SVDP, a z národnej transpozície — v SR zákon 362/2011 Z. z. a inšpekčná prax ŠÚKL. **Článok 84** je splnomocnenie na publikovanie GDP usmernení, **článok 85b ods. 3** rozširuje tie isté usmernenia na sprostredkovateľov liekov ([EMA](https://www.ema.europa.eu/en/human-regulatory-overview/post-authorisation/compliance-post-authorisation/good-distribution-practice)). Rozdiel medzi „požiadavkou" a „usmernením, ktoré inšpektori vynucujú" je presne tá nuansa, ktorú treba v ponuke pomenovať — nie preto, aby si znižoval význam, ale preto, že tým získaš kredibilitu u zákazníka, ktorý to vie.

⚠️ Doslovné znenie článku 84 a 85b ods. 3 smernice 2001/83/ES sa nepodarilo získať; cituj ich priamo z konsolidovanej smernice na EUR-Lexe.

### Štruktúra a doslovné klauzuly

Kapitola 3 je členená **podsekciami**, nie priebežne číslovanými odsekmi: **3.1 Principle, 3.2 Premises, 3.2.1 Temperature and environment control, 3.3 Equipment, 3.3.1 Computerised systems, 3.3.2 Qualification and validation**. Toto číslovanie je overené voči EUR-Lexu ✅.

⚠️ **Konflikt číslovania, ktorý musíš poznať.** Kópia hostovaná Komisiou bola prepísaná s **priebežným číslovaním odsekov** (3.0 Principle; 3.13 environment control; 3.14 mapping; 3.15 equipment design; 3.16 calibration/verification; 3.17–3.18 traceability + alarms; 3.19 records of repair/maintenance/calibration; 3.20+ computerised systems; 3.26+ qualification and validation; 4.5 retention). **Oficiálny text OJ toto číslovanie nepoužíva.** Buď je komisná kópia prelomená/konsolidovaná edícia, ktorá čísla doplnila, alebo ich vygeneroval sumarizátor — rozlíšiť sa to nedalo. **Praktické pravidlo: cituj podsekcie (3.2.1, 3.3, 3.3.1, 3.3.2, 4.2), nikdy „3.13/3.14/3.16/3.18".** Znenie viet je medzi oboma zdrojmi konzistentné a je bezpečné ho citovať; sporné je len číslovanie.

**3.1 Principle** ✅ [VERBATIM — EUR-Lex]:

> "Wholesale distributors must have suitable and adequate premises, installations and equipment, so as to ensure proper storage and distribution of medicinal products."

*Všimni si „must".* Toto je jedna z mála skutočne kogentných formulácií v kapitole 3 a zrkadlí zákonné znenie článku 79 písm. b) / 80 smernice 2001/83/ES. **Pre teba:** toto je veta, ktorou sa otvára ponuka pre veľkosklad — nie „mali by ste", ale „musia mať".

**3.2 Premises** ✅:

> "The premises should be designed or adapted to ensure that the required storage conditions are maintained."

> "Medicinal products should be stored in segregated areas which are clearly marked and have access restricted to authorised personnel."

> "Any falsified medicinal products, expired products, recalled products and rejected products found in the supply chain should be immediately physically segregated."

**Pre teba:** tretia veta je dôvod, prečo má zmysel mať v softvéri pojem „karanténna zóna" ako samostatné monitorované miesto s vlastnými limitmi — segregované priestory sú tiež skladové priestory a tiež potrebujú podmienky.

**3.2.1 Temperature and environment control** ✅ (overené voči EUR-Lexu):

> "Suitable equipment and procedures should be in place to check the environment where medicinal products are stored."

> "An initial temperature mapping exercise should be carried out on the storage area before use, under representative conditions."

Tú istú prvú vetu o mapovaní nezávisle cituje aj inšpektorát MHRA a pripisuje ju GDP 3.2.1 ([MHRA Inspectorate, *Temperature mapping – an introduction*](https://mhrainspectorate.blog.gov.uk/2016/07/14/temperature-mapping-an-introduction/)).

Ďalšie vety tej istej podsekcie ⚠️ (vrátili sa konzistentne, ale ako izolované vety z dlhšieho odseku):

> "Temperature monitoring equipment should be located according to the results of the mapping exercise, ensuring that monitoring devices are positioned in the areas that experience the extremes of fluctuations."

> "The mapping exercise should be repeated according to the results of a risk assessment exercise or whenever significant modifications are made to the facility or the temperature controlling equipment."

> "For small premises of a few square meters which are at room temperature, an assessment of potential risks (e.g. heaters) should be conducted and temperature monitors placed accordingly."

— [EU GDP 2013/C 343/01](https://eur-lex.europa.eu/LexUriServ/LexUriServ.do?uri=OJ:C:2013:343:0001:0014:EN:PDF)

**Pre teba — toto je komerčná chrbtica celej mapovacej služby.** Reťaz je: mapovanie pred použitím → mapovanie určí extrémy → trvalé senzory idú na extrémy → mapovanie sa opakuje pri zmene. Kto predá senzory bez mapovania, predal zákazníkovi inštaláciu, ktorú nevie obhájiť pred inšpektorom. Kto predá mapovanie prvé, urobil z počtu a pozícií senzorov **auditovaný výstup namiesto položky v cenovej ponuke.** A tretia veta („small premises of a few square meters") je zároveň carve-out, ktorý ti umožní čestne povedať malému skladu, že plné mapovanie nepotrebuje — stačí posúdenie rizík (radiátory) a podľa toho umiestnené monitory. To, že vieš zákazníkovi povedať „toto nepotrebujete", je najsilnejší predajný nástroj, aký sole trader má.

**3.3 Equipment** ✅:

> "All equipment impacting on storage and distribution of medicinal products should be designed, located and maintained to a standard which suits its intended purpose."

> "Equipment used to control or to monitor the environment where the medicinal products are stored should be calibrated at defined intervals."

Rozšírené znenie z komisnej kópie ⚠️:

> "Equipment used to control or to monitor the environment of the medicinal product, should be calibrated and their correct operation and suitability for purpose verified at defined intervals by the appropriate methodology."

> "Calibration of equipment should be traceable to a primary standard."

> "Appropriate alarm systems should be in place to provide alerts when there are excursions [deviations] from pre-defined storage conditions."

> "Adequate records of repair, maintenance and calibration activities for key equipment should be made and the results should be retained."

— [European Commission GDP guidelines PDF](https://health.ec.europa.eu/document/download/9f28179a-a6f8-418a-a5fc-ecfbb3ae3ba8_en)

**Pre teba, tri veci.** Prvá: *„traceable to a primary standard"* je operatívna fráza pre dodávateľa hardvéru — každý LoRaWAN senzor dodaný do GDP prevádzky potrebuje kalibračný list s neprerušeným reťazcom nadväznosti. ISO/IEC 17025 akreditácia vydávajúceho laboratória je obvyklý spôsob, ako to preukázať, ale **GDP ju nemenuje**. Druhá: *„verified at defined intervals"* naznačuje, že GDP počíta s **dvoma odlišnými činnosťami** — periodická laboratórna kalibrácia a priebežné overovanie v prevádzke. Softvér, ktorý eviduje oboje, je v súlade s textom. Tretia: **jediná veta o alarmoch v celom GDP je tá vyššie.** Nič viac. Žiadne alarmové limity, žiadna frekvencia testovania alarmov, žiadne eskalácie, žiadne pokrytie mimo pracovnej doby, žiadne ustanovenie o výpadku napájania. To nie je zlyhanie výskumu — je to skutočný obsah usmernenia. Bremeno definovať, čo znamená „appropriate", leží celé na systéme kvality prevádzky. **Platforma, ktorá tieto parametre robí konfigurovateľnými a auditovateľnými, predáva presne ten dôkaz, ktorý bude inšpektor pýtať.**

**3.3.1 Computerised systems** ✅:

> "Before a computerised system is brought into use, it should be demonstrated that the system is capable of achieving the desired results accurately, consistently and reproducibly."

Komisná kópia dopĺňa ⚠️:

> "Before a computerised system is brought into use, it should be confirmed as being capable of achieving the desired results including full traceability of distribution operations."

a vyžaduje ochranu dát *„for a period stated in national legislation but at least 5 years."*

**3.3.2 Qualification and validation** ✅:

> "Wholesale distributors should identify what key equipment qualification and/or key process validation is necessary to ensure correct installation and operation."

Komisná kópia ⚠️:

> "Wholesale distributors should identify what qualification and/or validation work is necessary to demonstrate control of key aspects of their activities."

> "Storage areas should be temperature mapped under representative conditions and should take into account seasonal variations."

**Kapitola 4 — Documentation, 4.1 Principle** ✅:

> "Good documentation constitutes an essential part of the quality system. Written documentation should prevent errors from spoken communication and permits the tracking of relevant operations during the distribution of medicinal products."

**4.2 General** ✅ (vrátené identicky z dvoch nezávislých fetchov dvoch rôznych hostov — najvyššia dôvera v celom korpuse):

> "Documents should be retained for the period stated in national legislation but at least five years."

a tiež:

> "Documentation should be sufficiently comprehensive with respect to the scope of the wholesale distributor's activities."

⚠️ Kapitola 4 GDP obsahuje **iba 4.1 a 4.2**; 4.3 neexistuje (uvedené fetchom, nepotvrdené nezávisle).

**Odkiaľ presne pochádza „päť rokov".** Nie je to vynález GDP usmernenia. Plynie z **článku 80 smernice 2001/83/ES** (záznamy dostupné príslušným orgánom na inšpekciu päť rokov) a GDP kapitola 4.2 to potom preberá ako **podlahu** — „the period stated in national legislation **but at least five years**". Pre slovenského zákazníka to znamená: ak § 18 zákona 362/2011 alebo požiadavka ŠÚKL určujú dlhšiu lehotu, platí dlhšia. Päť rokov je minimum EÚ, nie strop. ⚠️ **Doslovné znenie článku 80 písm. e)/f) sa nepodarilo získať** — šesť rôznych pokusov (EUR-Lex ELI, CELEX HTML, konsolidované PDF, EMA-hosted PDF, legislation.gov.uk) zlyhalo. Over v EUR-Lexe pred použitím.

### Čo GDP nepredpisuje — a prečo je to dôležité

Povedz to zákazníkovi rovno, pretože ťa to odlíši od konkurencie, ktorá predstiera opak:

**EU GDP nepredpisuje ani jedno číslo.** Žiadne rozstupy senzorov, žiadny počet senzorov, žiadnu dĺžku mapovania, žiadny interval rekalibrácie, žiadnu presnosť, žiadnu neistotu merania, žiadne alarmové limity, žiadnu frekvenciu testovania alarmov, žiadny interval opakovaného mapovania. Každé konkrétne číslo, ktoré v tomto obore uvidíš — 5–10 m mriežka, 24–72 h, 7 dní, 3 roky, ±0,5 °C — pochádza z **technických doplnkov WHO** alebo z praxe dodávateľov. WHO sú odporúčania, nie požiadavky EÚ. Európski inšpektori vrátane MHRA ich však berú ako referenčnú metodiku.

**Praktický dôsledok:** tieto čísla patria do **mapovacieho protokolu a posúdenia rizík zákazníka, s WHO ako zdôvodnením**, nie do tvrdenia „EU GDP to vyžaduje". MHRA sama výslovne odmieta určiť počet: vyžaduje *„calibrated monitoring probes in sufficient numbers dependant on the size of the storage area"* — a nič viac ([MHRA](https://mhrainspectorate.blog.gov.uk/2016/07/14/temperature-mapping-an-introduction/)).

Chýbajúca úprava neistoty merania je osobitne pozoruhodná. **Nič v EU GDP nevyžaduje uvedenie neistoty**, a nič nehovorí, či sa má limit 2–8 °C sprísniť o neistotu senzora. Dodávateľ, ktorý tvrdí „presnosť v súlade s GDP", tvrdí niečo, čo GDP nedefinuje.

### Lekárne sú mimo rozsahu GDP

Každé operatívne ustanovenie, ktoré sa podarilo získať, je adresované „wholesale distributors" — 3.1: *„Wholesale distributors must have suitable and adequate premises…"*; 3.3.2: *„Wholesale distributors should identify what key equipment qualification…"*. EMA rámcuje usmernenia ako platné pre držiteľov povolenia na veľkodistribúciu, s certifikátmi vedenými v **EudraGMDP**, ktorý EMA opisuje ako *„a publicly accessible EU database containing all wholesale distribution authorisations and details of registered importers and distributors of active substances in the EEA"* ([EMA](https://www.ema.europa.eu/en/human-regulatory-overview/post-authorisation/compliance-post-authorisation/good-distribution-practice)).

**Dôsledok pre pozicionovanie:** veľkosklad je vnútri GDP a bude podľa neho kontrolovaný. Lekáreň kupuje tú istú technológiu, ale regulačný driver je iný — národná lekárenská vyhláška, podmienky uchovávania zo SPC, a pre lekárne s veľkodistribučným povolením aj GDP. **Rovnaká „GDP compliance" správa pre oboch je nepresná.** Pre lekáreň je najcennejšie tvrdenie vôbec nie Annex 11 ani Part 11 — je to **vyhlásenie o zhode s vyhláškou 129/2012 § 30 ods. 8 / § 31 ods. 3 po jednotlivých bodoch** (kalibrované meradlo, automatická evidencia ≤ 2 h, retencia 12 mesiacov). Nenašiel sa žiadny konkurent, ktorý by taký dokument publikoval. Je lacný na výrobu a odpovedá presne na otázku, ktorú lekárnikovi položí inšpektor.

### Ďalšie poznámky k platnosti a k okolitým dokumentom

- Dočasné covidové predĺženia platnosti GDP certifikátov (do konca 2024) **už neplatia**; predĺženia rozhodujú členské štáty prípad od prípadu ([EMA](https://www.ema.europa.eu/en/human-regulatory-overview/post-authorisation/compliance-post-authorisation/good-distribution-practice)).
- ⚠️ **Nepodarilo sa potvrdiť, že 2013/C 343/01 krížovo odkazuje na GMP Annex 11.** V žiadnom získanom výňatku sa taký odkaz neobjavil. Je pravdepodobné, že Annex 11 sa na GDP počítačové systémy aplikuje **očakávaním inšpektorátu a analógiou**, nie textovým odkazom. **Netvrdiť, že GDP 3.3 cituje Annex 11, kým to nie je overené v texte OJ.**
- Získať sa nepodarilo: úplné neprerušené odseky 3.2.1 a 3.3; kapitola 1 (odchýlky, CAPA, change control, riadenie rizík kvality); kapitola 5 (operácie, riešenie teplotných odchýlok a dispozícia so zásobami); kapitola 9 (preprava) — s výnimkou nepotvrdenej informácie ⚠️, že kapitola 9.2 vyžaduje kalibráciu meracích zariadení vo vozidlách a kontajneroch **najmenej raz ročne**; slovenská jazyková verzia 2013/C 343/01.
- **PIC/S PE 011** (GDP guide pre lieky) sa nezískal. Dokument stiahnutý z `picscheme.org/docview/2466` sa ukázal byť **PI 047-1, *Guidelines on the Principles of Good Distribution Practice of Active Substances for Medicinal Products for Human Use*, 1. júla 2018** — teda usmernenie pre účinné látky, nie pre lieky. Pre úplnosť, jeho §5.3 znie ⚠️: *„Monitoring devices that are necessary to guarantee the quality attributes of the active substance should be calibrated according to an approved schedule against certified traceable standards."* ([PI 047-1](https://picscheme.org/docview/2466)). **Neodpovedá to na otázku o liekoch.**

---

## EU GMP Annex 11 (2011) — platná verzia, klauzula po klauzule

Zdroj všetkých citácií v tejto sekcii: [EudraLex Volume 4, Annex 11 Computerised Systems, revision January 2011 (PDF)](https://health.ec.europa.eu/system/files/2016-11/annex11_01-2011_en_0.pdf), účinný od 30. júna 2011. Toto je **verzia, proti ktorej sa dnes validuje.**

### Principle ✅ (citované v plnom znení)

> "This annex applies to all forms of computerised systems used as part of a GMP regulated activities. A computerised system is a set of software and hardware components which together fulfill certain functionalities. The application should be validated; IT infrastructure should be qualified. Where a computerised system replaces a manual operation, there should be no resultant decrease in product quality, process control or quality assurance. There should be no increase in the overall risk of the process."

**Pre telemetry.digital:** rozdelenie **„application validated / IT infrastructure qualified"** je základná architektúra celého annexu a mal by sa premietnuť do štruktúry tvojich dokumentov. Go aplikácia + PostgreSQL schéma + alarmová logika = *aplikácia*, ktorá sa **validuje**. Servery, OS, sieť, hosting, senzory a gateways = *IT infraštruktúra*, ktorá sa **kvalifikuje** (IQ/OQ). Sú to dva samostatné dokumentačné prúdy. A ak produkt nahrádza papierové zapisovače alebo manuálne odčítanie dvakrát denne, musíš vedieť obhájiť, že **nedošlo k zvýšeniu celkového rizika procesu**. Zjavné nové riziko, ktoré zavádzaš, je **tiché zlyhanie** — senzor prestane hlásiť a nikto si to nevšimne. Presne preto je v návrhu kapitola 8 o alarmoch a v platnej verzii klauzula 16 o kontinuite.

### 1. Risk Management ⚠️

> "Risk management should be applied throughout the lifecycle of the computerised system taking into account patient safety, data integrity and product quality."

*(Klauzula pokračuje druhou vetou o tom, že rozhodnutia o rozsahu validácie a kontrolách integrity údajov majú vychádzať z odôvodneného a zdokumentovaného posúdenia rizík — tú sa nepodarilo získať doslovne. ⚠️)*

**Pre teba:** trojica **patient safety / data integrity / product quality** sa v annexe opakuje stále dokola; používaj ju ako osnovu každého zdôvodnenia. Každé návrhové rozhodnutie — interval vzorkovania, alarmové prahy, redundancia, retencia — potrebuje zdôvodnenie viazané na tieto tri. **Jednostranové posúdenie rizík (GAMP 5 štýl alebo FMEA nad „senzor zlyhá / gateway offline / DB sa poškodí / alarm sa nedoručí") je najužitočnejší dokument, aký môžeš napísať ako prvý**, pretože klauzuly 4, 9, 12 aj 16 svoj rozsah odvodzujú práve od neho.

### 2. Personnel ⚠️

> "All personnel should have appropriate qualifications, level of access and defined responsibilities to carry out their assigned duties."

*(Klauzula ďalej pokrýva úzku spoluprácu kľúčových osôb ako Process Owner, System Owner, Qualified Persons a IT — nezískané doslovne. ⚠️)*

**Pre jednoosobovú firmu je toto reálny problém a treba ho riešiť čelne.** Roly **System Owner, Process Owner a QA** musia byť v dokumentácii zákazníka **menovite obsadené** a ty ich nemôžeš držať všetky u zákazníka. Vo vlastnom QMS musíš preukázať vlastnú kompetentnosť — životopis, záznam o školeniach, doklad o znalosti GAMP/GMP. To je prvá vec, ktorú si audit u dodávateľa vypýta.

### 3. Suppliers and Service Providers ✅ — klauzuly, ktoré sa vzťahujú na teba

> **3.1** "Formal agreements must exist between the manufacturer and any third parties, and these agreements should include clear statements of the responsibilities of the third party."
>
> **3.2** "The competence and reliability of a supplier are key factors when selecting a product or service provider." ⚠️ *(zvyšok klauzuly o potrebe auditu a jej zdôvodnení na základe rizika sa nezískal)*
>
> **3.3** "Documentation supplied with commercial off-the-shelf products should be reviewed by regulated users to check that user requirements are fulfilled."
>
> **3.4** "Quality system and audit information relating to suppliers or developers of software and implemented systems should be made available to inspectors on request."

**Toto je tvoja sekcia.** Všimni si formuláciu 3.1: **„must exist"**, nie „should". Predávať bez písomnej zmluvy o kvalite znamená, že zákazník zlyháva na klauzule 3.1 v deň jedna. A 3.4 znamená, že **tvoj vlastný systém kvality sa môže nepriamo dostať pred regulátora.**

### 4. Validation

> **4.1** ⚠️ "Manufacturers should be able to justify their standards, protocols, acceptance criteria, procedures and records based on their risk assessment."
>
> **4.2** ⚠️ "Validation documentation should include change control records (if applicable) and reports on any deviations observed."
>
> **4.3** ⚠️ "An up to date listing of all relevant systems and their GMP functionality (inventory) should be available."
>
> **4.4** ⚠️ "User Requirements Specifications should describe the required functions of the computerised system and be based on documented risk assessment and GMP impact." *(oficiálna klauzula pokračuje, že požiadavky majú byť sledovateľné počas celého životného cyklu — to je explicitný európsky základ pre matricu sledovateľnosti)*
>
> **4.5** ✅ (celá) "The regulated user should take all reasonable steps, to ensure that the system has been developed in accordance with an appropriate quality management system. The supplier should be assessed appropriately."
>
> **4.6** ⚠️ "For the validation of bespoke or customised computerised systems there should be a process in place that ensures the formal assessment and reporting of quality and performance measures."
>
> **4.7** ⚠️ "Evidence of appropriate test methods and test scenarios should be demonstrated. Particularly, system (process) parameter limits, data limits and error handling should be considered."
>
> **4.8** ✅ (celá) "If data are transferred to another data format or system, validation should include checks that data are not altered in value and/or meaning during this migration process."

**Čo z každej plynie konkrétne.**

**4.3** — zákazník potrebuje položku v inventári systémov. Priprav mu hotový jednostranový „system description / GMP functionality" list, ktorý si len vloží. Je to päťminútová práca a vyzerá to profesionálne.

**4.4** — **URS patrí zákazníkovi, nie tebe.** Malé slovenské sklady ho však napísať nevedia. Najlepší komerčný ťah: dodaj **šablónu URS**, ktorú si zákazník prečíta, upraví a schváli **vo vlastnom mene**. Návrh to robí explicitným: draft 9.9 hovorí, že regulovaný používateľ *„is fully accountable and should carefully review and authorise the use of the documentation"* — mierené presne na prax dodávateľov, ktorí dodávajú validačné balíky.

**4.5** — *„The supplier should be assessed appropriately."* Toto je háčik pre dodávateľský audit.

**4.6** — pretože telemetry.digital je **bespoke softvér, ktorý píšeš ty**, táto klauzula ťa zasahuje najtvrdšie. Potrebuješ zdokumentovaný vývojový proces s formálnym posudzovaním a reportovaním kvality a výkonu. V Go realite: verziovanie s zmysluplnou históriou, code review (problém pre jednotlivca — zvažuj zdokumentované kontrolné zoznamy vlastnej revízie alebo externého recenzenta), unit/integračné testy so zaznamenanými výsledkami, definovaný release proces a matrica sledovateľnosti URS → návrh → test.

**4.7** — testuj **limity a spracovanie chýb explicitne**: čo sa stane na hranici rozsahu senzora, pri nezmyselnej hodnote (−300 °C), pri odpojení gatewaya, pri zlyhaní zápisu do DB, pri skoku hodín alebo zmene letného času, pri duplicitných časových pečiatkach. **Testy chybových stavov sú tie, ktoré najčastejšie chýbajú a ktoré inšpektori najčastejšie vytýkajú.**

**4.8** — platí vždy, keď migruješ zákazníka zo starého loggera, meníš major verziu PostgreSQL cez dump/restore alebo meníš schému meraní. Vyžaduje zdokumentované kontroly, že hodnoty **aj význam** (jednotky, časové pásmo, identita senzora) prežili.

### 5. Data ✅ (celá)

> "Computerised systems exchanging data electronically with other systems should include appropriate built-in checks for the correct and secure entry and processing of data, in order to minimize the risks."

**Toto je rozhraniová klauzula a pre telemetriu je centrálna**, pretože celý produkt je výmena dát medzi systémami: senzor → gateway → ingest API → PostgreSQL → dashboard/report/export. „Built-in checks" znamenajú **technické kontroly v softvéri, nie procedúru**: kontrolné súčty/CRC na linke senzor–gateway, autentifikovaný a TLS šifrovaný ingest, **poradové čísla / detekcia medzier**, aby chýbajúci interval bol *detekovaný a viditeľný* namiesto ticho chýbajúceho, odmietnutie alebo karanténa poškodených payloadov, idempotenčné kľúče proti duplicitnému vloženiu a rozsahová/plauzibilitná validácia pri ingeste. Ak exportuješ do WMS/ERP zákazníka alebo posielaš CSV/PDF mailom, aj tie prenosy potrebujú kontroly.

**Tichá medzera v dátach je klasické zlyhanie klauzuly 5 v teplotnom monitoringu.** Systém musí vedieť dokázať, že **absencia dát je viditeľná.**

### 6. Accuracy Checks ✅ (celá)

> "For critical data entered manually, there should be an additional check on the accuracy of the data. This check may be done by a second operator or by validated electronic means. The criticality and the potential consequences of erroneous or incorrectly entered data to a system should be covered by risk management."

**Pre teba:** v prevažne automatizovanom systéme sú rizikovou plochou práve manuálne vstupy — **alarmové prahy, kalibračné offsety/koeficienty, priradenie senzora k miestu, komentáre k odchýlkam, hodnoty z kalibračného listu, parametre mapovacej štúdie.** Každý z nich je kritický ručne zadaný údaj. Buď postav do aplikácie **potvrdenie druhou osobou (štyri oči)** pre zmeny prahov a kalibrácie, alebo implementuj „validated electronic means" (napr. opätovné zadanie + rozsahová validácia) — a zdôvodni v posúdení rizík, prečo zvolená metóda stačí. **Zmena alarmového limitu z +8 °C na +15 °C je najkonzekventnejší manuálny vstup v celom produkte** a zaslúži si najsilnejšiu kontrolu: schválenie druhou osobou + audit trail + poznámka v alarmovom logu.

### 7. Data Storage ✅ (obe klauzuly celé)

> **7.1** "Data should be secured by both physical and electronic means against damage. Stored data should be checked for accessibility, readability and accuracy. Access to data should be ensured throughout the retention period."
>
> **7.2** "Regular back-ups of all relevant data should be done. Integrity and accuracy of backup data and the ability to restore the data should be checked during validation and monitored periodically."

**7.2 nie je splnená vetou „máme nočný pg_dump".** Sú to tri odlišné povinnosti: (a) zálohy sú *pravidelné* podľa zdokumentovaného plánu; (b) **integrita, presnosť a obnoviteľnosť sa kontrolujú počas validácie** — teda zdokumentovaný test obnovy je súčasťou validačného balíka, s dôkazmi; a (c) **monitorujú sa periodicky** potom — teda opakovaný, zaznamenaný test obnovy (minimálne ročne a po každej zmene zálohovacieho nástroja).

**7.1 a jeho „throughout the retention period"** je dopredu hľadiaca pasca: GMP retencia teplotných záznamov skladu beží roky, takže musíš vedieť preukázať, že dáta zapísané v roku 2026 sú prístupné, čitateľné a presné aj v roku 2031+ — čo obmedzuje voľbu formátov, cesty upgradu PostgreSQL a odpoveď na otázku, čo sa stane s dátami zákazníka, ak prestaneš podnikať (draft 7.5 bod viii, exit strategy).

### 8. Printouts ✅ (obe celé)

> **8.1** "It should be possible to obtain clear printed copies of electronically stored data."
>
> **8.2** "For records supporting batch release it should be possible to generate printouts indicating if any of the data has been changed since the original entry."

**Potrebuješ report/PDF export**, ktorý je jasný a úplný: teplotný graf + tabuľkové dáta + zoznam odchýlok + identita senzora + obdobie + časová pečiatka generovania + kto ho vygeneroval. **8.2 je tá dôležitá:** ak teplotný záznam skladu podporuje prepustenie šarže QP alebo GDP certifikáciu, výtlačok musí **ukázať, či sa údaje od pôvodného zápisu zmenili** — prakticky musí report zobrazovať upravené/anotované záznamy a niesť indikátor „modified", alebo musí vkladať/odkazovať relevantné záznamy audit trailu. **PDF, ktoré ticho zobrazí iba aktuálne hodnoty, klauzulu 8.2 nespĺňa.**

### 9. Audit Trails ✅ (celá)

> "Consideration should be given, based on a risk assessment, to building into the system the creation of a record of all GMP-relevant changes and deletions (a system generated 'audit trail'). For change or deletion of GMP-relevant data the reason should be documented. Audit trails need to be available and convertible to a generally intelligible form and regularly reviewed."

**Štyri samostatné povinnosti — a pozri si tú slabú formuláciu z roku 2011.** Text hovorí len *„consideration should be given, based on a risk assessment"*. Je to rizikovo založené, nie absolútne. **V praxi je tá mäkkosť mŕtva:** pre GMP-relevantné teplotné dáta má posúdenie rizík len jeden obhájiteľný výsledok, a návrh optionalitu odstraňuje úplne (draft 12.1 hovorí, že systémy *„should have an audit trail functionality"*). Konkrétne:

1. **System-generated** — zapisuje aplikácia, nie používateľ; append-only tabuľka v PostgreSQL, ideálne s odobratím práv UPDATE/DELETE na túto tabuľku pre aplikačnú rolu na úrovni DB.
2. **Zmeny AJ výmazy** — výmaz musí zanechať stopu.
3. **Dôvod zdokumentovaný** — UI musí **vyžadovať** dôvodový reťazec pri každej zmene/výmaze GMP dát (vrátane anotácie odchýlky alebo zmeny prahu). **Nullable stĺpec `reason` je compliance defekt.**
4. **Dostupné, konvertovateľné do všeobecne zrozumiteľnej formy, pravidelne preskúmavané** — čitateľný export pre človeka (nie surové JSON bloby, nie nepriehľadné ID: rozlož user ID na mená a názvy polí na popisky) a **systém musí podporovať review** — prehľadávateľný a filtrovateľný, pretože „regularly reviewed" bez toho nie je možné.

*Čo logovať:* kto, čo (stará hodnota → nová hodnota), kedy (s časovým pásmom), prečo. Surové merania prijaté automaticky vo všeobecnosti nie sú „zmeny" vyžadujúce záznam v audit traile, ale **každá** manuálna interakcia s nimi áno: anotácie, potvrdenia a zdôvodnenia odchýlok, zmeny alarmových prahov, zmeny kalibrácie senzora, zmeny používateľov a rolí, generovanie reportov, konfiguračné zmeny.

### 10. Change and Configuration Management ⚠️

> "Any changes to a computerised system including system configurations should only be made in a controlled manner in accordance with a defined procedure."

Pokrýva **tvoje** releasy aj **konfiguračné zmeny zákazníka**. Potrebuješ zdokumentovanú SOP pre riadenie zmien: posúdenie dopadu → regresné/validačné testovanie primerané dopadu → schválenie → release notes → informovanie zákazníka. Všimni si frázu **„including system configurations"** — zmena alarmového limitu alebo pridanie senzora v inštancii zákazníka **je konfiguračná zmena a musí prejsť jeho riadením zmien**, nie sa urobiť ad hoc.

### 11. Periodic evaluation ✅ (celá)

> "Computerised systems should be periodically evaluated to confirm that they remain in a valid state and are compliant with GMP. Such evaluations should include, where appropriate, the current range of functionality, deviation records, incidents, problems, upgrade history, performance, reliability, security and validation status reports."

Zákazník musí robiť periodický prehľad (typicky ročne, rizikovo založený). **Ty by si mal predávať/dodávať vstupy:** ročný report na zákazníka s históriou upgradov, zoznamom incidentov, štatistikou dostupnosti/výkonu, bezpečnostným stavom (nasadené patche, zraniteľnosti), otvorenými odchýlkami a aktuálnym validačným stavom. Je to lacná opakovaná tržba a presne to, na čo sa návrh rozširuje v dvanásťbodovej kapitole 14.

### 12. Security ✅ (všetky štyri celé)

> **12.1** "Physical and/or logical controls should be in place to restrict access to computerised system to authorised persons. Suitable methods of preventing unauthorised entry to the system may include the use of keys, pass cards, personal codes with passwords, biometrics, restricted access to computer equipment and data storage areas."
>
> **12.2** "The extent of security controls depends on the criticality of the computerised system."
>
> **12.3** "Creation, change, and cancellation of access authorisations should be recorded."
>
> **12.4** "Management systems for data and for documents should be designed to record the identity of operators entering, changing, confirming or deleting data including date and time."

**12.3 sa najčastejšie prehliada:** **samotná správa používateľov sa musí logovať** — vytvorenie účtu, zmena roly, deaktivácia. To je **samostatná logovacia povinnosť odlišná od audit trailu dát podľa klauzuly 9.** Tabuľka `users` s meniteľným stĺpcom `role` a bez histórie klauzulu 12.3 nespĺňa.

**12.4 je klauzula o tom, kto smie meniť dáta** — a všimni si, čo naozaj hovorí: vyžaduje, aby systém **zaznamenal identitu** operátorov zadávajúcich, meniacich, **potvrdzujúcich** alebo mažúcich dáta, **vrátane dátumu a času**. V kombinácii s 12.1 je praktické pravidlo: **žiadne zdieľané účty, žiadne anonymné zápisy, žiadny aplikačný „system" používateľ vykonávajúci zmeny za človeka.** Každá mutácia je pripísateľná menovanej osobe s časovou pečiatkou. Pre PostgreSQL to znamená, že aplikácia musí preniesť identitu koncového používateľa až do audit záznamu — jedno DB pripojenie s jedným DB používateľom je v poriadku, ale *aplikácia* musí zaznamenať človeka. A všimni si slovo **„confirming"** — kontrola druhou osobou podľa klauzuly 6 je sama udalosťou, ktorá musí zaznamenať, kto potvrdil a kedy.

### 13. Incident Management ✅ (celá)

> "All incidents, not only system failures and data errors, should be reported and assessed. The root cause of a critical incident should be identified and should form the basis of corrective and preventive actions."

Potrebuješ incidentný log a CAPA proces ako dodávateľ, a tvoja zmluva/SLA musí definovať **ako a ako rýchlo informuješ zákazníka** o incidente — pretože jeho vlastné GMP riadenie incidentov závisí od toho, že sa to dozvie. Všimni si šírku: **„not only system failures and data errors"** — bezpečnostný incident, nedoručené upozornenie na alarm, výpadok gatewaya aj near-miss sa počítajú. Pre monitorovací produkt je definičná trieda incidentu **„systém nedetegoval alebo neoznámil odchýlku"** a analýza príčin práve tohto musí byť zdokumentovaná.

### 14. Electronic Signature ✅ (celá)

> "Electronic records may be signed electronically. Electronic signatures are expected to:
> a. have the same impact as hand-written signatures within the boundaries of the company,
> b. be permanently linked to their respective record,
> c. include the time and date that they were applied."

Relevantné **len ak softvér podpisy zachytáva** — napr. QA recenzent schvaľujúci mesačný teplotný report alebo vyšetrenie odchýlky. Ak to implementuješ: zviaž podpis kryptograficky alebo referenčne s presnou verziou záznamu (aby ho neskoršia úprava zneplatnila), ukladaj časovú pečiatku s pásmom a zobrazuj celé meno podpisujúceho aj význam podpisu.

Ak e-podpisy **neimplementuješ**, zákazník vytlačí a podpíše perom — **hybridný prístup**, ktorý text z roku 2011 toleruje, ale návrh novej kapitoly 4 výslovne odrádza (`[CH4-DRAFT-2025]` 4.70 ⚠️: *„If records exist electronically such records should be signed electronically. The use of a hybrid system should be avoided."*). **Strategická poznámka: zabudovaný e-podpis je diferenciátor a poisťuje ťa proti návrhu.**

### 15. Batch release ✅ (celá)

> "When a computerised system is used for recording certification and batch release, the system should allow only Qualified Persons to certify the release of the batches and it should clearly identify and record the person releasing or certifying the batches."

**Takmer isto sa na telemetry.digital nevzťahuje.** Systém zaznamenáva skladové podmienky, necertifikuje ani neprepúšťa šarže. **Uveď to výslovne vo validačnej dokumentácii ako rozsahové vylúčenie** („klauzula 15 sa neuplatňuje — systém nevykonáva certifikáciu ani prepúšťanie šarží"). Deklarovať neaplikovateľnosť so zdôvodnením je dobrá prax a šetrí čas auditu. *Nepriama relevancia:* tvoje dáta môžu *podporovať* rozhodnutie QP, čo spúšťa požiadavku klauzuly 8.2 a návrhovej 12.10.

### 16. Business Continuity ✅ (celá)

> "For the availability of computerised systems supporting critical processes, provisions should be made to ensure continuity of support for those processes in the event of a system breakdown (e.g. a manual or alternative system). The time required to bring the alternative arrangements into use should be based on risk and appropriate for a particular system and the business process it supports. These arrangements should be adequately documented and tested."

Monitoring teploty liekového skladu **je** kritický proces — ak monitoring prestane, sklad beží naslepo a stav uložených zásob sa stáva neoveriteľným. Vyžaduje sa: zdokumentovaný záložný režim (kalibrované samostatné dataloggery, alebo manuálne odčítanie v definovaných intervaloch na papierový formulár), **rizikom zdôvodnený čas** na jeho uvedenie do prevádzky a — časť, ktorá chýba najčastejšie — **dôkaz, že bol záložný režim otestovaný.**

Návrhový dôsledok: senzory/gateways by mali **lokálne bufferovať**, aby výpadok servera alebo siete nevytvoril trvalú medzeru; **bufferované dáta doplnené neskôr musia niesť pôvodný čas merania a byť jasne identifikovateľné ako doplnené.** Tvoj hardvér s onboard flash bufferom a backfillom je presne toto — a presne toto je klauzula, ktorou sa to predáva.

### 17. Archiving ✅ (celá)

> "Data may be archived. This data should be checked for accessibility, readability and integrity. If relevant changes are to be made to the system (e.g. computer equipment or programs), then the ability to retrieve the data should be ensured and tested."

Archivačná klauzula z roku 2011 je notoricky tenká (tri vety). Kľúčový operačný spúšťač: **akákoľvek relevantná zmena systému zaväzuje k otestovanej kontrole vyvolateľnosti.** Major upgrade PostgreSQL, migrácia servera alebo zmena schémy si každá vyžaduje zdokumentovaný dôkaz, že archivované dáta sú stále vyvolateľné.

### Čo Annex 11 z roku 2011 vôbec nespomína

Ani raz: zložitosť hesiel, MFA, šifrovanie, firewally, patchovanie, antivírus, penetračné testovanie, oddelenie povinností. **Všetko bezpečnostno-technické v návrhu je ako explicitný text skutočne nové**, hoci inšpektori veľa z toho očakávali pod všeobecnou strechou 12.1/12.2 a podľa PIC/S PI 041-1.

**Mapovanie na architektúru:** klauzula 5 ↔ ingest pipeline; klauzula 6 ↔ formuláre pre prahy a kalibráciu; klauzula 9 + 12.3/12.4 ↔ **dve odlišné append-only log tabuľky** (audit trail dát, a prístupový/administratívny log); klauzula 7.2 ↔ dôkazy o zálohe a obnove; klauzula 16 ↔ lokálny buffer + zdokumentovaný manuálny fallback; klauzula 8.2 ↔ generátor PDF reportov.

⚠️ **Nezískané doslovne:** klauzuly 1 (2. veta), 2 (2. veta), 3.2 (zvyšok), 4.1–4.4, 4.6, 4.7, 10, a celý Glossary z roku 2011 (definície „Application", „Bespoke/Customized computerised system", „Commercial off the shelf software", „IT Infrastructure", „Life cycle", „Process owner", „System owner", „Third Party"). **Pred citovaním stiahni PDF z roku 2011 a over.**

---
## Návrh revidovaného Annexu 11 z júla 2025: čo postaviť teraz, aby si bol pripravený v roku 2028

**Status, ktorý si zapamätaj a opakuj zákazníkom: NIE JE V PLATNOSTI.** Konzultácia bola otvorená **7. júla 2025**, uzavretá **7. októbra 2025 o 23:59 CEST**, stav „Closed", s oznamom *„The response period for this consultation has ended. Thank you for your input."* Konzultačná stránka **neuvádza dátum publikácie finálnych verzií ani dátum účinnosti** ([konzultačná stránka EK](https://health.ec.europa.eu/consultations/stakeholders-consultation-eudralex-volume-4-good-manufacturing-practice-guidelines-chapter-4-annex_en)). Pôvodný harmonogram z koncepčného dokumentu: draft december 2024, pripomienky marec 2025, **prijatie EMA marec 2026, publikácia Európskou komisiou jún 2026, prijatie PIC/S september 2026**. Skutočný postup zaostal: draft 7. júla 2025, konzultácia uzavretá 7. októbra 2025, *„Mid-2026: Expected final publication"*, *„2027: Anticipated implementation date following a transition period"*, s poznámkou, že *„the revision process experienced some delays from the original timeline"*, a očakávaným prechodným obdobím **cca 12–18 mesiacov po publikácii** ([qmsdoc.com](https://qmsdoc.com/2026/01/15/revision-of-pic-s-gmp-annex-11-computerised-systems/)). Revízia je **spoločné úsilie EMA a PIC/S** — *„a joint EMA-PIC/S drafting group to maintain global harmonization of standards"* — takže PIC/S PE 009 Annex 11 bude kopírovať európsky text.

**Záver, ktorý riadi tvoje rozhodovanie: cieľ jún 2026 je zmeškaný; k 21. 9. 2026 je dokument ~3 mesiace po vlastnom revidovanom termíne a ~6 mesiacov po pôvodnom míľniku EMA. Míľnik PIC/S september 2026 tiež zjavne nie je splnený, keďže prijatie PIC/S nasleduje po publikácii EÚ. Reálny termín povinného súladu je 2028, najskôr.**

**Praktický dôsledok: validuj proti Annexu 11 z roku 2011 ako proti záväznému štandardu, ale navrhuj proti návrhu z roku 2025.** Návrh je najjasnejšie dostupné vyjadrenie toho, čo európski GMP inšpektori v praxi už očakávajú (veľa z neho len prepisuje existujúce data-integrity usmernenie PIC/S PI 041-1), a systém postavený podľa návrhu spĺňa text z roku 2011 automaticky. Opačne to neplatí — systém postavený len na minimum z roku 2011 bude treba prerábať. Konkrétne **nemeniteľnosť audit trailu (draft 12.3) sa do existujúcej schémy dodatočne dopĺňa veľmi ťažko.**

⚠️ Nenašlo sa žiadne oficiálne oznámenie EMA, Komisie ani PIC/S z roku 2026, ktoré by po zmeškanom termíne uvádzalo nový dátum publikácie. Neexistuje verejné vysvetlenie oneskorenia. Absencia dokumentu z oficiálnej stránky je najsilnejší dostupný dôkaz. ⚠️ Nepotvrdil sa ani často citovaný rozsah „z 5 na ~19 strán" — žiadny dosiahnuteľný zdroj počet strán neuvádza.

**Zdroj všetkých citácií v tejto sekcii:** [Draft revised Annex 11: Computerised Systems, konzultačná verzia, 7. júla 2025 (PDF)](https://health.ec.europa.eu/document/download/40231f18-e564-4043-94de-c031f813d38b_en?filename=mp_vol4_chap4_annex11_consultation_guideline_en.pdf).

### Dôvody revízie a štruktúra

> "The GMP/GDP Inspectors Working Group and the PIC/S Committee jointly recommended that the current version of Annex 11 on Computerised Systems, be revised to reflect changes in regulatory and manufacturing environments."

> "With an ever-evolving IT landscape, increased use of cloud services, and introduction of new technologies in computerised systems used in GMP activities, there is a growing need for updated guidance."

Revízia má *„clarify requirements and expectations from regulatory authorities, and remove ambiguity and inconsistencies."*

**Sedemnásť kapitol plus Glossary:** 1 Scope · 2 Principles · 3 Pharmaceutical Quality System · 4 Risk Management · 5 Personnel and Training · **6 System Requirements** · **7 Supplier and Service Management** · **8 Alarms** · 9 Qualification and Validation · 10 Handling of Data · **11 Identity and Access Management** · 12 Audit Trails · 13 Electronic Signatures · **14 Periodic Reviews** · **15 Security** · **16 Backup** · 17 Archiving.

Počty pododdielov potvrdené analýzou ECA: **kapitola 12 (Audit Trails) „ten neatly structured subsections"; kapitola 14 (Periodic Reviews) „twelve subsections"; kapitola 7 „nine subsections"** ([ECA Academy, *Annex 11 Draft - First Analysis*](https://www.gmp-compliance.org/gmp-news/annex-11-draft-first-analysis)). Vlastné načítanie kapitoly 15 vrátilo **20 očíslovaných pododdielov 15.1–15.20**.

**Číslovanie 2011 → draft sa nedá mapovať 1:1** — prečíslovanie je totálne. Približné mapovanie pre tvoju maticu sledovateľnosti: 1→4; 2→5; 3→7; 4→6+9; 5→10.2; 6→10.1; 7.1→15/17; 7.2→16; 8→(rozptýlené; 12.9); 9→12; 10→3 (zmeny); 11→14; 12→11+15; 13→3 (odchýlky); 14→13; 15→(bez priameho ekvivalentu); 16→15.7; 17→17. **Bez predchodcu z roku 2011 sú: kapitola 8 (Alarms) a väčšina kapitoly 15 (Security).**

### Kapitola 1 Scope a kapitola 2 Principles

> **1** "This annex applies to all types of computerised systems used in the manufacturing of medicinal products and active substances."

Všimni si zúženie rozsahu na **výrobu**. Distribučný sklad je pod GDP.

> **2.1** *Lifecycle management:* "Computerised systems should be validated before use and maintained in a validated state throughout their lifecycle."
>
> **2.2** *Quality Risk Management:* "Quality Risk Management (QRM) should be applied throughout all lifecycle phases of a computerised system used in GMP activities."
>
> **2.3** *Alternative practices:* "Practices which constitute alternatives to the activities required in this document may be used, if they have been proven and documented to provide the same or higher level of control."
>
> **2.4** *Data integrity:* "It is critically important that data captured, analysed and reported by systems used in GMP activities are trustworthy."
>
> **2.5** *System requirements:* "System requirements which describe the functionality the regulated user has automated and is relying on when performing GMP activities, should be documented and kept updated."
>
> **2.6** *Outsourced activities:* "When using outsourced activities, the regulated user remains fully responsible for adherence to the requirements included in this document."
>
> **2.7** *Security:* "Regulated users should keep updated about new security threats to GMP systems, and measures to protect these should be implemented and improved in a timely manner."
>
> **2.8** *No risk increase:* "Where a computerised system replaces another system or a manual operation, there should be no resultant decrease in product quality, patient safety or data integrity."

**Klauzula 2.3 je pre teba mimoriadne cenná a treba si ju zapamätať doslova.** Výslovne pripúšťa alternatívne postupy, ak sú **preukázané a zdokumentované** ako poskytujúce rovnakú alebo vyššiu úroveň kontroly. Jednotlivec, ktorý nemôže robiť klasické štyri oči pri code review, môže cez 2.3 obhájiť zdokumentovanú alternatívu (automatizované testovacie brány + periodická externá revízia kódu). **Toto je klauzula, na ktorú sa odvolávaš vždy, keď ťa dodávateľský dotazník tlačí do organizačného opatrenia, ktoré jednoosobová firma fyzicky nevie splniť.**

### Kapitola 3 — Pharmaceutical Quality System (celkom nová)

> **3.1** "A regulated user should implement a pharmaceutical quality system (PQS), which covers all computerised systems used in GMP activities and personnel involved with these."

Podriadené požiadavky:

> *Deviations:* "All deviations occurring during validation or operation of computerised systems are recorded and any significant deviations investigated with the objective of determining the root cause."
>
> *Changes:* "Any change to a computerised system including but not limited to its configuration, its hardware and software components, and its platform and operating system, are made in a controlled manner."
>
> *Internal audits:* "Internal audits are planned, conducted, reported and followed up on to detect procedural deviations and ensure product quality, patient safety and data integrity."
>
> *Management reviews:* "Regular management reviews cover relevant performance indicators for the computerised system and the process it is used in (quality metrics)."
>
> *Senior management oversight:* "Senior management effectively oversee the state of control throughout the system lifecycle, allocate appropriate resources, and implement a culture that promotes data integrity."

### Kapitola 4 — Risk Management

> **4.1** "Quality Risk Management (QRM) should be applied throughout the lifecycle of a computerised system considering any possible impact on product quality, patient safety or data integrity."
>
> **4.2** "Risks associated with the use of computerised systems in GMP activities should be identified and analysed according to an established procedure."
>
> **4.3** "The validation strategy and effort should be determined based on the intended use of the system and potential risks to product quality, patient safety and data integrity."
>
> **4.4** "Where applicable, risks associated with the use of computerised systems in GMP activities should be mitigated and brought down to an acceptable level, if possible, by modifying processes or system design."
>
> **4.5** "Quality risk management principles should be used to assess the criticality of data to product quality, patient safety and data integrity."

### Kapitola 5 — Personnel and Training

> **5.1** "When conducting the activities required in this document, there should be, where applicable, close cooperation between all relevant parties."
>
> **5.2** "All parties involved with computerised systems used in GMP activities should have adequate system specific training, and appropriate qualifications and experience."

### Kapitola 6 — System Requirements (nová; ECA: *„Almost one page is reserved for requirements specifications…which are often neglected in practice"*)

> **6.1** "A regulated user should establish and approve a set of system requirements (e.g. a User Requirements Specification, URS), which accurately describe the functionality the regulated user has automated and is relying on when performing GMP activities."
>
> **6.2** "The extent and detail of defined requirements should be commensurate with the risk, complexity and novelty of a system, and the description should be sufficient to support subsequent risk analysis, specification, design, purchase, configuration, qualification and validation."
>
> **6.3** "The regulated user should carefully review and approve the document and consider whether the system fulfils GMP requirements and company processes as is, or whether it should be configured or customised."
>
> **6.4** "Requirements should be updated and maintained throughout the lifecycle of a system to ensure that they continue to give a complete and accurate description of system functionality."
>
> **6.5** "Documented traceability between individual requirements, underlaying design specifications and corresponding qualification and validation test cases should be established and maintained."
>
> **6.6** "It should be clear what functionality, if any, is modified or added by configuration of a system. Options allowing configuration of system functionality should be described in the requirements specification."

**6.5 robí z matice sledovateľnosti požiadaviek fakticky povinnosť.** Udržiavaj živú maticu URS → návrhová špecifikácia → testovací prípad, aktualizovanú podľa 6.4. **Je to najcennejší jediný artefakt, aký môžeš zákazníkom dodať.**

### Kapitola 7 — Supplier and Service Management: deväť povinných prvkov zmluvy

⚠️ Klauzuly 7.1–7.4 boli fetchom skrátené; substancia je spoľahlivá, znenie nie:

> **7.1** ⚠️ "[…] the regulated user remains fully responsible for these activities"
>
> **7.2** ⚠️ audity požadované "according to risk and system criticality" na posúdenie postupov dodávateľa
>
> **7.3** ⚠️ "[…] effective oversight […] according to defined service level agreements (SLA) and key performance indicators (KPI)"
>
> **7.4** ⚠️ "[…] documentation for activities […] should be accessible and can be explained from their facility"

**7.5 — zmluva/dohoda musí pokryť deväť prvkov, i–ix** ⚠️ (body vii a viii boli na konci skrátené):

> **i.** "Describes the activities and documentation to be provided"
>
> **ii.** "Establishes the company procedures and regulatory requirements to be met"
>
> **iii.** "Agrees on regular, ad hoc and incident reporting and oversight"
>
> **iv.** "Agrees on conditions for supplier audits"
>
> **v.** "Agrees on support during regulatory inspections, if so requested"
>
> **vi.** "Agrees on resolution of issues brought up during normal operation"
>
> **vii.** "Defines requirements and processes for communication of quality and security [issues/events — text truncated]"
>
> **viii.** "Defines an exit strategy by which the regulated user may retain control [of data — text truncated]"
>
> **ix.** "Agrees on the process for release of new system versions"

**Toto je tvoja zmluvná osnova.** Napíš **jednu dvojjazyčnú (SK/EN) štandardnú zmluvu o kvalite, ktorá pokrýva všetkých deväť bodov už teraz** — prežije revíziu a odlíši ťa od konkurencie, ktorá žiadnu nemá.

### Kapitola 8 — Alarms: kapitola, ktorá znie ako špecifikácia tvojho produktu

⚠️ Všetky klauzuly 8.x boli fetchom skrátené; substancia spoľahlivá, znenie over.

> **8.1** ⚠️ "Alarms should be implemented […] where the user must take a specific action"
>
> **8.2** ⚠️ "Setting, changing or deactivation should only be available to users with appropriate access [privileges]"
>
> **8.3** ⚠️ "Alarms should set off visible and/or audible signals when set alarm limits [are] exceeded"
>
> **8.4** ⚠️ "Critical alarms […] should only be acknowledged by users with appropriate access privileges"
>
> **8.5** ⚠️ "All alarms and acknowledgements should be automatically added to an alarm log"
>
> **8.6** ⚠️ "Alarm logs should be searchable and sortable in the originating system"
>
> **8.7** ⚠️ "Alarm logs should be subject to appropriate periodic reviews based on approved procedures"

ECA poznamenáva, že kapitola 8 zavádza *„non-erasable/deactivatable record (log) with a corresponding annotation, similar to an audit trail"* ([ECA Academy](https://www.gmp-compliance.org/gmp-news/annex-11-draft-first-analysis)).

**Táto kapitola je v podstate napísaná pre systémy monitoringu teploty. Prečítaj si ju ako backlog:**

- **8.2** → alarmové limity sú privilegovaná konfigurácia; iba oprávnená rola smie nastaviť, zmeniť alebo **deaktivovať**; každá takáto akcia potrebuje audit trail + dôvod (prepojenie na klauzulu 6 a 12.4).
- **8.3** → **vizuálna a/alebo zvuková** signalizácia. E-mail/SMS/push samy nemusia stačiť; očakáva sa viditeľný alarmový stav na dashboarde.
- **8.4** → **potvrdenie alarmu je privilegované a osobné** — žiadne zdieľané tlačidlo „ack", žiadne potvrdzovanie z neautentifikovaného odkazu v e-maile. Toto je bežná návrhová skratka, ktorá by na 8.4 neprešla.
- **8.5** → **alarmový log ako samostatný artefakt oddelený od audit trailu**: každý vyvolaný alarm a každé potvrdenie automaticky zaznamenané. Append-only.
- **8.6** → alarmový log musí byť **prehľadávateľný a triediteľný priamo v pôvodnom systéme**, nie iba v exportovanom CSV. To je priama požiadavka na UI.
- **8.7** → periodický prehľad alarmových logov — ďalší vstup, ktorý vieš pre zákazníka zabaliť ako službu.
- **Implicitne:** **deaktivácia alarmu je výslovne predpokladaná a musí byť logovaná** — takže „stíšené"/„potlačené" senzory potrebujú prvotriedny, logovaný, časovo ohraničený stav, nie skrytý konfiguračný flag.

### Kapitola 9 — Qualification and Validation

> **9.1** "Qualification and validation activities for computerised systems should follow the general principles outlined in GMP Annex 15. The activities should address both standard and configured system functionality, as well as any functionality realised through customisation."
>
> **9.2** "Computerised systems should be qualified and validated in accordance with the principles of quality risk management. Decisions on the scope and extent of qualification and validation of specific functionality and entire systems should be based on a justified and documented risk assessment."
>
> **9.3** "Prior to commencing any test activity, it should be verified that a computerised system and its components have been correctly installed and configured according to specifications."
>
> **9.4** "System qualification and validation should provide evidence in the form of executed test scripts, and where relevant, screen dumps, that requirements, and where applicable, derived functional specifications, are met by the system."
>
> **9.5** "Test cases should be traceable to individual requirements or specifications, e.g. by means of a requirements traceability matrix."
>
> **9.6** "Increased focus should be on testing a system's handling of key functional requirements, on functionality intended to ensure that activities are conducted according to GMP, and on functionality designed to ensure data integrity."
>
> **9.7** "Qualification and validation activities should be conducted according to approved plans, protocols and test scripts. Test scripts should be described in sufficient detail to ensure a correct and repeatable conduct of test steps and prerequisites."
>
> **9.8** "Qualification and validation activities should be successfully completed and reported prior to approval and taking a system into use."
>
> **9.9** "The regulated user is fully accountable and should carefully review and authorise the use of the documentation. They should carefully consider whether it covers the implemented version and supports GMP, and company processes as is."

**9.1 explicitne odkazuje na Annex 15** — nové a záväzné pre metodiku. **9.4 „screen dumps"** znamená, že snímky obrazovky sa v záznamoch o vykonaní testov očakávajú. **9.9 je mierené na dodávateľské validačné balíky:** zákazník musí dokumentáciu preskúmať a autorizovať a **nemôže zodpovednosť outsourcovať na teba** — povedz to v predajnom materiáli rovno, aby boli očakávania správne. Paradoxne to predáva lepšie než sľub „postaráme sa o všetko".

### Kapitola 10 — Handling of Data

> **10.1** "Where critical data is entered manually, systems should, were applicable, have functionality to verify the plausibility of the inputs (e.g. within expected ranges), and alert the user when the input is not plausible." *(„were" je v návrhu preklep namiesto „where")*
>
> **10.2** "Where a routine work process requires that critical data be transferred from one system to another (e.g. from a laboratory instrument to a LIMS system), this should, where possible, be based on validated interfaces rather than on manual transcriptions."
>
> **10.3** "Where an ad hoc process requires that critical data or a whole database be migrated from one system to another (e.g. when moving data from a retired to a new system), this should be based on a validated process."
>
> **10.4** "Where applicable, critical data should be encrypted on a system."

**Veľká zmena oproti 2011:** klauzula 6 z roku 2011 žiadala **kontrolu druhou osobou alebo validovanými elektronickými prostriedkami**; draft **10.1 presúva ťažisko na zabudovanú plauzibilitnú/rozsahovú validáciu s upozornením používateľa** — teda na softvérovú funkciu, nie na procedúru. **10.4 zavádza šifrovanie**, ktoré v roku 2011 chýbalo úplne: implementuj šifrovanie v pokoji (šifrovanie disku/volume pre PostgreSQL alebo na úrovni stĺpcov pre kritické polia) aj pri prenose (TLS všade, vrátane senzor→gateway→ingest).

### Kapitola 11 — Identity and Access Management (nová)

⚠️ Všetky skrátené.

> **11.1** ⚠️ "All users should have unique and personal accounts. The use of shared accounts […] constitute[s] a violation"
>
> **11.2** ⚠️ "User accesses and roles should be granted, modified and revoked […] in a timely manner"
>
> **11.3** ⚠️ "The method of authentication should identify users with a high degree of certainty"
>
> **11.4** ⚠️ "Passwords […] should be changed at the first login, preferably required by the system"
>
> **11.5** ⚠️ "Passwords should be secure and enforced by systems […] contain a combination of uppercase, lowercase [, numbers and special characters]"
>
> **11.6** ⚠️ "Remote authentication on critical systems […] should include multifactor authentication (MFA)"
>
> **11.7** ⚠️ "Accounts should be automatically locked after a pre-defined number of successive failed [login] attempts"
>
> **11.8** ⚠️ "Systems should include an automatic inactivity logout […] The user should not be able to change [the timeout setting]"
>
> **11.9** ⚠️ "Systems should include an access log […] which, for each login, automatically logs the username [, date, time]"
>
> **11.10** ⚠️ *Guiding principles:* "Segregation of duties, i.e. that users […] do not have administrative privileges" / "Least privilege principle, i.e. that users do not have higher access privileges than necessary"
>
> **11.11** ⚠️ "User accounts should be subject to recurrent reviews where managers confirm continued access [need]"

ECA dodáva, že kapitola 11 *„restricts smart card authentication and emphasizes segregation of duties"*.

**Konkrétny feature checklist pre telemetry.digital:** unikátne osobné účty (11.1); životný cyklus používateľov s včasným odobratím prístupu (11.2); vynútená zmena hesla pri prvom prihlásení (11.4); zložitosť hesiel **vynútená systémom** (11.5); **MFA pre vzdialený prístup ku kritickým systémom (11.6)** — pri cloudovom/SaaS produkte je *každý* prístup vzdialený, takže MFA je fakticky povinné, a TOTP, ktoré už máš, to pokrýva; zablokovanie účtu po N neúspešných pokusoch (11.7); **automatické odhlásenie pri nečinnosti, ktoré používateľ nesmie zmeniť (11.8)** — musí to byť nastavenie prístupné len administrátorovi; **prístupový log oddelený od audit trailu aj od alarmového logu (11.9)**; návrh rolí presadzujúci oddelenie povinností (11.10); a podpora periodického prehľadu prístupov (11.11).

**11.10 je pre teba najťažšia položka v celom návrhu** a rieši sa v sekcii o dodávateľských dotazníkoch nižšie.

### Kapitola 12 — Audit Trails: najväčšia jediná zmena oproti roku 2011

⚠️ Skrátené.

> **12.1** ⚠️ "Systems […] should have an audit trail functionality which automatically logs all manual user interactions"
>
> **12.2** ⚠️ "The audit trail should unambiguously capture the user […] what was changed […] when the change was made [and why]"
>
> **12.3** ⚠️ "Audit trail functionality should be enabled and locked at all times […] should not be possible for any user to edit [or disable it]"
>
> **12.4** ⚠️ "Systems should accommodate effective and efficient reviews of audit trail data […] sort and search"
>
> **12.5** ⚠️ "Audit trail reviews should be conducted according to a documented procedure […] Appropriate action [should be] taken"
>
> **12.6** ⚠️ "Audit trail reviews should be conducted by personnel not directly involved in the activities reviewed"
>
> **12.7** ⚠️ "Reviews should be targeted, based on risk […] focus on detecting any deliberate or indeliberate changes"
>
> **12.8** ⚠️ "Audit trail reviews should be conducted in a timely manner […] prior to batch release, unless justified"
>
> **12.9** ⚠️ "It should be possible to obtain a complete electronic copy […] **Flat and locked files are not acceptable**"
>
> **12.10** ⚠️ "Audit trail reviews with direct impact on the release of a product should be available to the QP"

**Klauzula 9 z roku 2011 hovorila „consideration should be given, based on a risk assessment". 12.1 z toho robí plochú požiadavku pokrývajúcu „all manual user interactions".** Návrhové dôsledky:

- **12.3 — „enabled and locked at all times […] not possible for any user to edit"**: audit trail musí byť **technicky nemeniteľný vrátane voči administrátorom**. V PostgreSQL: odober aplikačnej role UPDATE/DELETE na audit tabuľke, použi triggery alebo append-only vzor, ideálne hash-chain riadkov, aby bola manipulácia detekovateľná. **Nesmie existovať konfiguračný prepínač na vypnutie audit trailu — ani skrytý.**
- **12.4 + 12.9 — audit trail musí byť preskúmateľný v systéme (triedenie/hľadanie) AJ exportovateľný ako „complete electronic copy"**, pričom **„Flat and locked files are not acceptable"** znamená, že **zamknutý PDF export požiadavku NESPĹŇA.** Musíš poskytnúť **štruktúrovaný, strojovo spracovateľný export (CSV/XML/JSON), ktorý ostáva triediteľný a prehľadávateľný u príjemcu.** Toto je priama, testovateľná produktová požiadavka a bežný bod zlyhania.
- **12.6 — „conducted by personnel not directly involved in the activities reviewed"** — toto je ten „nezávislý recenzent". Je to organizačná požiadavka na **zákazníka**, ale tvoj softvér ju musí **podporovať**: rola recenzenta s čítacím prístupom k audit trailom bez možnosti meniť podkladové dáta, a spôsob zaznamenať, že prehľad prebehol, kým, kedy a s akým výsledkom. **Zabudovaný workflow „audit trail review" je silný diferenciátor** — väčšina konkurentov nechá zákazníka riešiť to v Exceli.
- **12.7 — cielené, rizikovo založené prehľady** namiesto čítania všetkého: filtrovanie podľa rizikovo relevantných typov udalostí (zmeny prahov, výmazy, anotácie odchýlok, zmeny oprávnení) musí byť prvotriedna funkcia.

### Kapitola 13 — Electronic Signatures ✅ (vrátené ako celé vety)

> **13.1** *Scope:* "Requirements for electronic signatures in this document apply to systems and tools used in processes where GMP require a signature."
>
> **13.2** *Open systems:* "Where the system owner does not have full control of system accesses (open systems), or where required by other legislation, electronic signatures should, in addition, meet applicable national and international requirements, such as trusted services."
>
> **13.3** *Re-authentication:* "When executing an electronic signature, a system should enforce users to perform a full re-authentication providing at least the same level of security as during system login."
>
> **13.4** *Date and time:* "Systems should automatically log the date and time and, where applicable, the time zone when an electronic signature was applied."
>
> **13.5** *Meaning:* "It should be clear when a user is executing an electronic signature and where applicable, systems should prompt the user for the meaning of the signature (e.g. reviewer or approver)."
>
> **13.6** *Manifestation:* "When an electronic signature is displayed (on screen or print), the manifestation should include the full name of the user, the username, where applicable the role of the signer and the meaning of the signature, the date and time, and where needed the time zone, when the signature was applied."
>
> **13.7** *Indisputability:* "Electronic signatures should be indisputable and equivalent to hand-written signatures."
>
> **13.8** *Unbreakable link:* "Electronic signatures should be permanently linked to their respective records. Controls should be in place to ensure that a signed record cannot be modified or alternatively, that if a later change is made to a signed record, it will clearly appear as unsigned."
>
> **13.9** *Hybrid solution:* "If a wet-ink signature (on paper) is used to sign electronic records held in a computerised system (a hybrid solution), measures should be implemented to provide a high degree of certainty that any change to the electronic record will invalidate the signature."

**13.2 spája otvorené systémy s eIDAS trusted services** — relevantné, ak je telemetry.digital multi-tenant SaaS, kde prístupy kontroluješ ty, nie zákazník; to z neho v zmysle annexu pravdepodobne robí **otvorený systém** a pritiahne to požiadavky na kvalifikovaný podpis podľa eIDAS. **13.3 vyžaduje plnú re-autentifikáciu v momente podpisu** (nestačí aktívna relácia). **13.8** vyžaduje, aby neskoršia zmena spôsobila, že záznam **„clearly appear as unsigned"** — technicky sa to najlepšie rieši hashom podpísanej verzie záznamu.

Sekundárny komentár k tomu istému návrhu popisuje efekt takto ⚠️ (autorská parafráza, nie text návrhu): prvý podpis v relácii = plná re-autentifikácia, ďalšie podpisy = heslo/biometria; podpisovanie len čipovou kartou neprijateľné; ak sa záznam po podpise zmení, podpis sa anuluje/označí; systém sa musí pýtať na účel podpisu; časové pásmo sa musí zachytiť ([Investigations of a Dog, 29. 7. 2025](https://investigationsquality.com/2025/07/29/draft-annex-11-section-13-what-the-proposed-electronic-signature-rules-mean/)).

### Kapitola 14 — Periodic Reviews (12 pododdielov)

> **14.1** ⚠️ "After a system [is taken into use], periodic reviews should be conducted […] verify whether the system remains 'fit for intended use'"
>
> **14.2** ⚠️ rozsah zahŕňa body i–iii o zmenách hardvéru, softvéru, dokumentácie a konfigurácie; body iv–xii o nadväzných procesoch vrátane **prehľadov audit trailov, bezpečnostných incidentov a súladu** *(znenie jednotlivých bodov sa nezískalo)*
>
> **14.3** ⚠️ "The frequency of reviews should be established and justified based on the risk the system poses"

### Kapitola 15 — Security (20 pododdielov, najväčšia a takmer celá nová)

⚠️ Všetky citácie 15.x boli skrátené na elipsách; substancia spoľahlivá, presné znenie over.

> **15.1** "Regulated users should ensure an effective information security management system is implemented"
> **15.2** "Regulated users should keep updated about new security threats"
> **15.3** "Regulated users should undergo recurrent security awareness training"
> **15.4** "Servers, computers, devices […] should be physically protected against unauthorised access"
> **15.5** "Data centres should be constructed to minimise the risk […] natural and manmade disasters"
> **15.6** "Where relevant, critical data should be replicated […] automatically with a delay which is short enough"
> **15.7** "A disaster recovery plan should be in place, tested and available […] within a defined Recovery Time Objective"
> **15.8** "Networks should be segmented, and effective firewalls implemented […] control incoming and outgoing traffic"
> **15.9** "Firewall rules should be periodically reviewed […] firewalls continue to be set as tight as possible"
> **15.10** "Operating systems and platforms […] should be updated in a timely manner according to vendor recommendations"
> **15.11** "Validation of applications on updated operating systems […] should be planned and completed in due time"
> **15.12** "Applications on […] operating systems […] no longer supported […] should be isolated from computer networks"
> **15.13** "Relevant security patches […] should be deployed in a timely manner […] For critical vulnerabilities, immediately"
> **15.14** "Applications on operating systems […] not security patched in a timely manner […] constitute a major risk"
> **15.15** "The use of bidirectional devices […] should be strictly controlled within the organisation"
> **15.16** "Bidirectional devices […] should not be used unless […] effectively scanned and found to be harmless"
> **15.17** "Ports for bidirectional devices […] should be deactivated by default, blocked or even removed"
> **15.18** "Anti-virus software should be installed and activated […] continuously updated with most recent virus definitions"
> **15.19** "For critical systems facing the internet, penetration testing […] should be performed at regular intervals"
> **15.20** "When remotely connecting […] a secure and encrypted protocol should be used"

ECA zhŕňa kapitolu 15 ako *„firewalls, disaster recovery - RTO/RPO, patches, virus protection"* a povinné *„penetration tests for critical systems"*.

**Pre teba najnáročnejšia kapitola.** **15.6/15.7** — **RTO a RPO musia byť definované a plán obnovy po havárii otestovaný.** Pre cloudovú monitorovaciu službu to znamená zdokumentované DR runbooky a dôkaz o vykonanom DR teste, nie len „máme zálohy". **15.10–15.14** — reálna patchovacia disciplína: včasné patchovanie OS/platformy, **okamžité nasadenie pri kritických zraniteľnostiach (15.13)**, plánovaná revalidácia po aktualizácii OS (15.11), izolácia všetkého, čo beží na EOL platformách (15.12). Pre Go/PostgreSQL stack: sleduj CVE závislostí (`govulncheck`), sleduj EOL dátumy PostgreSQL a zdokumentuj patchovací proces. **15.19** — **pravidelné penetračné testovanie kritických systémov prístupných z internetu** je nová opakovaná externá nákladová položka, ktorú treba rozpočtovať — a dá sa predať ako signál dôveryhodnosti.

**Komerčný dôsledok 15.1 („information security management system"):** je to fakticky očakávanie tvaru ISO 27001. **Certifikácia ISO 27001 alebo zdokumentovaný ISMS mapovaný na ňu by ti umožnili odpovedať na otázky kapitoly 15 aj kapitoly 7 jedným ťahom** — a zároveň by vyrovnali argument LogHubu, ktorý ISO 27001 deklaruje.

### Kapitola 16 — Backup ✅ (celé vety)

> **16.1** "Data and metadata should be regularly backed up following established procedures to prevent the loss of data in case of accidental or deliberate change or deletion, loss as the result of a malfunction or corruption, e.g. as the result of a cyber-attack."
>
> **16.2** "The frequency, retention period and storage of backups is critically important to the effectiveness of the process to mitigate the loss of data. Backups should be made at suitable intervals (e.g. hourly, daily, weekly and monthly) and their retention determined through a risk-based approach (e.g. correspondingly a week, a month, a quarter, and years)."
>
> **16.3** "Backups should be physically separated from the server or computer holding the original data and stored at a safe distance from this, to prevent that both would be impacted by the same incident."
>
> **16.4** "Backups should not be stored at the same logical network as the original data to avoid simultaneous destruction or alteration."
>
> **16.5** "Depending on the criticality and urgency for recovery after an incident, applications and system configurations may also need to be backed up."
>
> **16.6** "Restore of data from backup should be tested and documented based on risk during system validation and after changes are made to the backup or restore processes and tools."

**16.2 dáva explicitný príklad grandfather-father-son rozvrhu** (hodinové/denné/týždenné/mesačné zálohy uchovávané týždeň/mesiac/štvrťrok/roky) — obhájiteľná predvolená hodnota, ktorú treba implementovať. **16.3 + 16.4 sú spolu protiransomvérová požiadavka:** zálohy musia byť oddelené **fyzicky aj logicky** — záloha na tom istom cloudovom účte/VPC ako produkčná databáza **16.4 nespĺňa.** Použi samostatný účet/poskytovateľa/región so samostatnými prihlasovacími údajmi, ideálne immutable/object-lock úložisko. **16.5** rozširuje rozsah zálohy na **kód aplikácie a systémovú konfiguráciu**, nielen dáta.

### Kapitola 17 — Archiving ✅ (celé vety; rozšírené z troch viet na päť pododdielov)

> **17.1** "After completion of a process, e.g. release of a product, GMP data and metadata (incl. audit trails) should be protected from deletion and changes throughout the retention period."
>
> **17.2** "When moving GMP data and metadata from one location to another in a system, or to a dedicated archival system, the integrity of the data should be verified by a high degree of certainty before any data is deleted, e.g. by means of a checksum."
>
> **17.3** "If data is archived on a server (disk), it should be regularly backed up following the same procedures as for live data (see 16 Backup)."
>
> **17.4** "If data is archived long-term on volatile storage media with limited durability (e.g. CD), this should follow a validated process."
>
> **17.5** "It should be possible to retrieve archived data and metadata in a format which allows searching and sorting of the data, or alternatively, to allow export of the data to a tool where this is possible."

ECA: kapitola 17 rieši archiváciu dát komplexne **po prvýkrát v GMP regulácii**.

**17.1 výslovne zahŕňa audit trails do toho, čo sa musí archivovať a chrániť** — audit trail sa uchováva tak dlho ako dáta, ktoré opisuje. **17.2 vyžaduje overenie integrity typu checksum pred akýmkoľvek výmazom počas presunu** — priamo aplikovateľné na tierovanie dát (presun starých partícií do chladného úložiska) aj na migrácie zákazníkov. **17.5 vyžaduje, aby archivované dáta ostali prehľadávateľné a triediteľné** — archivácia do kopy statických PDF alebo plochých súborov teda neprejde.

### Glossary návrhu ✅ (celé)

> **ALCOA+:** "An acronym for 'attributable, legible, contemporaneous, original and accurate', which puts additional emphasis on the attributes of being complete, consistent, enduring and available."
>
> **Audit trail:** "In computerised systems, an audit trail is a secure, computer generated, time-stamped electronic record that allows reconstruction of the events relating to the creation, modification, or deletion of an electronic record."
>
> **Backup:** "Provisions made for the recovery of data files or software, for the restart of processing, or for the use of alternative computer equipment after a system failure or disaster."
>
> **Electronic record:** "Any combination of text, graphics, data, audio, pictorial, or other information representation in digital form that is created, modified, maintained, archived, retrieved, or distributed by a computer system."
>
> **Validation:** "Action of proving that a process leads to the expected results. Validation of a computerised system requires ensuring and demonstrating the fitness for its purpose."

### AI, Annex 22 a cloud

Návrh Annexu 11 **sám o sebe neuvádza Annex 22 ani umelú inteligenciu** v získanom texte. AI rieši **samostatný nový návrh Annexu 22 (Artificial Intelligence)** publikovaný v tom istom konzultačnom balíku ([draft Annex 22 PDF](https://health.ec.europa.eu/document/download/5f38a92d-bb8e-4264-8898-ea076e926db6_en?filename=mp_vol4_chap4_annex22_consultation_guideline_en.pdf)) — jeho obsah nebol skúmaný. **Cloud sa výslovne spomína raz**, v úvode; inak je riešený nepriamo, ale všadeprítomne — cez kapitolu 7 (SLA/KPI, exit strategy) a kapitolu 15 (dátové centrá, DR/RTO, segmentácia siete, vzdialený prístup). **Integrita dát nemá jednu vyhradenú kapitolu**; je pretkaná ako základný princíp (2.4, 3.1, 4.5, 9.6 a celé kapitoly 11/12/16/17).

---

## Návrh revidovanej Chapter 4 (Documentation): čo sa mení pri elektronických záznamoch

**Zdroj:** [Draft revised Chapter 4 Documentation (PDF)](https://health.ec.europa.eu/document/download/fa336a6e-753b-46fc-b53b-9178bacd8878_en?filename=mp_vol4_chap4_consultation_guideline_en.pdf). **NIE JE V PLATNOSTI.** Oficiálny index EudraLex stále uvádza „Chapter 4 - Documentation (January 2011)".

Štruktúra návrhu: 1 Principle (4.1–4.9) · 2 Data Governance Systems (4.10–4.18) · 3 Risk Management (4.19–4.22) · 4 General Requirements (4.23–4.26) · 5 Master Documents (4.27–4.48) · 6 Generation and Control (4.49–4.55) · 7 Good Documentation Practice (4.56–4.63) · 8 Signatures (4.64–4.75) · 9 Retention (4.76–4.79) · 10 Data Integrity (4.80–4.81) · 11 Hybrid Systems (4.82–4.85) · 12 Glossary.

> **4.7** "Documentation may exist in a variety of forms, including paper-based, electronic, or other means (e.g. photography, imagery, video and audio recordings)."
>
> **4.8** "Whether documents are created, stored, and managed electronically, paper based, by other means or through a hybrid system, they must meet the same GMP requirements for legibility, accuracy, integrity, and completeness throughout the whole lifecycle."
>
> **4.26** "Data which is recorded or processed electronically should not be converted to or stored in a paper form unless it meets the requirements set out in section 13 'hybrid systems' or the conversion is validated or verified for accuracy."
>
> **4.70** "If records exist electronically such records should be signed electronically. The use of a hybrid system should be avoided."
>
> **4.36** "Where a validated process is continuously monitored and controlled, then automatically generated reports may be limited to compliance summaries and exception/out-of-specification (OOS) data reports."

Raw data a metadáta:

> **4.27** "Records include the raw data which is used to generate other records. For electronic records regulated users should define which data are to be used as raw data. At least, all data on which quality decisions are based should be defined as raw data."
>
> **4.12** "For derived data the traceability which allows reconstruction of all data processing activities should be maintained."
>
> *Glossary — Raw data:* "Raw data is defined as the original record (data) which can be described as the first capture of stored information, whether recorded on paper or electronically."
>
> *Glossary — Complete electronic record:* "A complete record of data generated electronically includes relevant metadata."
>
> *Glossary — True copy:* "An exact copy of original documentation that preserves the same content, meaning and attributes of the original. The term 'true copy' is synonymous with 'certified' or 'verified copy'."
>
> **4.58** "Recorded media should be durable throughout the retention period. If this is not feasible, then true copies should be generated. For this case a documented system should be in place to verify and record the integrity of the copy."

Retencia:

> **4.77** "Batch documentation which must be kept for one year after expiry of the batch to which it relates or at least five years after certification of the batch by the Qualified Person, whichever is the longer."
>
> **4.77** (IMP) "For investigational medicinal products, the batch documentation must be kept for at least five years after the completion or formal discontinuation of the last clinical trial in which the batch was used."
>
> **4.78** "For other types of documentation, the retention period will depend on the business activity which the documentation supports. Critical documentation, including raw data (for example relating to validation or stability), which supports information in the Marketing Authorisation should be retained whilst the authorization remains in force."
>
> **4.79** "A documented process for the disposal of records should be in place to ensure that the correct original records or true copies are disposed only after the defined retention period."

Hybridné systémy:

> **4.82** "They should be clearly defined and identified, and each contributing element of the system validated and controlled according to risk management principles."
>
> **4.83** "A detailed description of the entire system should be available. The description should outline all major components, their functions, and interactions with each other as well as control for data management and data integrity." … "Procedures and records should be available to manage and appropriately control the interface between manual and computerised systems."
>
> **4.85** "Procedures should be in place to manage the review of data generated by hybrid systems which clearly outline the process for the evaluation, approval and archiving of electronic and paper-based data."

**Štyri veci, ktoré z toho pre teba priamo plynú.**

**4.27 je priamo použiteľná:** zákazník musí **definovať, ktoré telemetrické dáta sú raw data**. Pre systém monitoringu teploty je raw data **prvé zachytené meranie tak, ako prišlo zo senzora** — nie spriemerovaná/podvzorkovaná krivka na dashboarde a nie PDF report. **Návrhový dôsledok: nikdy neprepisuj ani nezahadzuj pôvodné meranie.** Ak podvzorkuješ alebo agreguješ kvôli výkonu, pôvodná séria musí zostať uchovaná a vyvolateľná, a **4.12** vyžaduje sledovateľnosť umožňujúcu rekonštrukciu všetkého spracovania (teda aj agregačná logika a jej parametre musia byť zdokumentované a reprodukovateľné). **Agresívny downsampling, ktorý ničí originály, by bol zlyhanie integrity dát.**

**Retencia riadi architektúru úložiska.** Teplotné záznamy skladu podporujú skladové podmienky šarží; podľa logiky 4.77 tie záznamy musia žiť **aspoň 5 rokov po certifikácii QP, alebo 1 rok po exspirácii šarže, ak je to dlhšie** — reálne **5–10+ rokov** pre sklad s dlho exspirujúcimi produktmi. V kombinácii s klauzulou 7.1 z roku 2011 a s draft 17.5 potrebuješ nacenený, otestovaný plán dlhodobej retencie — **a zmluvnú odpoveď na otázku „čo sa stane s mojimi ôsmimi rokmi dát, ak prestaneš podnikať".**

**4.70 + 4.26 tlačia silno proti hybridu.** Workflow, kde systém vyrobí PDF, ktoré QA osoba vytlačí a podpíše perom, je presne ten hybridný vzor, o ktorom návrh hovorí „should be avoided". Zabudovaný e-podpis je teda zároveň compliance aj konkurenčný argument.

**4.36 je malý, ale skutočný darček pre dodávateľov monitoringu:** tam, kde je validovaný proces kontinuálne monitorovaný a riadený, **automaticky generované reporty sa môžu obmedziť na súhrny zhody a reporty o odchýlkach/OOS**. To legitimizuje **exception-based reporting** — zákazník nemusí tlačiť 8 760 hodinových meraní na senzor za rok; stačí súhrn plus report odchýlok. **To je silný predajný bod** — a stále musí byť podložený plnými uchovanými raw dátami.

⚠️ **Nezrovnalosť v návrhu, ktorú si over:** klauzula **4.26 krížovo odkazuje na „section 13 'hybrid systems'"**, ale hybridné systémy sú v štruktúre sekciou 11 (4.82–4.85). Môže ísť o chybu číslovania v konzultačnom návrhu alebo o artefakt načítania.

---

## EU GMP Annex 15: právny základ predaja validačného balíka

**Zdroj:** [EudraLex Vol. 4, Annex 15 „Qualification and Validation" (v platnosti od 1. októbra 2015)](https://health.ec.europa.eu/system/files/2016-11/2015-10_annex15_0.pdf). Všetky nasledujúce klauzuly sa vrátili ako doslovný prepis ✅, s dvoma výnimkami označenými ⚠️.

### Sekcia 1 — Organising and Planning

> **1.1** "All qualification and validation activities should be planned and take the life cycle of facilities, equipment, utilities, process and product into consideration."
>
> **1.2** "Qualification and validation activities should only be performed by suitably trained personnel who follow approved procedures."
>
> **1.3** "Qualification/validation personnel should report as defined in the pharmaceutical quality system although this may not necessarily be to a quality management or a quality assurance function. However, there should be appropriate quality oversight over the whole validation life cycle."
>
> **1.4** "The key elements of the site qualification and validation programme should be clearly defined and documented in a validation master plan (VMP) or equivalent document."
>
> **1.5** "The VMP or equivalent document should define the qualification/validation system and include or reference information on at least the following:
> i. Qualification and Validation policy
> ii. The organisational structure including roles and responsibilities for qualification and validation activities
> iii. Summary of the facilities, equipment, systems, processes on site and the qualification and validation status
> iv. Change control and deviation management for qualification and validation
> v. Guidance on developing acceptance criteria
> vi. References to existing documents
> vii. The qualification and validation strategy, including requalification, where applicable."

### Sekcia 2 — Documentation

> **2.1** "Good documentation practices are important to support knowledge management throughout the product lifecycle."
>
> **2.2** "All documents generated during qualification and validation should be approved and authorised by appropriate personnel as defined in the pharmaceutical quality system."
>
> **2.3** "The inter-relationship between documents in complex validation projects should be clearly defined."
>
> **2.4** "Validation protocols should be prepared which defines the critical systems, attributes and parameters and the associated acceptance criteria."
>
> **2.5** "Qualification documents may be combined together, where appropriate, e.g. installation qualification (IQ) and operational qualification (OQ)."
>
> **2.6** "Where validation protocols and other documentation are supplied by a third party providing validation services, appropriate personnel at the manufacturing site should confirm suitability and compliance with internal procedures before approval."

**Klauzula 2.6 je právny základ toho, že môžeš predávať validačný balík.** Tretia strana (ty) smie legitímne napísať a dodať protokoly — ale dokument musí byť napísaný tak, aby ho QA zákazníka vedela preskúmať a schváliť proti vlastným SOP. Prakticky to znamená: nechaj v dokumentoch **podpisové bloky pre zákazníka, pole pre jeho číslo dokumentu a vyhlásenie o súlade s jeho validačnou SOP.** Bez týchto troch vecí nie je balík použiteľný, aj keby bol obsahovo dokonalý.

**Klauzula 2.5 zas legitimizuje predaj kombinovaného IOQ** pre malé inštalácie, čo znižuje cenový bod bez oslabenia súladu. Pre lekáreň s dvoma senzormi je IOQ správna odpoveď; plné oddelené IQ a OQ je overkill, ktorý zákazníka odradí.

### Sekcia 3 — Qualification stages

> **3.1** "Qualification activities should consider all stages from initial development of the user requirements specification through to the end of use of the equipment, facility, utility or system."
>
> **3.2 (URS)** "The specification for equipment, facilities, utilities or systems should be defined in a URS and/or a functional specification." ⚠️ *(oficiálna klauzula pokračuje v zmysle, že základné prvky kvality treba zabudovať v tejto fáze a GMP riziká znížiť na prijateľnú úroveň — pokračovanie sa nezískalo doslovne)*
>
> **3.3 (DQ)** "The next element in the qualification of equipment, facilities, utilities, or systems is DQ where the compliance of the design with GMP should be demonstrated and documented."
>
> **3.4 (FAT)** "Equipment, especially if incorporating novel or complex technology, may be evaluated, if applicable, at the vendor prior to delivery."
>
> **3.5** "Prior to installation, equipment should be confirmed to comply with the URS/functional specification at the vendor site, if applicable."
>
> **3.6** "Where appropriate and justified, documentation review and some tests could be performed at the FAT or other stages without the need to repeat on site at IQ/OQ if it can be shown that the functionality is not affected by the transport and installation."
>
> **3.7 (SAT)** "FAT may be supplemented by the execution of a SAT following the receipt of equipment at the manufacturing site."
>
> **3.8 (IQ)** "IQ should be performed on equipment, facilities, utilities, or systems."
>
> **3.9 (obsah IQ)** "IQ should include, but is not limited to the following:
> i. Verification of the correct installation of components, instrumentation, equipment, pipe work and services against the engineering drawings and specifications
> ii. Verification of the correct installation against pre-defined criteria
> iii. Collection and collation of supplier operating and working instructions and maintenance requirements
> iv. Calibration of instrumentation
> v. Verification of the materials of construction."
>
> **3.10 (IOQ)** "OQ normally follows IQ but depending on the complexity of the equipment, it may be performed as a combined Installation/Operation Qualification (IOQ)."
>
> **3.11 (obsah OQ)** "OQ should include but is not limited to the following:
> i. Tests that have been developed from the knowledge of processes, systems and equipment to ensure the system is operating as designed
> ii. Tests to confirm upper and lower operating limits, and /or 'worst case' conditions."
>
> **3.12** "The completion of a successful OQ should allow the finalisation of standard operating and cleaning procedures, operator training and preventative maintenance requirements."
>
> **3.13 (PQ)** "PQ should normally follow the successful completion of IQ and OQ. However, it may in some cases be appropriate to perform it in conjunction with OQ or Process Validation."
>
> **3.14 (obsah PQ)** "PQ should include, but is not limited to the following:
> i. Tests, using production materials, qualified substitutes or simulated product proven to have equivalent behaviour under normal operating conditions with worst case batch sizes.
> ii. Tests should cover the operating range of the intended process, unless documented evidence from the development phases confirming the operational ranges is available."

**Klauzula 3.6 je tvoj nástroj na zníženie nákladov.** Umožňuje spraviť **FAT na serveri/softvéri u teba doma** (nakonfigurovaná testovacia inštancia) a neopakovať všetko na mieste — ale musí to byť **zdokumentované a zdôvodnené**. Pre softvérovú časť monitorovacieho systému je to úplne legitímne: overenie, že alarmová logika funguje, sa transportom nezmení. Overenie, že senzor visí na správnom mieste, áno.

### Sekcia 4 — Re-qualification

> **4.1** "Equipment, facilities, utilities and systems should be evaluated at an appropriate frequency to confirm that they remain in a state of control."
>
> **4.2** "Where re-qualification is necessary and performed at a specific time period, the period should be justified and the criteria for evaluation defined." ⚠️ *(oficiálna klauzula obsahuje aj znenie o možnosti posudzovať drobné zmeny v čase; over)*

**Klauzula 4.3 neexistuje** — sekcia 4 končí na 4.2 ✅.

### Sekcia 11 — Change control

> **11.1** "The control of change is an important part of knowledge management and should be handled within the pharmaceutical quality system."
>
> **11.2** "Written procedures should be in place to describe the actions to be taken if a planned change is proposed to a starting material, product component, process, equipment, premises, product range, method of production or testing, batch size, design space or any other change during the lifecycle that may affect product quality or reproducibility."
>
> **11.3** "Where design space is used, the impact on changes to the design space should be considered against the registered design space within the marketing authorisation and the need for any regulatory actions assessed."
>
> **11.4** "Quality risk management should be used to evaluate planned changes to determine the potential impact on product quality, pharmaceutical quality systems, documentation, validation, regulatory status, calibration, maintenance and on any other system to avoid unintended consequences and to plan for any necessary process validation, verification or requalification efforts."
>
> **11.5** "Changes should be authorised and approved by the responsible persons or relevant functional personnel in accordance with the pharmaceutical quality system."

**11.4 je spúšťacia veta pre regresiu/rekvalifikáciu po softvérovom release.** Je to klauzula, ktorú budeš citovať v každom release packu.

⚠️ Nezískané: pokračovanie 3.2 a 4.2; klauzula 11.6 (post-implementačné hodnotenie účinnosti zmeny), ak existuje — sekcia 11 môže pokračovať za 11.5 a treba to skontrolovať; sekcie 5 (Process Validation), 6 (Verification of transportation — relevantné, ak zákazník aj prepravuje), 8 (Qualification of utilities) a Glossary.

### Presný zoznam dokumentov, ktorý z Annexu 15 doslovne plynie

Čítané do písmena, Annex 15 vyžaduje túto reťaz: **VMP (alebo odkaz na VMP zákazníka, 1.4–1.5) → URS a/alebo funkčná špecifikácia (3.2) → DQ (3.3) → voliteľne FAT/SAT (3.4–3.7) → IQ protokol + report (3.8–3.9) → OQ protokol + report (3.10–3.11) → PQ protokol + report (3.13–3.14) → Validation Summary Report → stratégia riadenia zmien a rekvalifikácie (1.5 vii, 4.1–4.2, 11.x).** Každý z nich má za sebou menovanú klauzulu — a presne to patrí na titulnú stranu plateného balíka. Zákazník, ktorý vidí, že každý dokument v balíku je naviazaný na klauzulu, prestane pýtať zľavu.

---

## GAMP 5: kategória 5 je drahá, kategória 4 je predajná

GAMP 5 nie je zákon, ale je to de facto očakávaná metodika v EÚ a **jeho softvérové kategórie určujú rozsah validačného úsilia.** ⚠️ Primárny text ISPE GAMP 5 (2. vydanie, 2022) je platený a nebol získaný; všetky popisy kategórií nižšie pochádzajú zo sekundárneho komentára a presné znenie GAMP ani číslovanie príloh (napr. M4 kategórie, M2 posúdenie dodávateľa) sa nepodarilo overiť.

| Kategória | Čo to je | Čo sa musí dodať |
|---|---|---|
| **1 — Infrastructure software** | OS, databázy, virtualizácia použité tak, ako sú dodané: Windows Server, Red Hat Linux, Oracle DB, MS SQL Server, VMware vSphere, Active Directory | *„configuration documentation + change control only"*; typicky len konfiguračné záznamy a overenie inštalácie |
| **3 — Non-configured products** | komerčný softvér použitý presne tak, ako je dodaný, bez vlastných workflow či polí | *„functional testing for intended use"* + overenie inštalácie |
| **4 — Configured products** | *„the most common category in pharma"*: SAP S/4HANA, Veeva Vault QMS, LabWare LIMS, MasterControl. *„You validate your configuration, not the base platform."* | *„full IQ/OQ/PQ on your configuration"* s RTM viažucou požiadavky na testovacie prípady; **kategória 4 umožňuje využiť dôkazy dodávateľa** |
| **5 — Custom software** | *„built specifically for you — including any custom script that processes GxP data, regardless of code length"*; príklady výslovne zahŕňajú **bespoke SCADA/DCS systémy** a **custom environmental monitoring systémy** | *„full SDLC lifecycle validation"*, plná dokumentačná prísnosť, *„full testing rigour and unscripted documentation"*, code reviews |

— [GoValidation](https://govalidation.com/blog/gamp5-software-category-examples/); [Kneat](https://kneat.com/article/what-is-gamp-5/)

**Zlá správa: vlastný systém environmentálneho monitoringu je výslovne uvedený ako kategória 5**, pretože je *„built specifically for your organisation"*. Kategória 5 *„demands full development life cycle documentation and code reviews plus rigorous testing across unit through PQ phases."* V dodávateľských termínoch: kategória 4 vyžaduje len konfiguráciu a postačí dokumentácia návrhu a testovania kódu, ak dodávateľ poskytne primerané dôkazy; kategória 5 vyžaduje plný model životného cyklu kombinujúci vývoj a integráciu systému, *„necessitating substantially more rigorous documentation and testing"* ([Osborne, *GAMP 5 and the supplier*](https://secure.toolkitfiles.co.uk/clients/20735/sitedata/files/GAMP_5_and_the_supplier.pdf)).

**Dobrá správa, a je komerčne zásadná — únikový východ do kategórie 4.** Ak sa **ten istý softvér predáva ako štandardný produkt mnohým zákazníkom** a zákazník iba konfiguruje (alarmové limity, používatelia, názvy senzorov, šablóny reportov) **bez akéhokoľvek kódu špecifického pre zákazníka**, dá sa na mieste zákazníka argumentovať a zdokumentovať **kategória 4**, pričom prísnosť kategórie 5 absorbuješ raz vo vlastnom internom produktovom životnom cykle. Balík pre zákazníka sa tým mení na „validujte konfiguráciu", čo je podstatne lacnejší a opakovateľnejší predaj.

**Čo pre to musíš urobiť, aby to obstálo:**

1. **Žiadny kód špecifický pre zákazníka.** Ani jeden `if customer == "X"`. Ani jedna vetva. Ani jeden zákaznícky patch. Toto je tvrdá podmienka a je to zároveň dobrá inžinierska disciplína.
2. **Všetko zákaznícky špecifické musí byť konfigurácia** — dátami riadené, uložené v DB alebo v konfiguračnom súbore, nie v kóde. Vrátane šablón reportov, prekladov, pravidiel alarmov, rolí.
3. **Zdokumentuj to v samostatnom dokumente „GAMP category determination + justification"** vo validačnom pláne / posúdení rizík, s dôkazom, že zákaznícky kód neexistuje (napr. vyhlásenie o jednotnom buildovanom artefakte a jeho hash).
4. **Draft 6.6 ti to uľahčuje:** *„It should be clear what functionality, if any, is modified or added by configuration of a system. Options allowing configuration of system functionality should be described in the requirements specification."* — teda maj **dokumentovaný zoznam konfiguračných možností**.
5. **Kategória 5 prísnosť však stále musíš mať u seba** — vlastný SDLC, testy, release proces. Len ju nepredávaš zákazníkovi ako jeho úlohu.

**Prečo je kategória 5 aj dôvod, prečo ťa budú auditovať:** pri kategórii 5 neexistuje inštalovaná báza identických validovaných inštancií, takže jedinou istotou zákazníka je tvoj vlastný systém kvality. To je presne dôvod, prečo sekcia o dodávateľskom dotazníku nižšie nie je voliteľná.

**V-model a sledovateľnosť:** V-model mapuje vývojové činnosti (ľavá strana: URS → FS → DS) na zodpovedajúce kvalifikácie (pravá strana: PQ, OQ, IQ) a *„ensures traceability between specifications and test results"* ([Kneat](https://kneat.com/article/what-is-gamp-5/)). *„A traceability matrix is usually created to map each requirement through the design and testing process, ensuring nothing is missed."* ([IntuitionLabs](https://intuitionlabs.ai/articles/gamp-5-computerized-system-validation-pharma)).

**Hranica využitia dodávateľa, ktorú si zapamätaj a povedz zákazníkovi skôr, než sa spýta:** *„the supplier is not responsible for the testing, the end customer is"* ([Osborne](https://secure.toolkitfiles.co.uk/clients/20735/sitedata/files/GAMP_5_and_the_supplier.pdf)). Regulované firmy majú *„seek to maximise supplier involvement throughout the system life cycle"*, ale len po uspokojivom posúdení, a dokumentácia dodávateľa sa hodnotí na *„suitability, accuracy and completeness"*.

**GAMP 5 druhé vydanie (2022)** zdôrazňuje: profesionálny úsudok a kritické myslenie pri proporcionálnom posudzovaní rizík; validáciu cloudu a SaaS; ALCOA++; kybernetickú bezpečnosť integrovanú do validácie; zosúladenie s CSA; a agilný/iteratívny vývoj s uznaním, že *„modern development does not always follow a linear path"* ([Kneat](https://kneat.com/article/what-is-gamp-5/)).

**K otázke CSA vs CSV, stručne a prakticky:** CSA je koncept FDA pre produkčný a systémovo-kvalitný softvér, nie európske právo, a **európske CSA usmernenie neexistuje.** Jeho logika (úsilie úmerné riziku, kritické myslenie, unscripted testovanie, využitie systémom generovaných dôkazov) však bola absorbovaná do GAMP 5 2. vydania, ktoré európski inšpektori očakávajú. **Marketuj a dokumentuj balík ako „GMP Annex 11 / Annex 15 / GAMP 5", nie ako „CSA".** Slovenský inšpektor alebo QA zákazníka pozná Annex 11/15 a GAMP 5; „CSA" vyvolá otázky. CSA-štýlové efektivity používaj, ale vždy ich ukotvi v zdokumentovanom posúdení rizík a v Annex 11 klauzule 4.1 („should be able to justify"). ⚠️ Existencia a presný názov/dátum údajného **FDA CSA usmernenia zo septembra 2025** neboli overené z primárneho zdroja FDA; pôvodný draft bol zo septembra 2022. Over na fda.gov.

⚠️ Tiež neoverené: či GAMP 5 2. vydanie skutočne zrušilo kategóriu 2 (firmware) — je to široko uvádzané, ale z primárneho zdroja sa to nepotvrdilo.

---
## Integrita dát a ALCOA+: ako sa tvoj návrh mapuje na predpisy

### PIC/S PI 041-1 (1. júla 2021) — deväť definícií ALCOA+ doslovne

**ŠÚKL je členom PIC/S, takže PI 041-1 je pre teba prakticky inšpekčný štandard**, nie akademický materiál. Zdroj: [PIC/S PI 041-1](https://picscheme.org/docview/4234).

**Potvrdená štruktúra dokumentu** ✅ (kotva pre všetky citácie): 5 DATA GOVERNANCE SYSTEM (5.1 What is data governance?; 5.2 Data governance systems; 5.3 Risk management approach; **5.4 Data criticality**; **5.5 Data risk**; 5.6 Data governance system review) · 6 ORGANISATIONAL INFLUENCES (6.1–6.7) · **7 GENERAL DATA INTEGRITY PRINCIPLES AND ENABLERS** (číslované odseky 7.1…7.x) · 8 PAPER-BASED SYSTEMS (…8.7 Making corrections on records; 8.8 Verification of records; 8.9 Direct print-outs from electronic systems; **8.10 Document retention**; 8.11 Disposal of original records or true copies) · **9 COMPUTERISED SYSTEMS** (9.1 Structure of the PQS; 9.2 Qualification and validation; 9.3 Validation and Maintenance; **9.4 Data Transfer**; **9.5 System security**; **9.6 Audit trails**; **9.7 Data capture/entry**; **9.8 Review of data**; **9.9 Storage, archival and disposal**; 9.10 Management of Hybrid Systems) · 10 outsourced activities · 11 regulatory actions · 12 remediation · 13 Glossary · 14 Revision history.

**Deväť definícií ALCOA+, PI 041-1 §7.5** ✅ — vrátili sa **identicky z troch nezávislých načítaní**, preto najvyššia dôvera. §7.4 uvádza, že akronym ALCOA zhŕňa kľúčové koncepty a že Complete, Consistent, Enduring a Available ho rozširujú na ALCOA+.

| Prvok | Doslovná definícia (PI 041-1 §7.5) | Čo to znamená pre telemetry.digital |
|---|---|---|
| **Attributable** | "It should be possible to identify the individual or computerised system that performed a recorded task and when the task was performed." | **Toto je najdôležitejšia veta celého výskumu pre tvoj dizajn.** „Attributable" výslovne pripúšťa, že vykonávateľom môže byť *„the individual **or computerised system**"*. Meranie pripísané **identite senzora + verzii dekodéra**, nie ľudskému používateľovi, je presne to, s čím usmernenie počíta. Tvoj DS28E07 s kalibračnými koeficientmi v sonde je fyzická implementácia atribuovateľnosti. |
| **Legible** | "All records should be legible – the information should be readable and unambiguous in order for it to be understandable and of use." | Export audit trailu s rozloženými menami a popiskami polí, nie surové ID. |
| **Contemporaneous** | "The evidence of actions, events or decisions should be recorded as they take place." | Kľúčové: ide o to, **kedy sa dôkaz zaznamená**, nie kedy dorazí na server. Viď backfill nižšie. |
| **Original** | "The original record can be described as the first-capture of information, whether recorded on paper (static) or electronically." | Prvý zachytený údaj sa nikdy neprepisuje. |
| **Accurate** | "Records need to be a truthful representation of facts to be accurate." | Kalibračná korekcia musí byť sledovateľná, nie skrytá. |
| **Complete** | "All information that would be critical to recreating an event is important when trying to understand the event." | **Toto je základ pre povinnú detekciu medzier** — viď nižšie. |
| **Consistent** | "Information should be created, processed, and stored in a logical manner that has a defined consistency." | Základ pre argument o UTC a letnom čase. |
| **Enduring** | "Records should be kept in a manner such that they exist for the entire period during which they might be needed." | 5–10 rokov, nie 12 mesiacov, pre distribútorov. |
| **Available** | "Records should be available for review at any time during the required retention period." | Archív musí byť dotazovateľný, nie len uskladnený. |

Ďalšie relevantné pasáže ⚠️ (znenie nepotvrdené, číslovanie nepotvrdené):

- §5.4 Data criticality — body na zváženie zahŕňajú "Which decision does the data influence?" a "What is the impact of the data to product quality or safety?"
- §5.5 Data risk — posúdenie má zvažovať "the vulnerability of data to involuntary alteration, deletion, loss (either accidental or by security failure) or re-creation or deliberate falsification"
- §5.6.2 (self-inspection) má zahŕňať "A risk-based sample of computerised system logs / audit trails to ensure that information of relevance to GMP/GDP activity is reported accurately." *(vrátené dvakrát nezávisle s rovnakým znením — stredne vysoká dôvera)*
- §9.1 oblasť: "In dealing with raw data, the complete capture and retention of raw data would normally be required in order to reconstruct the manufacturing event or analysis."
- metadáta: časť metadát je kritická pre rekonštrukciu udalostí a je „relevantná"; príklady: "user identification, times, critical process parameters"
- §9.4 Data Transfer: "Interfaces should be assessed and addressed during validation to ensure the correct and complete transfer of data."
- §9.5 System security: "There should also be restrictions to prevent users from amending audit trail functions and from changing any pre-defined directory paths where data files are to be stored."
- §8.10: "The retention period of each type of records should (at a minimum) meet those periods specified by GMP/GDP requirements."

❌ **Opätovné varovanie:** sedem „klauzúl 9.6.1–9.6.7" o audit trailoch, ktoré nástroj vrátil, **neexistuje** — §9.6 a §9.7 sú tabuľky Item / Expectation / Potential risk. Do validačného dokumentu ich nevkladaj. **§9.6 a §9.7 si musíš otvoriť a prepísať bunky tabuľky sám** — sú to najdôležitejšie sekcie pre tvoj prípad a práve tie sa nepodarilo spoľahlivo získať.

### MHRA „GXP Data Integrity Guidance and Definitions" (marec 2018) — najpoužiteľnejší text

MHRA je po Brexite britský regulátor a **na Slovensku nemá právnu silu.** Napriek tomu ho európski inšpektori aj ISPE/GAMP rutinne citujú ako referenčný výklad. **Prezentuj ho ako presvedčivý, nie záväzný.** Zdroj: [MHRA GXP Data Integrity Guidance and Definitions, Revision 1, March 2018](https://assets.publishing.service.gov.uk/media/5aa2b9ede5274a3e391e37f3/MHRA_GxP_data_integrity_guide_March_edited_Final.pdf).

**§5 — hodiny** ✅ (vrátené dvakrát nezávisle): systémy majú poskytovať

> "access to appropriately controlled/synchronised clocks for recording timed events"

na zabezpečenie rekonštrukcie a sledovateľnosti, vrátane *„knowing and specifying the time zone where this data is used across multiple sites"*.

**§6.2 Raw data** ✅: "The original record (data) which can be described as the first-capture of information."

**§6.6 Data lifecycle** ✅: "All phases in the life of the data from generation through processing, use, retention, archive/retrieval and destruction."

**§6.8 Data transfer / migration** ✅: "The process of transferring data between different data storage types, formats, or computerised systems."

**§6.9 Data processing** ✅ — tri vety, ktoré sú pre teba zásadné:

> "A sequence of operations performed on data to extract, present or obtain information in a defined format."

> "There should be adequate traceability of any user-defined parameters used within data processing activities to the raw data, including attribution to who performed the activity."

> "If data processing has been repeated with progressive modification of processing parameters this should be visible to ensure that the processing parameters are not being manipulated to achieve a more desirable result."

**§6.10 Excluding data** ✅:

> "Data may only be excluded where it can be demonstrated through valid scientific justification that the data are not representative of the quantity measured, sampled or acquired."

> "All data (even if excluded) should be retained with the original data set, and be available for review in a format that allows the validity of the decision to exclude the data to be confirmed."

**§6.11 Original record / true copy** ✅:

> Original record: "The first or source capture of data or information e.g. original paper record of manual observation or electronic raw data file from a computerised system."

> True copy: "A copy (irrespective of the type of media used) of the original record that has been verified (i.e. by a dated signature or by generation through a validated process) to have the same information, including data that describe the context, content, and structure, as the original."

> "True copies of original records may be retained in place of the original record (e.g. scan of a paper record), if a documented system is in place to verify and record the integrity of the copy."

**§6.13 Audit trail — kľúčová klauzula** ✅:

> "The audit trail is a form of metadata containing information associated with actions that relate to the creation, modification or deletion of GXP records."

> "Audit trails (identified by risk assessment as required) should be switched on. Users should not be able to amend or switch off the audit trail."

> "Where a system administrator amends, or switches off the audit trail a record of that action should be retained."

> "Routine data review should include a documented audit trail review where this is determined by a risk assessment."

> "Reviewers should have sufficient knowledge and system access to review relevant audit trails, raw data and metadata."

**§6.15 Data review and approval** ✅:

> "There should be a procedure that describes the process for review and approval of data. Data review should also include a risk-based review of relevant metadata, including relevant audit trails records."

> "Data review should be documented and the record should include a positive statement regarding whether issues were found or not, the date that review was performed and the signature of the reviewer."

> Procedúry "should enable data corrections or clarifications to provide visibility of the original record, and traceability of the correction, using ALCOA principles."

**§6.17 Data retention** ✅:

> "Data retention may be for archiving (protected data for long-term storage) or backup (data for the purposes of disaster recovery)."

**§6.17.2 Backup** ✅ (dve vety, ktoré vyhrávajú argument):

> "Backup and recovery processes should be validated and periodically tested. Each back up should be verified to ensure that it has functioned correctly."

> "Backups for recovery purposes do not replace the need for the long term, retention of data and metadata in its final form."

**Čo z MHRA plynie pre tvoj návrh.** §6.13 odpovedá na tri tvoje otázky jednou klauzulou: audit traily **musia byť zapnuté**, **používatelia ich nesmú meniť ani vypnúť**, a **vypnutie administrátorom sa samo musí zaznamenať.** Append-only PostgreSQL návrh, kde aplikačná rola nemá `UPDATE`/`DELETE` na tabuľkách meraní ani auditu, je najsilnejšia možná implementácia tejto vety. §6.17.2 je priamy argument proti návrhu, kde jediná dlhodobá kópia surových meraní je nočný `pg_dump`. A **§6.9 sa mapuje takmer presne na tvoje verziované dekodéry**: verzia dekodéra je „user-defined parameter used within data processing activities" a §6.9 vyžaduje sledovateľnosť od nej späť k raw dátam a pripísanie tomu, kto činnosť vykonal. **Uchovanie surového payloadu plus ID verzie dekodéra spĺňa §6.9 lepšie než uloženie iba dekódovanej hodnoty.** §6.10 je zas základ pre pravidlo „nikdy natvrdo nemaž meranie, ktoré sa neskôr ukáže ako chybné — označ ho".

### WHO TRS 996 Annex 5 a pravidlo o oneskorených zápisoch

**Definícia audit trailu, Glossary** ✅: audit trail je "a form of metadata that contains information associated with actions that relate to the creation, modification or deletion of GXP records"; uľahčuje rekonštrukciu histórie takých udalostí "including the '**who, what, when and why**' of the action" ([WHO TRS 996 Annex 5](https://rx-360.org/wp-content/uploads/2018/08/WHO_TRS_996_annex05.pdf)).

**Appendix 1 — nezávislosť** ⚠️: prístup k rozšíreným oprávneniam má byť "strictly controlled" a "restricted only to persons independent of those responsible for the content of electronic records."

**Appendix 1 — kľúčová veta pre backfill** ⚠️ (najpriamejšie relevantná veta v celom korpuse):

> "it is unacceptable to backdate or forward date a record. Instead the date recorded should be the actual date of the data entry. **Late entries should be indicated as such with both the date of the activity and the date of the entry being recorded.**"

**§9.4** ⚠️: "Contemporaneous data are data recorded at the time they are generated or observed."

**§4.12** ⚠️: kontroly majú zahŕňať "restricting the ability to change any clock used for recording timed events, for example, system clocks in electronic systems" a "ensuring automated data capture or printers are attached and connected to equipment … to ensure independent and timely recording of the data."

**Appendix 1 (Contemporaneous, elektronické)** ⚠️: elektronické záznamy vyžadujú "secure system time/date stamps that cannot be altered by personnel" a "procedures and maintenance programmes that ensure time/date stamps are synchronized across the GXP operations."

**§4.13** ⚠️: organizácie majú riešiť "controls that allow for the determination of the timing of one activity relative to another (e.g. time zone controls)."

**§11.11** ⚠️: "The approach to reviewing specific record content, such as critical data fields and metadata … should meet all applicable regulatory requirements and be risk-based."

### Backfill, letný čas a rekalibrácia uložených hodnôt: tri skutočné regulačné mlčania

Toto je časť, kde musíš byť voči zákazníkovi najpoctivejší, pretože usmernenia ti odpoveď nedajú a konkurencia bude predstierať, že áno.

**Mlčanie č. 1 — bufferované a doplnené (backfilled) dáta.** **Žiadne usmernenie, ktoré sa podarilo nájsť, nerieši dataloggery s onboard pamäťou, store-and-forward alebo doplnenie dát po výpadku siete.** Prehľadaný bol korpus PIC/S, MHRA, WHO, Annex 11, návrh Annexu 11 aj FDA. Je to **skutočné mlčanie, nie zlyhanie hľadania** — s jednou výhradou: ⚠️ nepodarilo sa úplne prečítať **PI 041-1 §9.7 „Data capture/entry for computerised systems"**, čo je jediné pravdepodobné miesto, kde by taká veta mohla byť. **Prečítaj tú sekciu skôr, než vyhlásiš, že usmernenia mlčia.**

Najbližší aplikovateľný princíp je **WHO pravidlo o oneskorených zápisoch** citované vyššie. Je napísané pre papier, ale mapuje sa jedna k jednej na schému, ktorá ukladá **`measured_at` (onboard časová pečiatka loggera = dátum činnosti)** a **`received_at` / `ingested_at` (prijatie na serveri = dátum zápisu)**, pričom dvojica je nemeniteľná a obe hodnoty sú viditeľné na každom reporte. **Uložiť len jednu z tých dvoch je presne to, čo pravidlo zakazuje.** Tvoja architektúra už toto rieši správne — treba to len **napísať do validačnej dokumentácie ako explicitný záver posúdenia rizík**, nie to nechať ako implementačný detail.

**„Contemporaneous" je splnené v momente zachytenia, nie v momente ingestu.** PI 041-1 §7.5 aj WHO §9.4 definujú kontemporaneitu odkazom na to, *kedy sa údaj generuje alebo pozoruje*. Logger s onboard pamäťou, ktorý pečiatkuje pri senzore a neskôr nahrá, teda zaznamenáva kontemporánne; výpadok siete odkladá *prenos*, nie *zaznamenanie*. Je to silný a principiálny argument — **ale závisí celý od toho, že hodiny loggera sú dôveryhodné a kontrolované.** Práve preto je DS3231M RTC v tvojom hardvéri nosným prvkom celého tohto argumentu, a nie luxusom. Zdokumentuj to tak.

**Backfill nesmie nikdy prepisovať ani ticho prekladať.** Append-only vkladanie oneskorených záznamov so zachovaným `received_at` znamená, že auditný obraz sa rekonštruuje správne; návrh, ktorý by preusporiadal alebo deduplikoval prepísaním, by porušil princíp „original record"/„first capture" (PI 041-1 §7.5 Original; MHRA §6.2, §6.11).

**Spätne objavená odchýlka je odchýlkou od momentu objavenia.** K tomuto scenáru neexistuje text, ale aplikovateľný reťazec je: GDP kap. 3.3 vyžaduje alarmy pri odchýlkach; GDP kap. 4 vyžaduje záznam; systém kvality (GDP kap. 1) vyžaduje vyšetrenie odchýlok. Praktická pozícia: *odchýlka* sa datuje na `measured_at`, *záznam o odchýlke* na moment objavenia, a **medzera medzi nimi je sama údajom, ktorý musí vyšetrovanie vysvetliť** (prečo nebol alarm vyvolaný v reálnom čase?). Monitorovací systém by mal preto robiť z „odchýlka detegovaná na doplnených dátach" **samostatný, viditeľný typ udalosti**, nie ju schovať medzi bežné alarmy. To je funkcia, ktorú konkurencia nemá.

**Detekcia medzier je nevyslovená, ale implikovaná požiadavka.** Nič v usmerneniach nehovorí „detekuj chýbajúce dáta", ale ALCOA+ **Complete** („All information that would be critical to recreating an event", PI 041-1 §7.5) znamená, že obdobie bez dát musí byť *viditeľné ako medzera*, nie neviditeľné. **Systém, ktorý ticho nakreslí neprerušenú čiaru cez štvorhodinový výpadok, nie je Complete.** Explicitné reportovanie medzier — a ich uzatváranie, keď príde backfill — je priama implementácia „Complete". Toto je inferencia z princípov, nie citovaná požiadavka, ale inšpektor ju okamžite rozpozná.

**Mlčanie č. 2 — letný čas.** **Nenašlo sa žiadne usmernenie k letnému času pre GxP záznamy. Skutočné mlčanie.** V zóne Europe/Bratislava nastáva hodina 02:00–03:00 pri októbrovej zmene v lokálnom čase dvakrát. Systém ukladajúci naivný lokálny čas robí dve odlišné hodiny nerozlíšiteľnými — priame porušenie ALCOA+ **Consistent** („stored in a logical manner that has a defined consistency") a diskutabilne aj **Attributable** (§7.5 vyžaduje identifikovať „when the task was performed"). Uloženie UTC s renderovaním zóny problém odstraňuje úplne. Prezentuj to ako inferenciu z princípov, nie ako citovanú požiadavku — ale je to presne ten typ nálezu, ktorý inšpektor okamžite ocení.

**„Trusted time source" nie je definovaný regulačný pojem** v žiadnom dosiahnuteľnom dokumente. Regulačné očakávanie je *kontrolovaný a synchronizovaný*, nie *kryptograficky atestovaný*. NTP k zdokumentovanému, spoľahlivému zdroju plus záznamy o periodickom overovaní teda latku spĺňajú; RFC 3161 trusted timestamping je bonus, nie požiadavka. **Povedz si to nahlas, aby si to neprevalidoval.**

**Štyri konkrétne inžinierske rozhodnutia, ktoré z WHO/MHRA priamo plynú a ktoré vieš zdokumentovať:**

1. *„Restricting the ability to change any clock"* → aplikácia nikdy neprijme hodiny dodané klientom ako autoritatívny `received_at`; serverové hodiny sú disciplinované NTP, konfigurácia NTP je súčasťou kvalifikovanej infraštruktúry a žiadny aplikačný používateľ nemá OS oprávnenie nastaviť hodiny.
2. *„Secure time/date stamps that cannot be altered by personnel"* → `received_at` zapisuje databáza (`now()` / `clock_timestamp()` v append-only inserte), nie aplikačný kód a nikdy nie klient.
3. *„Synchronized across the GXP operations"* → drift hodín loggera je monitorovaný, periodicky korigovaný parameter s dokumentovanou toleranciou; **korekcia driftu je sama auditovanou konfiguračnou udalosťou.**
4. *„Time zone controls"* → ukladaj všetko ako `timestamptz` (interne UTC), renderuj s explicitnou zónou, nikdy neukladaj naivnú lokálnu pečiatku.

⚠️ Nenašlo sa žiadne usmernenie určujúce prijateľnú toleranciu driftu hodín, NTP stratum ani frekvenciu resynchronizácie.

### Raw vs kalibrovaná hodnota — a čo s uloženými dátami po rekalibrácii

**Usmernenia nikdy nerozhodnú, či je „originálnym záznamom" surová ADC/odporová hodnota senzora alebo kalibrovaná teplota.** GMP Chapter 4 rozhodnutie výslovne deleguje ✅:

> "For electronic records regulated users should define which data are to be used as raw data. At least, all data on which quality decisions are based should be defined as raw data."

— [EU GMP Chapter 4](https://health.ec.europa.eu/system/files/2016-11/chapter4_01-2011_en_0.pdf)

Keďže rozhodnutia o kvalite stoja na **kalibrovanej teplote v °C**, kalibrovaná hodnota **musí** byť považovaná za raw data a uchovaná. Otázka je, či sa musí uchovať aj podkladová surová hodnota. **Najsilnejšia pozícia — a tá, ktorá robí z tvojho verziovaného dekodéra prednosť namiesto záťaže — je:**

- uchovať **surový payload tak, ako prišiel** (nemeniteľne, append-only);
- uchovať **verziu dekodéra/kalibrácie**, ktorá bola aplikovaná, a **odvodenú hodnotu**;
- vedieť hodnotu **kedykoľvek znovu odvodiť** zo surového payloadu + verzie dekodéra;
- každú zmenu kalibračných koeficientov považovať za **auditovanú konfiguračnú zmenu s dôvodom**, ktorá vytvorí **nový odvodený záznam**, nie mutáciu starého.

To priamo spĺňa MHRA §6.9 (sledovateľnosť parametrov spracovania) aj PI 041-1 („complete capture and retention of raw data … in order to reconstruct").

**Aplikovanie kalibračnej korekcie na už uložené dáta je nebezpečný prípad a usmernenia k nemu mlčia.** ⚠️ **Nenašlo sa žiadne usmernenie riešiace rekalibráciu už nasadeného senzora a nakladanie s predtým uloženými hodnotami. Skutočné mlčanie; pozícia nižšie je inferencia z prvých princípov.** Prepísanie uložených teplôt rekalibrovanými hodnotami by porušilo Annex 11 klauzulu 9 (zmena bez audit trailu a dôvodu), PI 041-1 §7.5 Original (ničí prvé zachytenie) aj MHRA §6.9 (neviditeľná modifikácia parametrov spracovania). **Zápis *novej* odvodenej série s novou verziou dekodéra pri zachovaní viditeľnosti starej série neporuší ani jedno z toho.** Toto je najjasnejšia odpoveď typu „kde sú tvoje návrhové rozhodnutia obhájiteľné" v celom materiáli — a je to funkcia, ktorou vieš v dodávateľskom dotazníku vyniknúť.

### Zálohy, archív, výmaz

**Annex 11 klauzula 7.2 vyžaduje kontrolu schopnosti obnovy počas validácie *aj* periodicky — dve odlišné povinnosti.** Návrh §16.6 pridáva tretí spúšťač: **po zmenách zálohovacieho alebo obnovovacieho procesu.** Zdokumentovaný test obnovy (dátum, rozsah, kto, výsledok, nezrovnalosti) vykonaný pri validácii, podľa definovaného periodického rozvrhu a po každej zmene nástroja spĺňa všetky tri.

**Automatický výmaz:** žiadny nástroj časovo založený výmaz výslovne nezakazuje, ale draft §17.1 („protected from deletion and changes throughout the retention period") plus §17.2 („verified … before any data is deleted") znamenajú, že akýkoľvek automatický purge musí byť (a) nemožný pred uplynutím retenčnej lehoty, (b) predchádzaný overeným archívom a (c) auditovaný. **Cron job, ktorý dropuje staré partície bez týchto troch kontrol, je neobhájiteľný.** Rozumné produktové rozhodnutie: **žiadny automatický výmaz vôbec; výmaz je manuálna, autorizovaná, logovaná operácia.**

**Dizajnová konsekvencia retencie:** systém potrebuje **retenčnú politiku podľa triedy záznamu, nie jedno globálne číslo**, a keďže GMP prípad je „exspirácia + 1 rok", retenčné hodiny pre niektoré záznamy sa nedajú spočítať z časovej pečiatky merania — závisia od externých šaržových dát. **Najbezpečnejšia inžinierska pozícia pre monitorovací produkt: uchovávaj všetko, nemaž nič automaticky.**

**Audit trail sa musí uchovávať minimálne tak dlho ako podkladový záznam** (explicitné v 21 CFR 11.10(e); implicitné v PI 041-1 §8.10 a Annex 11 kl. 17). **Audit trail sa teda nesmie orezávať na kratšom rozvrhu než merania.**

### Kde audit trail v append-only systéme naozaj „hryzie"

Toto je najhodnotnejšia analytická poznámka celého materiálu a treba ju napísať do validačnej dokumentácie doslova. Pre append-only sklad meraní je čestná pozícia: **merania sa nikdy nemenia, takže audit trail meraní je degenerovaný** — povinnosť audit trailu miesto toho dopadá na (i) konfiguračné zmeny (alarmové limity, kalibračné offsety, priradenia senzor↔miesto, verzie dekodérov), (ii) zmeny používateľov a prístupov, (iii) potvrdenia odchýlok a ich zdôvodnenia, a (iv) akúkoľvek administratívnu akciu. **Tam sa musí zachytávať „kto/čo/kedy/prečo" a to je to, čo má pokrývať „routine data review".** Povedať to výslovne vo validačnej dokumentácii je oveľa silnejšie než tvrdiť „máme audit trail".

Z toho plynie ďalší presný záver: **„prečo" (dôvod zmeny) vyžaduje Annex 11 kl. 9 len pri zmene alebo výmaze GMP-relevantných dát.** V append-only systéme neexistuje zmena ani výmaz meraní, takže povinné pole dôvodu patrí na konfiguračné akcie a na potvrdenia — a to je obhájiteľné a inšpektovateľné návrhové rozhodnutie.

**Nezávislosť audit trailu od operátora** podporuje WHO Appendix 1 aj PI 041-1 §9.5 (obmedzenie používateľom meniť funkcie audit trailu). **V jednoosobovej firme je skutočné oddelenie povinností nemožné, takže kompenzačné opatrenie musí byť technické:** databázová rola bez DELETE/UPDATE na auditných tabuľkách, plus **zdokumentované priznanie, že administrátor je zároveň používateľom**, plus dôkazy o periodickom prehľade. **Inšpektori očakávajú, že ten argument o kompenzačných opatreniach bude napísaný.** Nie zamlčaný.

⚠️ **Poznámka k pojmu „targeted audit trail review":** tento presný výraz sa **nenachádza ako definovaný pojem v žiadnom primárnom dokumente**, ku ktorému sa podarilo dostať. Najbližšie koncepty sú MHRA §6.13 („documented audit trail review where this is determined by a risk assessment"), PI 041-1 §5.6.2 („risk-based sample of computerised system logs / audit trails"), draft Annex 11 12.5/12.8 a WHO §11.11. Koncept rizikovo založeného vzorkovania je overený; **to, že nejaký dokument používa frázu „targeted audit trail review", overené nie je.**

### Porovnanie s 21 CFR Part 11 (pre zákazníkov, ktorí exportujú do USA)

Sú komplementárne, nie ekvivalentné: **21 CFR Part 11 je záväzná americká regulácia úzko o elektronických záznamoch a podpisoch; Annex 11 je európske GMP usmernenie riadiace celý životný cyklus počítačového systému.** Annex 11 je širší na validáciu, riadenie rizík, periodický prehľad a kvalifikáciu dodávateľa; Part 11 je preskriptívnejší na technickú mechaniku podpisov. **Systém postavený na prísnejšiu z dvoch noriem v každej téme spĺňa obe, a návrh z roku 2025 rozdiel výrazne zužuje.**

Kľúčový rozdiel v prospech Annexu 11 ⚠️: *„Annex 11 explicitly requires supplier qualification in its general requirements (Clause 3). FDA 21 CFR Part 11 does not explicitly address risk management or vendor assessment."* ([SimplerQMS](https://simplerqms.com/21-cfr-part-11-vs-eu-annex-11/)).

**Praktické pravidlá duálneho súladu:**
- **Audit trail: vždy zapnutý, nikdy rizikovo voliteľný.** Part 11 §11.10(e) ho vyžaduje bezpodmienečne pre záznamy pod predikátovými pravidlami; draft Annex 11 12.1 súhlasí. **Nestavaj konfiguračný prepínač „audit trail enabled".**
- **E-podpisy: riaď sa preskriptívnosťou Part 11** — manifestácia podpisu s menom, dátumom/časom a významom (§11.50); väzba podpis/záznam (§11.70); dve odlišné identifikačné komponenty; re-autentifikácia pri podpise (zhodné s draft 13.3).
- **Kópie: Part 11 §11.10(b)** vyžaduje schopnosť vygenerovať presné a úplné kópie v čitateľnej **aj elektronickej** forme vhodnej na inšpekciu — čo sa zhoduje s draft 12.9 („Flat and locked files are not acceptable") a posilňuje to.
- **Validácia, kvalifikácia dodávateľa, periodický prehľad, kontinuita: riaď sa Annexom 11** — Part 11 k nim mlčí.

⚠️ **Text 21 CFR Part 11 sa nepodarilo získať** (eCFR blokovaný). Všetky detaily subpart C (§11.100, §11.200, §11.300) aj §11.10(e), §11.50 a §11.70 v tomto reporte sú **z pamäti a musia byť overené** na [ecfr.gov](https://www.ecfr.gov/current/title-21/chapter-I/subchapter-A/part-11). Zároveň: zdroj SimplerQMS je marketingová stránka dodávateľa QMS a jeho charakterizácia Annexu 11 ako „legally binding" je zjednodušenie — Annex 11 je usmernenie k smernici, vynucované inšpekciou, nie priamo záväzný zákon. **Nepreberaj tú formuláciu nekriticky.**

---
## Mapovanie teplôt: praktická príručka s číslami a s tým, odkiaľ pochádzajú

**Najdôležitejšia veta tejto sekcie: každé číslo nižšie pochádza z WHO alebo z praxe dodávateľov. EU GDP neurčuje ani jedno.** Do zákazníckeho protokolu ich píš ako *„metodika podľa WHO TRS 961 Annex 9 Supplement 8"*, nie ako *„EU GDP vyžaduje"*.

### Koľko loggerov — mriežka, nie vzorec

**Žiadny regulátor nepublikuje vzorec „loggerov na m³".** Všetci vyžadujú **zdokumentované, rizikovo založené zdôvodnenie**. Jediná kvantitatívna regulačná kotva je mriežka WHO.

**Horizontálne pravidlo WHO** ✅:

> "EDLMs should be arranged in a grid fashion along the width and length of the area so that the area is reasonably covered, with EDLMs located every 5–10 metres."

— [WHO TRS 961 Annex 9, Supplement 8, *Temperature mapping of storage areas*, §2.2.6 Step 5](https://cdn.who.int/media/docs/default-source/medicines/norms-and-standards/guidelines/distribution/trs961-annex9-supp8.pdf). Veľmi veľké zariadenia môžu odôvodniť rozstup 20–30 m.

**Vertikálne pravidlo WHO** ✅: pri výške stropu **≤ 3,6 m** loggery **priamo nad sebou v troch výškach — podlaha, 1,2 m a 3,0 m**. Nad 3,6 m viacero vertikálnych polí; pracovný príklad pre 6 m vysokú miestnosť používa **0,3 m, 1,8 m, 3,6 m a 5,4 m**. Doslovne: *„If the ceiling height is 3.6 metres or less, position EDLMs directly above one another at high, medium and low level"* (§2.2.6 Step 5).

**Počty od dodávateľov** (prezentuj ako prax, nie ako požiadavku):

| Priestor | Počet senzorov | Zdroj |
|---|---|---|
| Malá komora < 20 m³ (~700 cu ft) | **16 senzorov** = 1 stred + 8 rohov + 6 stredov stien (jeden na stranu) + 1 vedľa riadiaceho senzora | [Vaisala](https://www.vaisala.com/en/blog/2020-10/video-qa-guidance-warehouse-mapping-sensor-placement), citujúc ISPE Good Practice Guide |
| Malé komory a zariadenia | **9–15 loggerov** (štruktúrovaná mriežka) | [Kaye Instruments](https://www.kayeinstruments.com/en/news/blog-post/guidelines-for-selecting-sensor-quantity-and-optimal-placement-in-area-temperature-mapping) |
| Chladiace miestnosti a riadené priestory | **15–30 loggerov** podľa využiteľného objemu a zložitosti prúdenia | [Kaye](https://www.kayeinstruments.com/en/news/blog-post/guidelines-for-selecting-sensor-quantity-and-optimal-placement-in-area-temperature-mapping) |
| Sklady a veľké priestory | **≈ 1 logger na 25 až 50 m²**, upravené o rizikové faktory | [Kaye](https://www.kayeinstruments.com/en/news/blog-post/guidelines-for-selecting-sensor-quantity-and-optimal-placement-in-area-temperature-mapping) |
| Malé mrazničky | minimálne **3×3 vertikálna mriežka (9 loggerov)** | [Eupry](https://eupry.com/iq-oq-pq/freezers/) |
| Stredné walk-in mrazničky | **5×3 až 5×4 mriežka (15–20 loggerov)** | [Eupry](https://eupry.com/iq-oq-pq/freezers/) |
| Väčší sklad | *„a smaller area may be covered by as little as 15 data loggers, while larger warehouses can require more than 300"* | [Eupry warehouse mapping guide](https://eupry.com/temperature-mapping/warehouse-mapping-guide/) |

**Štandardné rozloženia:** **9-bodové** = matica 3×3 pre malé komory (4 rohy, 4 stredy stien, 1 stred), s vertikálnym zopakovaním, ak výška presahuje 2–3 m; **27-bodové** = 3×3×3 pre väčšie chladiace miestnosti, rozstup delením každého rozmeru na tretiny ([Kaye](https://www.kayeinstruments.com/en/news/blog-post/guidelines-for-selecting-sensor-quantity-and-optimal-placement-in-area-temperature-mapping)).

**Vertikálne stohy:** minimálne **3 senzory na stoh** (nízko, stred, vysoko); 2-senzorové stohy len pri výške pod **2,5 m**. *„Sensors should be placed in three planes in three dimensions"* (pripísané USP General Chapter <1079>). **Bočný rozstup medzi stohmi: max 20 m pre ambientné sklady** (WHO pripúšťa 30 m pre veľmi veľké), **max 8 m pre chladiace miestnosti a mrazničky** ([Vaisala](https://www.vaisala.com/en/blog/2020-10/video-qa-guidance-warehouse-mapping-sensor-placement)).

**Kaye vlastná výhrada, ktorú si prisvoj:** *„There is no universal calculation model; defensible studies combine grid-based layouts with risk-based placement strategies."* ISPE to isté: *„A rationale that describes the location and number of sensors to be used should be developed"*, schválené Quality a vlastníkom systému; ISPE cituje francúzsku normu **NF X15-140** pre komory do ~700 cu ft ([ISPE white paper](https://ispe.org/pharmaceutical-engineering/white-papers/controlled-temperature-chamber-mapping)). MHRA hovorí len: *„Use calibrated monitoring probes in sufficient numbers dependant on the size of the storage area."*

**Praktický oceňovací model odvodený z WHO**, ktorý vieš použiť na kalkuláciu ponuky:
`N ≈ (ceil(L/8) × ceil(W/8)) × počet_výškových_úrovní`, kde počet úrovní = 3 pre miestnosti ≤ 3,6 m a 4+ nad, plus extra senzory na identifikovaných rizikových bodoch (dvere, výustky HVAC, vonkajšie steny, radiátory, riadiaci senzor). Reprodukuje to údaj ~1 logger / 25–50 m² pre typický sklad s tromi úrovňami. **Rozstup sprísni z 10 m na ~8 m a v chladiacich miestnostiach/mrazničkách na 5 m**, lebo tam sú gradienty strmšie a absolútne tolerančné pásmo užšie.

### Ako dlho merať

| Typ priestoru | Trvanie | Zdroj |
|---|---|---|
| Teplotne riadené priestory (chladiace miestnosti, mrazničky, komory) | *„for between 24 and 72 hours, or longer if justified"* | WHO Supp 8 §2.2.6 Step 9 ✅ |
| Ambientné/skladové priestory | *„a minimum of seven consecutive days – including five working days and two weekend days"* | WHO Supp 8 §2.2.6 Step 9 ✅ |
| Kvalifikačné minimum (OQ mapovanie) | *„The whole area should be monitored for a period of at least 24 hours using EDLMs"*, PQ opakuje ten istý protokol **pri plnom zaťažení** | [WHO Supp 7 §2.5.6 Table 8, §2.6.2](https://cdn.who.int/media/docs/default-source/medicines/norms-and-standards/guidelines/distribution/trs961-annex9-supp7.pdf?sfvrsn=3c9a94ed_2) ✅ |
| ISPE | *„at least 24 hours (include time to capture defrost cycles and switchover of redundant systems)"* | [ISPE](https://ispe.org/pharmaceutical-engineering/white-papers/controlled-temperature-chamber-mapping) |
| Mrazničky (dodávateľ) | prázdna komora (OQ) min. 24 h; zaťažená (PQ) 48–72 h; musí zahrnúť aspoň jeden odmrazovací cyklus | [Eupry](https://eupry.com/iq-oq-pq/freezers/) |
| PQ monitorovacieho systému (dodávateľ) | celý proces IQ/OQ/PQ *„typically requires 4–12 weeks, with PQ specifically requiring 4–8 weeks of extended monitoring"* | [SenseAnywhere](https://www.senseanywhere.com/what-are-the-iq-oq-and-pq-phases-of-temperature-monitoring-system-qualification/) |

**Sezónnosť** ✅: *„At least two temperature-mapping studies may be needed in each area to observe the effect of seasonal variation"* — jedna v najteplejšej, jedna v najchladnejšej sezóne (WHO Supp 8 §2.2.4). WHO poznamenáva, že to **typicky nie je potrebné pre chladiace miestnosti a mrazničky**. MHRA uvádza, že mapovanie sa bežne robí v **januári/februári a júli/auguste**. ISPE rozhoduje o sezónnom testovaní posúdením rizík porovnaním vonkajších podmienok s **95 % letnými/zimnými návrhovými podmienkami**.

**Redundantné chladiace jednotky** ✅: štúdia musí zahrnúť obdobie prevádzky s **každou jednotkou bežiacou samostatne** (WHO Supp 8 §2.2.6).

**Prázdny vs zaťažený** (ISPE): treba zvážiť dva scenáre — *„empty store with maximum specified load arriving"* a *„store with maximum load"* obmedzujúce prúdenie vzduchu.

**Komerčne čistý balík:** chladiace miestnosti/chladničky/mrazničky = **72 h** (pokryje ≥ 2 odmrazovacie cykly), ambientný sklad = **7 dní, dvakrát ročne (leto + zima)**. Spĺňa to minimá WHO s rezervou a dá sa to ľahko obhájiť. **A keďže WHO vyžaduje prázdnu (OQ) aj zaťaženú (PQ) štúdiu, prvá kvalifikácia nového skladu reálne znamená 2 × 7 dní plus 2 sezónne opakovania — to je zmysluplný fakturovateľný rozsah, nie jedna návšteva.**

### Kde presne senzory umiestniť

Mriežka dáva základné pokrytie; **rizikovo založené doplnky na dverách, prívodoch/spiatočkách HVAC, vonkajších stenách, radiátoroch, v podstrešnom priestore a na vrchu regálov nájdu skutočné teplé a studené body.**

MHRA ✅: umiestnenie sa má sústrediť na *„areas that experience the extremes of fluctuations"* a zahrnúť potenciálne problémové miesta ako **radiátory, okná, dvere a vysoko uložené miesta**; dáta *„should be recorded … to determine the most appropriate positions for the permanent monitoring probes and should cover the areas that have the widest temperature fluctuations"*.

ISPE: umiestňovať *„where product will be stored"* a kde *„changes in the conditions are likely to be found during use"*; na miestach reprezentujúcich *„maximum and minimum temperatures"*; a vedľa riadiaceho senzora *„where appropriate"*.

WHO pre malé zariadenia (chladničky, mrazničky, skrine) ✅: *„EDLMs should be used to record temperatures at the top and bottom of the cabinet as well as at the rear and front"* (Supp 7 §2.7).

Kaye: *„Concentrate density in high-risk zones (doors, HVAC interfaces, perimeter walls)"*; umiestňovať v reprezentatívnych výškach uloženia produktu; posúdiť vertikálnu stratifikáciu; **zdokumentovať všetky rozhodnutia o umiestnení so sledovateľnosťou.**

Mrazničky, vysokorizikové zóny (dodávateľ): *„door area, compressor side, top shelf, load center"*.

**CDC (americký zdroj, užitočná prax, nie európske právo):** *„Place the buffered probe of the DDL in the center of the unit with the vaccines surrounding it"*; vyhýbať sa umiestneniu *„near the unit's walls, floor, vent, ceiling, or door"*; aktívny displej držať mimo jednotky ([CDC](https://www.cdc.gov/vaccines/hcp/storage-handling/monitoring-equipment.html)).

**Napätie, ktoré musíš vedieť vysvetliť zákazníkovi:** CDC hovorí „stred jednotky, obklopený produktom" (chráni pred falošnými alarmami, reprezentuje produkt), GDP hovorí „extrémy fluktuácií" (chráni pred nedetegovanými odchýlkami). **Pre GDP sklad vyhráva európske pravidlo.** Pre malú chladničku je bežný kompromis jedna bufferovaná sonda v strede **plus mapovací dôkaz, že najhoršia polica ostáva v rozsahu.**

**Predajná asymetria, ktorú treba pomenovať:** *mapovacie loggery idú všade vrátane najhorších miest; trvalé monitory idú len na najhoršie miesta plus jeden reprezentatívny referenčný bod.* Mapovací report je dokument, ktorý mení „odhadli sme to" na „odmerali sme to" — a to je presne ten inšpekčný nález, ktorému sa GDP veľkodistribútor chce vyhnúť.

⚠️ Žiadny zdroj neuvádza numerickú minimálnu vzdialenosť od steny, výparníka alebo dverí pre umiestnenie loggera v ambientných skladoch (dodávatelia bežne používajú 20–50 cm, ale citovateľný publikovaný údaj sa nenašiel).

### Presnosť a neistota: kde sa rozhoduje, či tvoj senzor obstojí

**WHO špecifikácia pre EDLM** ✅: loggery musia mať *„a US National Institute of Standards and Technology (NIST)-traceable 3-point calibration … with a guaranteed error of no more than ± 0.5 °C at each calibration point"*; kalibračné body sú **jeden pod minimom rozsahu, jeden v strede, jeden nad maximom** (Supp 8 §2.1).

**WHO interval záznamu pre mapovanie** ✅: *„1 minute to 15 minutes (maximum)"* (Supp 8 §2.1). **Rozsah merania EDLM** ✅: *„from −30.0 °C to +60.0 °C"*, s používateľsky programovateľnou periódou vzorkovania v rozsahu 1 až 15 minút alebo viac (Supp 7 §2.1).

**ISPE pravidlo vhodnosti na účel — najdôležitejšie číslo v tejto sekcii** ✅: zariadenie s presnosťou **„± 2 °C" „would not be appropriate"**, zatiaľ čo **„± 0.3 °C"** je vhodné pre rozsahy **2–8 °C** ([ISPE](https://ispe.org/pharmaceutical-engineering/white-papers/controlled-temperature-chamber-mapping)).

**ISPE metóda guard bandu / rozhodovacích limitov** pre nakladanie s neistotou merania proti akceptačným kritériám ✅:

```
GB = k × u_system,  kde k > 2.00
Dolný rozhodovací limit:  DL ≥ Limit + [GB − Usr]
Horný rozhodovací limit:  DL ≤ Limit − [GB − Usr]
```

— [ISPE white paper](https://ispe.org/pharmaceutical-engineering/white-papers/controlled-temperature-chamber-mapping)

**Toto je najsilnejší technický predajný argument, aký máš, a treba ho vedieť vysvetliť na papieri.** So senzorom s neistotou ±1 °C a k = 2 vychádza GB = 2 °C, takže komora 2–8 °C by sa musela reálne držať v pásme 4–6 °C, aby sa dal preukázať súlad — **prevádzkovo nemožné okno.** Presne preto je lacný generický LoRaWAN senzor s ±1 °C obhájiteľný pre ambientný sklad 15–25 °C (pásmo 10 K), ale **ťažko obhájiteľný pre chladničku 2–8 °C** a nepoužiteľný pri −80 °C bez vhodnej sondy (väčšina interných LoRaWAN senzorov nejde pod −40 °C). **Tvoje Pt1000 + MAX31865 riešenie je práve odpoveďou na tento argument** — a guard band je spôsob, ako ho zákazníkovi vyčísliť namiesto toho, aby si sa hádal o cene.

**Trvalé nainštalované senzory v kvalifikovanom priestore** ✅: všetky *„critical controllers and sensors should have: a current calibration certificate, with the certificate available; a calibration that is traceable to national standards; an attached calibration tag"* (WHO Supp 7 §2.5.2). **Kalibračný štítok na senzore** je detail, ktorý inšpektori kontrolujú a ktorý generickí dodávatelia vynechávajú.

⚠️ **Neexistuje európske usmernenie s explicitným číslom presnosti pre trvalé GDP monitorovacie senzory.** EU GDP k číslam mlčí. ±0,5 °C pochádza z WHO (mapovanie) a CDC (americký vakcínový monitoring: *„Recommended uncertainty of ±0.5 °C (±1 °F)"*, rekalibrácia *„every 2 to 3 years, or according to the manufacturer's recommended schedule"*). **Cituj ako konvenciu, nie ako európsku požiadavku.** WHO ±0,5 °C je špecifikácia **mapovacieho** loggera; pre trvalé monitory neexistuje samostatné publikované číslo a praktická konvencia je, že trvalé monitory majú byť aspoň tak dobré ako mapovacie loggery pre tú istú komoru.

### Dvere: pravidlo 30 minút

**Otvorenie dverí je očakávaná udalosť, nie odchýlka** — a WHO to robí explicitným s **30-minútovým kritériom zotavenia** ✅:

> "temperature should be maintained within the defined temperature limits except for a maximum of 30 minutes following the door opening"

— WHO Supp 8 §2.2.6 Step 4

a kvalifikačné akceptačné kritérium pre test otvorenia dverí ✅:

> "the temperature recorded by all the EDLMs located inside the storage area should return to being within the specified temperature range (e.g. 2.0 °C to 8.0 °C) within 30 minutes after the door(s) are closed"

— WHO Supp 7 §2.6.3 Table 10

Návrh testu ✅: *„The door-open period used for the test should represent actual door opening behaviour observed during routine operations"* (Supp 7 §2.6.3). ISPE dodáva, že testovanie dverí má zahŕňať *„doors held open for the maximum specified time in extreme environmental conditions"*.

**Zmierňovanie falošných alarmov z dverí:** ISPE ponúka dve možnosti — zaviesť *„time delays into the alarm system"* **alebo** umiestniť *„sensor into a container of thermal fluid"*. Vaisala pridáva: minimalizovať trvanie a účel otvárania, umiestniť mapy skladu na dvere komôr, aby sa skrátilo hľadanie, oneskorenia alarmu, upravené setpointy, školenie personálu o dopade otvárania dverí ([Vaisala, *Managing GxP Monitoring System Alarms*](https://www.vaisala.com/en/blog/2023-08/managing-gxp-monitoring-system-alarms)). NHS SPS: *„Audible alerts may be set if a refrigerator or walk-in cold store door has been left open for a set period"* ([NHS SPS](https://sps.nhs.uk/articles/establishing-alerts-for-temperature-monitoring-systems/)). Očakávanie pre mrazničky (dodávateľ): nárast ≤ 10 °C pri krátkom prístupe (30–60 s) so zotavením na setpoint do 15–30 minút.

**Dva závery, ktoré predávaj.** Prvý: **anotuj, nemaž.** 30-minútové okno WHO dáva objektívne, citovateľné pravidlo na klasifikáciu nárastu spojeného s dverami ako očakávanej udalosti namiesto odchýlky — **za predpokladu, že udalosť je zaznamenaná** (kontakt na dverách alebo záznam operátora) a že sa preukáže zotavenie do 30 minút. Mazanie dátových bodov by bol problém integrity dát. Druhý: **kontakt na dverách popri teplotnom senzore teda nie je hračka, ale dôkaz, ktorý robí 30-minútové pravidlo auditovateľným.** Pre LoRaWAN inštalatéra je to priamočiary upsell s regulačným zdôvodnením.

⚠️ Nenašlo sa žiadne usmernenie, ktoré by povoľovalo **vylúčiť** dáta z otvorenia dverí z MKT alebo zo štatistiky odchýlok. Bezpečné čítanie: vylúčenie nie je sankcionované; anotácia so zdôvodnením áno.

### Alarmové limity a oneskorenia

**Žiadny regulátor nepublikuje numerické oneskorenia alarmov.** Obhájiteľný postup publikovaný NHS SPS aj Vaisalou je: **nastav alarmový prah vnútri špecifikačného limitu a začni bez oneskorenia, eskaluj na 2 minúty a potom na 5 minút len ak sa objavia falošné alarmy** — pričom každú voľbu zdokumentuj.

NHS SPS ✅: *„For example, a refrigerator should not exceed a temperature of 8 °C, setting the alarm threshold at 7.5 °C allows a user to respond before an excursion happens."* · *„A delay may be set on the alarm to allow for acceptable small excursions without an alert, e.g. the transient rise in temperature associated with a fridge door opening."* · *„if there is a function to silence an alarm (e.g. while stocking a fridge) the alarm should sound again after a defined period (e.g. 10 mins)."* ([NHS SPS](https://sps.nhs.uk/articles/establishing-alerts-for-temperature-monitoring-systems/))

Vaisala ✅: *„start with no delays and see if you get nuisance alarms. If you're getting nuisance alarms, move to a two-minute delay. If you're still getting nuisance alarms, move out to a five-minute delay."* Vaisala výslovne hovorí, že **neexistuje štandardné oneskorenie alarmu** — závisí od vlastností produktu a tolerancie rizika. A dôvod, prečo na tom záleží: *„if alarms keep going off and they're not important, people begin to ignore them."* Odporúča **konzistentné oneskorenia a buffery naprieč všetkými komorami**, aby sa znížila logistická zložitosť ([Vaisala](https://www.vaisala.com/en/blog/2023-08/managing-gxp-monitoring-system-alarms)).

**WHO kvalifikačná požiadavka na testovanie alarmov** ✅: *„For each test, record the alarm settings and trigger the desired alarm event. Confirm that the alarm system is activated"*; musia byť zdokumentované **High alarm setting** aj **Low alarm setting** (Supp 7 §2.5.5 Table 7).

**Reťaz zdôvodnenia, ktorú napíš do návrhového dokumentu monitoringu** — a ktorá je zároveň druhým komerčným ospravedlnením mapovania: tepelná zotrvačnosť náplne (zmeraná pri mapovaní) → pozorovaný čas zotavenia po reprezentatívnych otvoreniach dverí (zmeraný pri teste dverí) → **oneskorenie alarmu zvolené tak, aby bolo kratšie než čas, za ktorý teplota produktu dosiahne špecifikačný limit, ale dlhšie než typické tranzienty dverí.** Keďže WHO limituje zotavenie na 30 minút, oneskorenie alarmu má byť výrazne pod 30 minút; 2–5 minút pre vzduchový senzor (alebo 15–30 min pri glykolom bufferovanej sonde, ktorá má vlastné tepelné oneskorenie) je bežný rozsah implikovaný týmito zdrojmi.

⚠️ **Žiadny citovateľný zdroj nedáva numerické oneskorenia podľa triedy zariadenia** (napr. „chladnička 15 min, mraznička 30 min, ambient 60 min"). Akákoľvek takáto tabuľka v tvojej ponuke musí byť označená ako **tvoja vlastná rizikovo založená konvencia**, nie ako usmernenie.

### Bufferované sondy vs vzduchové senzory

Buffering existuje na to, aby senzor čítal niečo blízke **teplote produktu** namiesto teploty vzduchu, čím potláča tranzienty z dverí a odmrazovania. Najjasnejšia publikovaná požiadavka je CDC (povinná bufferovaná sonda pre skladovanie vakcín): *„Detachable probe that best reflects vaccine temperatures (e.g., a probe buffered with glycol, glass beads, sand, or Teflon®)"*, s definíciou bufferingu ako ponorenia *„in a vial filled with liquid (e.g., glycol, ethanol, glycerin), loose media (e.g., sand, glass beads), or a solid block of material"*; interval logovania pre bufferované zariadenia *„at least every 30 minutes"* ([CDC](https://www.cdc.gov/vaccines/hcp/storage-handling/monitoring-equipment.html)).

⚠️ **Nenašiel sa žiadny európsky regulačný text, ktorý by nariaďoval bufferované sondy alebo zakazoval vzduchové senzory.** Voľba je zdokumentované rizikové rozhodnutie. **Povedz to zákazníkovi rovno.**

**Rozhodovacie pravidlo, ktoré z prameňov plynie:** používaj **vzduchové senzory na mapovanie** (chceš vidieť tranzienty a skutočné gradienty — buffering by skryl presne to správanie teplých/studených bodov, za ktorého nájdenie zákazník platí), a zvažuj **bufferované sondy na trvalý monitoring chladničiek a malých komôr**, kde je riadenou veličinou teplota produktu a kde nebufferované vzduchové senzory generujú falošné alarmy. Pre veľké ambientné sklady je buffering zbytočný, pretože tepelná časová konštanta miestnosti je aj tak dlhá. **Bufferovaná sonda je funkčne ekvivalentná dlhému oneskoreniu alarmu, takže aplikovať oboje naraz riskuje zamaskovanie skutočnej odchýlky** — návrhový dokument má zvoliť jedno a zdôvodniť to.

⚠️ **Žiadne publikované usmernenie neurčuje objem glykolovej fľaštičky.** Bežná prax dodávateľov je 10–30 ml, citovateľný regulačný alebo normatívny údaj sa nenašiel. Dokument NIST *Guidelines for Storage and Temperature Monitoring of Refrigerated Vaccines* sa objavil vo vyhľadávaní a je pravdepodobne najlepším zdrojom kvantitatívnych dát o časových konštantách bufferov; nebol načítaný.

### Výpadok napájania a holdover time

WHO vyžaduje test výpadku napájania ako súčasť kvalifikácie, s cieľom stanoviť **holdover time** ✅:

> "The length of time during which the installation can maintain the specified temperature range following a power failure – this is known as the holdover time"

— WHO Supp 7 §2.5.7

Dve sankcionované metódy ✅: **Metóda 1 (beh do odchýlky):** *„When the temperature inside the storage area goes out of range, then the storage area and equipment can be powered back on"*. **Metóda 2 (pevné obdobie):** *„the power is stopped for a predefined period – for example, two hours"*.

Záložné zariadenie — najjasnejšie publikované odporúčanie je CDC ✅: *„Have at least one backup DDL or TMD in case a primary device breaks, malfunctions, or requires calibration."* Očakávanie pre ULT mrazničky (dodávateľ): *„Typical target is 4–8 hours for ULT freezers to allow emergency response time"*.

**Holdover time je číslo, ktoré určuje SLA eskalácie alarmov.** Ak chladiaca miestnosť udrží 2–8 °C tri hodiny bez napájania, výjazdová procedúra mimo pracovnej doby musí garantovať reakciu výrazne pod tri hodiny. **LoRaWAN systém s batériovými senzormi a gatewayom na UPS má tu skutočnú výhodu oproti drôtovým napájaným systémom, a mapovací/kvalifikačný report je dokument, ktorý ju kvantifikuje.** To je argument, ktorý predáva tvoj hardvér lepšie než akýkoľvek datasheet.

**Nezávislý batériový logger v každom kritickom priestore** je praktická odpoveď na poruchový režim, ktorý sieťový systém nemôže pokryť: výpadok gatewaya alebo siete. ⚠️ **Nenašiel sa žiadny európsky ani WHO text vyžadujúci nezávislé záložné dataloggery** v kritických priestoroch. Prax je rozšírená a CDC ju odporúča, ale **prezentuj ju ako rizikovo založenú dobrú prax, nie ako požiadavku.** ⚠️ WHO tiež neuvádza minimálny prijateľný holdover time; vyžaduje len, aby bol zmeraný a zdokumentovaný.

### Kedy premapovať

Spúšťačovo aj periodicky.

- **WHO** ✅: *„subsequent mapping exercises may also be required periodically – for example, every three years"*; ďalšie spúšťače: **významné modifikácie, zmeny vzoru nakladania, úpravy zariadení a nevysvetlená variabilita** (Supp 8 §1.1). Supplement 8 §1.1 zároveň uvádza posúdenia po výpadku napájania medzi spúšťačmi.
- **EU GDP §3.2.1** ⚠️: *„The mapping exercise should be repeated according to the results of a risk assessment exercise or whenever significant modifications are made to the facility or the temperature controlling equipment."*
- **MHRA** ✅: premapovanie *„whenever significant modifications are made to the facility or the temperature controlling equipment"*, a posúdenie rizík *„would also define and justify the regularity of any future mapping exercises"*.
- **WHO Supp 7 §1.1** ✅: *„Further qualification exercises should be conducted whenever significant modifications are made to the installation, or to the way in which it is used"* — **žiadny kalendárny interval rekvalifikácie sa neuvádza.**
- Dodávateľská prax: premapovanie *„every 1–3 years depending on risk"* a po premiestnení, opravách, zmenách dispozície alebo poruche zariadenia ([ELPRO](https://www.elpro.com/en/temperature-monitoring/qualification-mapping)); alebo spúšťané zmenami dispozície, HVAC, skladovej kapacity, prípadne trendovými dátami naznačujúcimi nestabilitu ([Eupry](https://eupry.com/temperature-mapping/warehouse-mapping-guide/)).

**Obhájiteľná komerčná pozícia:** **3-ročné periodické premapovanie ako predvolené (vlastný príklad WHO), s okamžitým premapovaním pri zmene dispozície regálov, výmene alebo zmene HVAC/chladenia, zmene teplotnej triedy skladovaného tovaru, rozšírení priestoru alebo pri vzore nevysvetlených odchýlok.** Rámcovanie 3-ročného cyklu ako „vlastného príkladu WHO" z neho robí opakovanú službu. **Ale pozor: EU GDP tri roky NEuvádza. Firma, ktorá tvrdí „európske právo vyžaduje premapovanie každé 3 roky", preháňa.** Čestné rámcovanie: *„EU GDP vyžaduje rizikovo založenú periodicitu; WHO ponúka tri roky ako pracovný príklad."* ⚠️ Žiadny európsky zdroj neuvádza maximálny interval — veľkodistribútor, ktorý si so zdokumentovaným posúdením rizík stanoví 5 rokov, nie je automaticky v nesúlade.

### Výstupy mapovacej štúdie

**Dva riadené dokumenty: schválený mapovací protokol pred štúdiou a recenzovaný mapovací report po nej**, ktorého ústredným výstupom je **zónovací diagram bezpečných/nebezpečných miest uloženia** a **zdôvodnený zoznam trvalých monitorovacích pozícií.**

Štruktúra reportu podľa WHO ✅ (Supp 8 §2.2.7): Introduction; Summary with deviations; Conclusions and recommendations; Annexes obsahujúce **prieskum lokality, hárky surových dát, grafy, výsledky analýzy, kalibračné certifikáty a CAPA formuláre**, ak sú potrebné. Kľúčové výstupy WHO: (1) zdokumentovaný mapovací protokol **pred začiatkom štúdie**; (2) hotový mapovací report **so schválením v peer review**; (3) **vizuálny zónovací diagram zobrazujúci bezpečné a nebezpečné miesta uloženia.**

**Predajná sada pre jednotlivca:** (1) mapovací protokol s akceptačnými kritériami a výkresom rozloženia senzorov; (2) export surových dát (CSV) plus grafy na logger; (3) tabuľka min/max/mean/MKT na logger; (4) heat map alebo zónovací výkres teplých/studených bodov; (5) odporúčanie umiestnenia trvalých senzorov so zdôvodnením; (6) odchýlky/CAPA; (7) príloha s kalibračnými certifikátmi; (8) schvaľovacia/peer-review strana. **Položky 4 a 5 sú tie, ktoré priamo zdôvodňujú následný predaj LoRaWAN senzorov.**

Kalibračné certifikáty sú súčasťou výstupu a ich platnosť je nosná: *„Expired certificates invalidate associated data"* ([Eupry](https://eupry.com/iq-oq-pq/freezers/)) — konzistentné s požiadavkou WHO na kalibračné certifikáty.

⚠️ WHO nepredpisuje šablónu pre heat map a žiadny regulátor nešpecifikuje štatistické spracovanie (či reportovať smerodajnú odchýlku alebo len min/max).

### Mean Kinetic Temperature (MKT)

MKT je Haynesovou/Arrheniovou funkciou vážená jediná ekvivalentná teplota. Regulačná definícia pochádza z **ICH Q1A(R2)** ⚠️ (citované druhotne z dokumentácie dodávateľov, ktoré ju pripisujú Q1A(R2); **ICH Q1A(R2) nebolo načítané priamo — over pred citovaním v zákazníckom materiáli**):

> "a single derived temperature that, if maintained over a defined period of time, affords the same thermal challenge to a drug substance or drug product as would be experienced over a range of both higher and lower temperatures"

— [Rotronic RMS, Mean Kinetic Temperature](https://service.rotronic.com/manual/mean-kinetic-temperature.html)

Základ vzorca je *„the formula of J. D. Haynes (J. Pharm. Sci., 60:927-929, 1971)"*. Konvenčné konštanty ✅: *„LogTag® Analyzer uses an activation energy value of 83.144 KJ/mol for the calculation"*; plynová konštanta *„0.0083144 KJ/mol K"* ([LogTag Analyzer Help](https://help.logtagrecorders.com/LTA3/Topics/UserGuide/Chapter2-Using_Analyzer/Mean_Kinetic_Temperature.htm)).

**Vzorec, rozpísaný:**

```
MKT (v kelvinoch) = (ΔH / R) / −ln[ ( Σᵢ tᵢ · e^(−ΔH/(R·Tᵢ)) ) / ( Σᵢ tᵢ ) ]

pri rovnakých časových intervaloch:
T_K = (ΔH/R) / −ln( (Σᵢ e^(−ΔH/(R·Tᵢ))) / n )

ΔH = 83,144 kJ/mol
R  = 0,0083144 kJ·mol⁻¹·K⁻¹
⇒ ΔH/R = 10 000 K presne
Tᵢ v kelvinoch, n = počet meraní. Na °C: odpočítaj 273,15.
```

**Prečo práve 83,144 kJ/mol:** nie je to náhodné — robí to **ΔH/R = 10 000 K presne**, a preto sa táto hodnota stala de facto priemyselným štandardom v softvéri loggerov. Je to pekný detail do zákazníckeho vysvetľujúceho materiálu. MKT *„exceeds arithmetic mean temperature by accounting for the Arrhenius equation"* — je vždy **vyššia** než aritmetický priemer. Vlastné hodnoty ΔH sú v analytickom softvéri konfigurovateľné a softvér označí, keď bola použitá neštandardná ΔH.

**Praktické rozdelenie úloh pri hodnotení odchýlok, ktoré musíš vedieť vysvetliť:** **MKT odpovedá na otázku „bola kumulatívna tepelná záťaž počas skladovacieho obdobia prijateľná?"** (napr. sklad držaný na 15–25 °C počas roka, alebo ambientný sklad, ktorý v horúcich popoludniach presiahol 25 °C). **MKT NEodpovedá na otázku „zamrzol produkt?" ani „bola chladnička 4 hodiny na 15 °C?"** — akútne bodové odchýlky proti absolútnemu limitu (2–8 °C, „do not freeze") sa musia hodnotiť proti min/max a proti produktovo špecifickým excursion dátam, nie spriemerovať. **Pre monitorovací produkt je MKT najlepšie pozicionovaná ako reportovacia funkcia pre ambientné/CRT sklady**, plus min/max/trvanie nad limitom pre chladový reťazec.

⚠️ **USP General Chapter <1079.2>** *„Mean Kinetic Temperature in the Evaluation of Temperature Excursions During Storage and Transportation"* je najexplicitnejší publikovaný text o tom, **kedy sa MKT smie a nesmie použiť**; PDF na usp.org vrátilo HTTP 403 a nedalo sa citovať. **Toto je najcennejšia chýbajúca citácia v celom materiáli** — dostupné aj cez APEC toolkit. ⚠️ Nenašiel sa žiadny zdroj, ktorý by hovoril „použi MKT namiesto jednoduchého priemeru pri hodnotení odchýlok" — voľba je vecou zdokumentovanej firemnej politiky plus stabilitných dát produktu.

### Definície skladovacích podmienok podľa EMA

**EMA Guideline on declaration of storage conditions, CPMP/QWP/609/96 Rev. 2 (19. november 2007)** viaže každé označenie na stabilitné dáta, ktoré ho podporujú ⚠️ (znenie z fetchu):

| Testovacia podmienka | Povinné označenie | Dodatočné označenie |
|---|---|---|
| 25 °C/60 % RH dlhodobo **s** 40 °C/75 % RH zrýchlene | „None" | „Do not refrigerate or freeze" |
| 25 °C/60 % RH dlhodobo **s intermediárnymi** podmienkami | „Do not store above 30 °C or Store below 30 °C" | „Do not refrigerate or freeze" |
| 25 °C/60 % RH **len** dlhodobo | „Do not store above 25 °C or Store below 25 °C" | „Do not refrigerate or freeze" |
| **5 °C ± 3 °C (long term)** | „Store in a refrigerator or Store and transport refrigerated" | „Do not freeze"; SPC a písomná informácia majú uvádzať teplotný rozsah, napr. (2 °C to 8 °C) |
| „Below zero" | „Store in a freezer or Store and transport frozen" | — |
| bez osobitných podmienok | „This medicinal product does not require any special storage conditions." | — |

— [EMA CPMP/QWP/609/96 Rev 2](https://www.ema.europa.eu/system/files/documents/scientific-guideline/wc500003468_en.pdf); [aktuálna verzia](https://www.ema.europa.eu/en/documents/scientific-guideline/guideline-declaration-storage-conditions_en.pdf)

**Prevádzkové pásma, ktoré musíš obslúžiť:** ambient/CRT **15–25 °C**, chladnička **+2 až +8 °C**, mraznička **−25 až −15 °C** (typicky pre „Store in a freezer"), ULT **−80 °C ± 10 K** (produktovo špecifické, definované v SPC, napr. mRNA vakcíny). **Keďže testovacia podmienka EMA pre chladené je „5 °C ± 3 °C", pásmo 2–8 °C je absolútna tolerancia, nie cieľ s rezervou** — preto potrebuje monitoring chladničiek vysokú presnosť a preto sa alarmové limity nastavujú **dovnútra** pásma.

⚠️ **„Store below −15 °C" sa v základnej tabuľke EMA nepodarilo potvrdiť.** Ani aktuálna, ani Rev 2 verzia, ktoré boli načítané, neukazujú prah −15 °C — len „Store in a freezer" / „Below zero". Hodnota −15 °C je bežná v USP a v národnej praxi; **nepripisuj ju usmerneniu EMA bez overenia prílohy.** ⚠️ Všeobecné kapitoly Ph. Eur. definujúce „cold place", „refrigerator", „deep-freeze" neboli načítané; citovateľným európskym zdrojom pre označenia je usmernenie EMA.

---
## Katalóg testov IQ/OQ/PQ: z čoho spravíš šablóny

⚠️ **Poctivosť na úvod:** žiadny európsky regulačný ani normatívny text nevymenúva položky nižšie. Sú to **inžinierska syntéza z požiadaviek klauzúl plus prax dodávateľov.** Zákazníkovi ich prezentuj ako **metódu dodávateľa zdôvodnenú odkazmi na klauzuly**, nie ako citáciu. To je zároveň presne to, čo Annex 15 clause 2.6 predpokladá.

### URS — čo tam musí byť

URS je nariadený Annexom 15 klauzulou 3.2 a Annexom 11 klauzulou 4.4, musí byť rizikovo posúdený a sledovateľný. Pre monitorovací systém musí pokryť trinásť skupín **číslovaných, testovateľných požiadaviek** (každá musí ísť dosledovať k testu):

1. **Rozsah a zamýšľané použitie** — ktoré miestnosti/komory/vozidlá, aké skladovacie podmienky produktu (2–8 °C, 15–25 °C, −20 °C), kontext GMP vs GDP, regulačné odkazy.
2. **Požiadavky na meranie** — merací rozsah, požadovaná presnosť/neistota (napr. ±0,5 °C v pásme 2–8 °C), rozlíšenie, typ senzora, nadväzná kalibrácia na národný etalón, kalibračný interval, doba odozvy, interval logovania (citovaná prax: **5–15 minút**, [ELPRO](https://www.elpro.com/en/temperature-monitoring/qualification-mapping)).
3. **Umiestnenie senzorov** — počet a pozície pevných senzorov, zdôvodnené mapovacím reportom; umiestnenie na najhorších miestach.
4. **Alarmy** — limity (výstražné + akčné), oneskorenia/tolerančné časy, alarm pri odchýlke, pri zlyhaní senzora, pri strate komunikácie, pri nízkej batérii, pri výpadku napájania; potvrdenie alarmu a záznam dôvodu potvrdenia.
5. **Notifikácie a eskalácia** — kanály (SMS/e-mail/app/hlas), zoznamy príjemcov, eskalačné rebríčky a časy, mimo pracovnej doby, potvrdenie doručenia.
6. **Integrita dát / ALCOA+** — kontemporánny záznam, nemožnosť ktoréhokoľvek používateľa zmeniť alebo zmazať surové merania, bezpečné časové pečiatky, audit trail všetkých GMP-relevantných zmien (Annex 11 §9), retenčná lehota, archivácia a čitateľnosť (§17).
7. **Bezpečnosť a riadenie prístupu** — unikátne ID, model rolí (read-only / operátor / QA / admin), politika hesiel, žiadne zdieľané účty, záznam vytvorenia/zmeny/zrušenia prístupových práv (§12.3), zamknutie relácie, TOTP.
8. **Dostupnosť a kontinuita** — autonómne logovanie na senzore/gatewayi pri výpadku siete, hĺbka bufferu, automatický backfill, UPS/batéria, frekvencia záloh a schopnosť obnovy (§7.2), kontinuita pri zlyhaní systému (§16).
9. **Reporting** — periodické reporty, reporty odchýlok, výtlačky (§8.1), exportné formáty, obsah reportu a podpis.
10. **Synchronizácia času** — jeden autoritatívny zdroj času (NTP), časové pásmo a spracovanie letného času, zákaz používateľom nastaviteľných hodín na zariadeniach.
11. **Rozhrania** — LoRaWAN network server, integrácia na WMS/QMS, exportné API.
12. **Regulačné požiadavky** — EU GMP Annex 11 a 15, EU GDP, GAMP 5, a (ak sa týka zákazníka) 21 CFR Part 11, plus pri lekárni vyhláška 129/2012 § 30 ods. 8.
13. **Nefunkčné** — počet senzorov/používateľov, výkon, podporované prehliadače, model hostingu (on-prem vs cloud), podpora a SLA.

⚠️ Žiadny európsky dokument nepredpisuje obsah URS pre monitorovacie systémy; táto štruktúra je zostavená z požiadaviek klauzúl plus praxe dodávateľov a tak ju treba prezentovať.

Vendorský V-model pre teplotný monitoring radí v poradí: **URS → Risk Analysis → Design Specification → IQ → OQ → PQ**, naprieč štyrmi fázami Plan / Build / Test / Use ([ELPRO](https://www.elpro.com/en/temperature-monitoring/qualification-mapping)). **Mapovací report je vstupom do URS/návrhu, nie dodatkom** — výsledky mapovania dávajú *„recommendations for monitoring sensor placement and alarm limits"* ([Eupry](https://eupry.com/temperature-mapping/warehouse-mapping-guide/)).

### IQ — desať položiek

IQ = „všetko je fyzicky a logicky na mieste, správne a zdokumentované". Annex 15 §3.9 predpisuje minimum (overenie inštalácie proti výkresom a špecifikáciám, overenie proti vopred definovaným kritériám, zber a zoradenie prevádzkových a pracovných inštrukcií dodávateľa a požiadaviek na údržbu, kalibrácia prístrojov, overenie materiálov konštrukcie).

- **IQ-01 Inventár hardvéru** — každý senzor/logger, gateway, anténa, server, UPS so výrobcom, modelom, **výrobným číslom**, inventárnym štítkom a fyzickým umiestnením.
- **IQ-02 Overenie umiestnenia senzorov** — umiestnenie každého senzora zdokumentované proti pôdorysu/mapovaciemu reportu, s fotografiou a ID miesta; potvrdenie, že polohy zodpovedajú odporúčaniam mapovacej štúdie.
- **IQ-03 Kalibračné certifikáty** — platný nadväzný certifikát pre každý senzor a pre referenčný etalón, s kalibračnými bodmi, neistotou, akreditáciou vydávajúceho laboratória, dátumom kalibrácie a dátumom platnosti.
- **IQ-04 Verzie firmvéru/softvéru** — zaznamenaná verzia firmvéru senzorov, firmvéru gatewaya, LoRaWAN network servera, aplikácie, OS, databázy. **Toto je konfiguračná baseline**, proti ktorej sa posudzuje budúce riadenie zmien.
- **IQ-05 Konfigurácia siete** — LoRaWAN frekvenčný plán, správa DevEUI/AppEUI/kľúčov, IP/konektivita gatewaya, pravidlá firewallu, porty, TLS, sieťová schéma.
- **IQ-06 Špecifikácia servera** — hardvér alebo VM, CPU/RAM/disk, OS a úroveň patchov, lokalita hostingu (on-prem / dátové centrum / cloudový región), dimenzovanie úložiska a retencie, zdroj NTP, antivírus, zálohovací agent.
- **IQ-07 Overenie inštalácie** — fyzické uchytenie, kabeláž, napájanie, pripojenie na UPS, ponorenie/umiestnenie sondy, kontrola pokrytia/RSSI gatewayom pre každý senzor.
- **IQ-08 Zber dokumentácie** — používateľský manuál, administrátorský manuál, inštalačný manuál, pokyny na údržbu, návrhy SOP, release notes dodávateľa, softvérová licencia, dokumentácia kvality dodávateľa.
- **IQ-09 Prostredie** — okolité podmienky serverovne / umiestnenia gatewaya.
- **IQ-10 Baseline účtov a rolí** — zoznam vytvorených používateľských účtov a priradených rolí pri odovzdaní.

Pre sklady IQ overuje pripravenosť zariadenia cez *„design documentation"*, *„as-built drawings, HVAC schematics, airflow design, and insulation details"*, potvrdenie utilít, *„equipment identification"* **s výrobnými číslami**, *„calibration evidence"* pre referenčné prístroje aj pevné sondy a **záznam konfiguračnej baseline** ([Eupry](https://eupry.com/iq-oq-pq/warehouses/)). Pre monitorovací systém konkrétne: overenie polohy senzorov, testovanie komunikácie, dokumentácia environmentálnych podmienok miesta inštalácie, revízia dokumentácie certifikátov a kalibračných záznamov, a overenie utilít a záložných systémov ([SenseAnywhere](https://www.senseanywhere.com/what-are-the-iq-oq-and-pq-phases-of-temperature-monitoring-system-qualification/)).

### OQ — pätnásť testovacích oblastí s metódou

OQ = „systém robí to, čo má, vrátane limitov a najhorších prípadov" (Annex 15 §3.11). Pre monitorovací systém sú najhoršími prípadmi odchýlky, zlyhanie senzora, výpadok napájania a výpadok siete — a OQ je miesto, kde sa preverujú alarmy, integrita dát, audit trail, riadenie prístupu, záloha/obnova a synchronizácia času.

**A. Overenie presnosti senzora proti referenčnému etalónu.** Umiestni pevný senzor a kalibrovaný referenčný teplomer (nadväzný, s platným certifikátom, s neistotou menšou než preukazovaná tolerancia — typicky aspoň **TUR 4:1**) do stabilného kúpeľa/kalibrátora alebo stabilnej komory. Testuj vo **viacerých bodoch naprieč prevádzkovým rozsahom** (pre aplikáciu 2–8 °C: 0 °C, 5 °C, 10 °C; pre 15–25 °C: 15 °C, 20 °C, 25 °C; pre mrazničky: −25 °C, −20 °C, −15 °C). Akceptačné kritérium: odchýlka od referencie v rámci presnosti uvedenej v URS (typicky ±0,5 °C), zaznamenaná pre každý senzor. Dôkazy: certifikát referencie, surové odčítania, tabuľka odchýlok, pass/fail na senzor.

**B. Testovanie alarmov — ako vyvolať alarm, štyri metódy v poradí preferencie.** Toto je najdôležitejší odsek celého katalógu.

1. **Fyzický podnet (preferovaný, najobhájiteľnejší):** vyber senzor z riadeného priestoru, ponor ho do teplého/ľadového kúpeľa alebo ho vlož do kalibrovaného dry-blocku nastaveného nad/pod alarmový limit a nechaj reálnu nameranú hodnotu prekročiť reálny alarmový limit. Zaznamená **celý reťazec**: meranie → vyhodnotenie limitu → alarmový stav → notifikácia.
2. **Dočasná zmena limitu (druhá najlepšia):** dočasne zúž alarmové limity (napr. nastav horný limit tesne pod aktuálnu okolitú teplotu), aby ich reálna nameraná hodnota prekročila; zdokumentuj zmenu limitu v audit traile a potom ju vráť. Užitočné pre mrazničky, kde je fyzický podnet pomalý.
3. **Výzva otvorením dverí / odpojením napájania:** otvor dvere komory alebo vypni chladenie, kým teplota neprekročí limit — zároveň to preukáže oneskorenie alarmu a zotavenie.
4. **Simulovaná/injektovaná hodnota (najslabšia; prijateľná len ak je zdôvodnená a zdokumentovaná):** testovací/simulačný režim. **Inšpektori sú voči nej skeptickí, lebo obchádza snímací reťazec.** Ak ju musíš použiť, napíš do protokolu, prečo fyzický podnet nebol možný.

Pre každý test zaznamenaj: čas nástupu odchýlky, alarmový limit, nastavené oneskorenie alarmu, čas vyvolania alarmu, čas odoslania notifikácie, čas prijatia notifikácie každým príjemcom, eskalačné kroky, potvrdenie a záznam v audit traile.

**C. Testovanie limitov.** Over horný limit, dolný limit a (ak sa používajú) výstražné vs akčné limity samostatne; over oneskorenie/tolerančný čas alarmu — alarm **nesmie** vzniknúť pri odchýlke kratšej než nastavené oneskorenie a **musí** vzniknúť, keď ho prekročí (testuj obe strany); over hysterézu/návrat do normálu.

**D. Testovanie notifikácií a eskalácie.** Pre každý nakonfigurovaný kanál (e-mail, SMS, push, hlas) a každého príjemcu over doručenie a obsah; over eskalačný rebríček tým, že prvý alarm **nepotvrdíš** a overíš eskaláciu na druhého a tretieho príjemcu po nastavených intervaloch; testuj pohotovostný rozpis mimo pracovnej doby; over, že nedosiahnuteľný príjemca spustí fallback.

**E. Testovanie výpadku napájania.** Odpoj sieťové napájanie od (i) senzora/loggera, (ii) gatewaya, (iii) servera/UPS. Over: lokálne bufferovanie pokračuje, je vyvolaný alarm straty napájania, dáta sa nestratia, po obnovení sa dáta automaticky doplnia so správnymi pôvodnými časovými pečiatkami a sú vhodne označené, a systém sa vráti do normálu bez manuálnej rekonštrukcie.

**F. Testovanie výpadku siete.** Odpoj gateway od internetu / zablokuj LoRaWAN uplink na definovaný čas (napr. 1–4 h). Over: senzor loguje autonómne ďalej; alarm straty komunikácie vznikne po nastavenom timeoute; po obnovení sa všetky bufferované merania prenesú **s pôvodnými časovými pečiatkami**; žiadne duplikáty, žiadne medzery; kapacita bufferu postačuje na dobu autonómie uvedenú v URS.

**G. Testovanie zlyhania/odpojenia senzora.** Fyzicky odpoj alebo odstráň sondu; over, že vznikne **alarm poruchy senzora, nie ticho zamrznutá posledná hodnota.** Testuj aj alarm nízkej batérie.

**H. Testovanie prístupu a rolí.** Pre každú rolu skús povolené aj zakázané akcie a zdokumentuj výsledok (read-only používateľ nesmie zmeniť alarmové limity; operátor nesmie mazať dáta; iba admin smie vytvárať používateľov). Testuj: prihlásenie s platným/neplatným heslom, zablokovanie účtu po n neúspešných pokusoch, zložitosť a expiráciu hesla, timeout relácie, zabránenie duplicitným/zdieľaným ID, okamžité odňatie prístupu pri deaktivácii používateľa. Over, že vytvorenie, zmena a zrušenie prístupových práv sú zaznamenané (Annex 11 §12.3). Pri TOTP: over registráciu, over, že bez druhého faktora sa nedá prihlásiť, a over obnovovací postup pri strate zariadenia.

**I. Testovanie audit trailu.** Vykonaj sadu zámerných GMP-relevantných akcií (zmena alarmového limitu, potvrdenie alarmu s dôvodom, vytvorenie/deaktivácia používateľa, zmena konfigurácie senzora, pokus o zmazanie dát) a over, že každá sa objaví v audit traile s: kto, čo (stará hodnota → nová hodnota), kedy (s časovým pásmom) a prečo (dôvod). Over, že audit trail **nie je editovateľný ani zmazateľný žiadnym používateľom vrátane admina**, je čitateľný/exportovateľný a dá sa filtrovať a vytlačiť. **Doplň test podľa draft 12.9: over, že export je štruktúrovaný a strojovo spracovateľný, nie „flat and locked file".**

**J. Testovanie integrity dát.** Over, že surové merania sa nedajú zmeniť ani zmazať z aplikácie ani (kde je to relevantné) priamym prístupom do databázy riadeným procedúrou; over, že exportovaný report zodpovedá zdrojovým dátam; over ochranu záznamov checksumom, ak je implementovaná; over, že záznam prežije vynútené prerušenie uprostred zápisu; over uchovanie dát počas požadovanej lehoty.

**K. Testovanie zálohy a obnovy.** Spusti zálohu, over, že sa dokončí a uloží podľa špecifikácie; potom **obnov do testovacej inštancie a preukáž, že obnovené dáta sú úplné, presné a čitateľné, vrátane audit trailu.** Toto výslovne vyžaduje Annex 11 §7.2. Zaznamenaj dosiahnuté RPO/RTO.

**L. Testovanie generovania reportov.** Vygeneruj každý typ reportu (periodický, odchýlkový, stav kalibrácií, audit trail, história alarmov); over správnosť obsahu proti surovým dátam, správne časové pásmo, správne hlavičky (systém, obdobie, ID senzorov, limity), stránkovanie, značku „koniec reportu", a že sa dá vyrobiť jasný výtlačok/PDF (Annex 11 §8.1). **Pri záznamoch podporujúcich prepustenie over aj indikáciu zmeny podľa §8.2.**

**M. Testovanie synchronizácie času.** Over, že všetky komponenty (senzor/logger, gateway, network server, aplikačný server, databáza) berú čas z jedného autoritatívneho zdroja (NTP); over spracovanie časového pásma a letného času (záznam vytvorený v momente prechodu na zimný čas musí byť jednoznačný); over, že používateľ nemôže zmeniť systémové hodiny ani časovú pečiatku záznamu; over, že časová pečiatka bufferovaného merania po výpadku siete je **čas merania, nie čas nahrania.**

**N. Testovanie výpočtov a odvodených hodnôt.** Tam, kde softvér počíta MKT, priemery, min/max alebo trvanie odchýlky, over proti nezávislému ručnému výpočtu.

**O. Testovanie riadenia zmien konfigurácie a limitov.** Over, že zmena alarmového limitu vyžaduje správne oprávnenie, je zaznamenaná v audit traile a nadobudne účinnosť tak, ako je zdokumentované.

Pre sklady OQ testuje riadenie a alarmy cez: *„functional checks"* odozvy HVAC a rozhraní senzorov; **„alarm challenges" simulujúce teplotné odchýlky a merajúce časy odozvy**; *„door cycle tests"* hodnotiace infiltráciu pri rampe a zotavenie; *„stress mapping"* s rôznymi zaťaženiami na identifikáciu najhorších miest; a **„data integrity checks" pre synchronizáciu času a audit traily** ([Eupry](https://eupry.com/iq-oq-pq/warehouses/)). Pre monitorovací systém: overenie kalibrácie testovaním senzorov vo viacerých bodoch rozsahu, testovanie funkčnosti alarmov, validácia integrity dát, testovanie stability počas sieťových prerušení a kolísania napájania, a overenie bezpečnostných funkcií ([SenseAnywhere](https://www.senseanywhere.com/what-are-the-iq-oq-and-pq-phases-of-temperature-monitoring-system-qualification/)).

**Annex 15 §3.12 pripomína, že SOP a školenia sú výstupmi OQ**, nie vstupmi — úspešné OQ *„should allow the finalisation of standard operating and cleaning procedures, operator training and preventative maintenance requirements."* Preto SOP odovzdávaj až po OQ, nie pred ním.

### PQ — desať položiek a otázka trvania

PQ = systém v reálnej, rutinnej, zaťaženej prevádzke počas dlhšieho obdobia.

- **PQ-01 Predĺžená nepretržitá prevádzka** počas dohodnutého obdobia (bežne 30 dní; podľa citovanej praxe dodávateľov 4–8 týždňov) so skladom v rutinnej prevádzke — plné zaťaženie, bežná premávka cez dvere, bežné pohyby tovaru.
- **PQ-02 Metrika úplnosti dát** — percento očakávaných meraní skutočne prijatých a uložených na senzor za celé obdobie (napr. akceptačné kritérium ≥ 99,5 %). **Toto je najpresvedčivejšia PQ metrika pre bezdrôtový systém a je to číslo, ktoré by malo byť v každej ponuke.**
- **PQ-03 Spoľahlivosť komunikácie** — počet a trvanie prenosových medzier na senzor; trend RSSI/SNR; dostupnosť gatewaya.
- **PQ-04 Výkon alarmov v reálnej prevádzke** — každý reálny alarm v období preskúmaný: bol pravý, bol doručený, bol potvrdený včas, bola eskalácia správna; miera falošných alarmov.
- **PQ-05 Porovnanie proti nezávislej referencii/mapovaciemu loggeru** umiestnenému vedľa pevného senzora počas PQ, preukazujúce zhodu v tolerancii v čase (zároveň dokladuje drift).
- **PQ-06 Kontrola driftu senzora** — po-PQ overenie/kalibračná kontrola proti referencii preukazujúca drift v limitoch.
- **PQ-07 Výkon batérie** a projektovaná životnosť batérie LoRaWAN uzlov.
- **PQ-08 Stabilita servera/aplikácie** — dostupnosť, úspešnosť záloh, preskúmané chybové logy, žiadne nevysvetlené medzery.
- **PQ-09 Integrita dát za obdobie** — audit trail za obdobie preskúmaný; žiadne nevysvetlené modifikácie.
- **PQ-10 Sezónny najhorší prípad** — buď spusti PQ v najteplejšej/najchladnejšej sezóne, alebo urob druhý krátky potvrdzovací beh v opačnej sezóne, alebo to zdôvodni posúdením rizík s použitím mapovacích dát.

**Obhájiteľné komerčné predvolené nastavenie: minimálne 30 po sebe nasledujúcich dní PQ, so sezónnym potvrdením, a s PQ zámerne obsahujúcim aspoň jednu plánovanú výpadkovú udalosť napájania a jednu siete**, ak neboli pokryté už v OQ.

⚠️ **Neexistuje európsky regulačný text, ktorý by určoval trvanie PQ pre monitorovacie systémy.** Údaj 4–8 týždňov pochádza od dodávateľa (SenseAnywhere) a 7–14 dní pre mapovanie od iného (Eupry); sú to priemyselné praxe, nie zákon. Annex 15 vyžaduje len zdôvodnenie prístupu. **Tento rozdiel povedz zákazníkovi čestne** — paradoxne to zvyšuje cenu, ktorú je ochotný zaplatiť, pretože z teba robí poradcu namiesto predajcu šablóny. ⚠️ Nenašiel sa ani žiadny primárny európsky zdroj predpisujúci minimálny počet pevných monitorovacích senzorov na m³ alebo na komoru.

### Matica sledovateľnosti požiadaviek (RTM)

RTM spája každú URS požiadavku → prvok špecifikácie/návrhu → testovací prípad → výsledok testu. ⚠️ **Annex 15 frázu „traceability matrix" nepoužíva** — nehovor, že „to vyžaduje Annex 15". Požiadavka je odvodená z Annexu 11 §4.4 (požiadavky sledovateľné počas životného cyklu) a z draft 6.5, ktorý to hovorí explicitne.

Praktický formát: jeden riadok na URS požiadavku, stĺpce: **URS ID | Text požiadavky | GMP-kritická (Á/N) | Trieda rizika | Odkaz na funkčnú/návrhovú špecifikáciu | Testovací protokol a ID testu (IQ-xx / OQ-xx / PQ-xx) | Výsledok (pass/fail, dátum) | Odkaz na odchýlku, ak nejaká.**

**RTM je zároveň nástroj, ktorý robí riadenie zmien lacným:** keď release zmení funkciu, RTM okamžite ukáže, ktoré testovacie prípady treba znovu vykonať. **Oplatí sa to postaviť ako funkciu produktu, nie ako tabuľku vo Worde.** Keďže je softvér bespoke (Cat 5), RTM by mala dosledovať aj k návrhovým/kódovým modulom, nielen k testom.

### Kompletný zoznam dokumentov plateného validačného balíka

Zostavené z požiadaviek klauzúl vyššie; **12–17 dokumentov** podľa rozsahu. Každý s uvedenou klauzulovou oporou.

1. **Validačný plán** (projektovo špecifický; odkazuje na VMP zákazníka) — Annex 15 §1.4–1.5, §2.1–2.4
2. **URS** — Annex 15 §3.2; Annex 11 §4.4
3. **GxP posúdenie + posúdenie rizík** (kritickosť systému, funkčné posúdenie rizík na požiadavku) — Annex 11 §1; Annex 15 Principle
4. **Určenie a zdôvodnenie GAMP kategórie** — GAMP 5 (argument Cat 4 vs Cat 5)
5. **Funkčná / návrhová / konfiguračná špecifikácia** — Annex 15 §3.2–3.3
6. **DQ report** — Annex 15 §3.3
7. **Záznam o posúdení dodávateľa + index dokumentácie dodávateľa** — Annex 11 §3.1–3.4, §4.5–4.6
8. **RTM** — Annex 11 §4.4; draft 6.5
9. **IQ protokol a IQ report** — Annex 15 §3.8–3.9
10. **OQ protokol a OQ report** (možno spojiť s IQ ako IOQ podľa §2.5) — Annex 15 §3.10–3.11
11. **PQ protokol a PQ report** — Annex 15 §3.13–3.14
12. **Mapovací protokol a report** (samostatná štúdia; vstup pre umiestnenie senzorov a alarmové limity)
13. **Kalibračné certifikáty** (všetky senzory + referenčný etalón) — Annex 15 §3.9 iv
14. **Log odchýlok / nezhôd** pre kvalifikáciu — Annex 15 §2.x; Annex 11 §4.2
15. **Validation Summary Report** s vyhlásením o uvoľnení na použitie — Annex 15 §2.2
16. **Odovzdávacia sada SOP a záznamov o školení** (použitie systému, riešenie alarmov, prehľad audit trailu, správa používateľov, záloha/obnova, periodický prehľad, riadenie zmien) — Annex 15 §3.12
17. **Postup riadenia zmien / releasov a stratégia rekvalifikácie** — Annex 15 §1.5 vii, §4.1–4.2, §11.x; Annex 11 §10–11

**Každý dokument musí niesť:** meno zákazníka a lokalitu, názov a ID systému, ID a verziu dokumentu, podpisové bloky autor/recenzent/schvaľovateľ **vrátane schválenia QA zákazníka** (Annex 15 §2.6), a sekciu s regulačnými odkazmi.

---

## Riadenie zmien: čo dodať s každou verziou

Softvérový release na validovanej lokalite je **zmena podliehajúca systému riadenia zmien zákazníka** (Annex 11 §10, Annex 15 §11). Tvoja povinnosť je dať zákazníkovi všetko, čo potrebuje na vykonanie posúdenia rizík a na rozhodnutie o rozsahu regresie. Spúšťacia veta je **Annex 15 klauzula 11.4.**

### Deväťpoložkový release pack

1. **Release notes** — nové číslo verzie, predchádzajúce číslo verzie, dátum releasu a položkový zoznam zmien rozdelený na: nové funkcie, zmenené funkcie, opravy chýb, bezpečnostné patche a zmeny databázovej schémy.
2. **Klasifikácia zmeny / vyhlásenie o dopade na každú zmenu** — GxP-relevantná áno/nie, ktorých URS požiadaviek sa týka, ktorých validovaných funkcií sa týka. **Toto je to, čo umožní QA zákazníka urobiť posúdenie podľa Annexu 15 §11.4 bez reverzného inžinierstva kódu.**
3. **Testovacie dôkazy zo strany dodávateľa** — unit/integračný/systémový testovací report pre release, vykonaný na tvojej vlastnej testovacej inštancii, s pass/fail a otvorenými známymi defektmi.
4. **Odporúčaný rozsah regresného testovania u zákazníka** — tvoj návrh, ktoré OQ testovacie prípady znovu vykonať, **so zdôvodnením toho, čo sa znovu nevykonáva** (toto zdôvodnenie je presne ten dokument, ktorý inšpektori pýtajú).
5. **Aktualizovaná dokumentácia** — aktualizované URS/FS/návrhová špecifikácia tam, kde sa funkcie zmenili, aktualizovaný používateľský manuál, aktualizovaná RTM.
6. **Inštalačné/upgradovacie pokyny a plán návratu** — vrátane krokov migrácie databázy a, podľa Annexu 11 §4.8, dôkazu, že migrované dáta nezmenili hodnotu ani význam.
7. **Zoznam známych problémov / defektov** prenesených do releasu.
8. **Bezpečnostné vyhlásenie** — riešené CVE, aktualizácie závislostí.
9. **Vyhlásenie o validačnom stave** — tvoja deklarácia toho, čo bolo overené a čo zostáva zodpovednosťou zákazníka.

### Rozsah regresie

Rizikovo založený, podľa Annexu 15 §11.4:

- **Každý OQ testovací prípad, ktorého funkcia sa priamo zmenila** (povinné).
- **Každý OQ testovací prípad pre funkciu, ktorá zdieľa kód, dáta alebo konfiguráciu so zmenenou funkciou** (integračné riziko).
- **Jadro kritickej cesty, znovu vykonané pri každom release bez ohľadu na obsah zmeny**, pretože na tom závisí bezpečnosť pacienta: získavanie a ukladanie meraní; vyhodnocovanie alarmových limitov a vyvolanie alarmu; doručenie notifikácie; zaznamenávanie a nemennosť audit trailu; riadenie prístupu; správnosť exportu/reportu; synchronizácia času a správnosť časových pečiatok; kontinuita dát pri strate siete.
- **Po-upgradová kontrola integrity dát:** záznamy vytvorené pred upgradom sú stále úplné, čitateľné a nezmenené; história audit trailu zachovaná; report za obdobie prekrývajúce upgrade je správny.
- **Záloha/obnova overená po upgrade.**
- Výsledkom je **regresný testovací report**, na ktorý sa odkazuje záznam o riadení zmien u zákazníka; zmena sa uzatvára až po preskúmaní účinnosti.

**Praktický model pre jednotlivca: udržiavaj stálu „regresnú sadu" (povedzme 15–25 testovacích prípadov vybraných z OQ) a predávaj ročnú alebo per-release službu „rekvalifikácia pri upgrade", kde sa tie prípady znovu vykonajú a vyreportujú.** To mení riadenie zmien na opakovanú tržbu a je to presne to, čo zákazníci potrebujú.

**Zásadné obmedzenie architektúry, ktoré musíš prijať:** zákazníci nedokážu udržať validovaný stav (draft 2.1), ak im tlačíš zmeny bez ohlásenia. **Continuous deployment priamo do produkcie je nezlučiteľné s GMP** a release proces musíš navrhnúť podľa toho. Draft 7.5 bod ix to robí zmluvnou povinnosťou.

⚠️ Nenašiel sa európsky regulačný text, ktorý by definoval „regresné testovanie" alebo predpisoval jeho rozsah; rozsah vyššie je odvodený z povinnosti posúdenia rizík v Annexe 15 §11.4 a z praxe GAMP. ⚠️ Klauzula Annex 15 11.6 (post-implementačné hodnotenie účinnosti zmeny) sa nevrátila — sekcia 11 môže pokračovať za 11.5; over.

### Periodický prehľad — trinásť bodov

Platia dve samostatné povinnosti: **Annex 11 §11 periodické hodnotenie počítačového systému** a **Annex 15 §4.1–4.2 rekvalifikácia zariadení/systémov v zdôvodnenej frekvencii.** ⚠️ **Ani jeden neuvádza číslo** — interval musí zdôvodniť zákazník; priemyselná prax pre kritické GxP systémy sa bežne pohybuje na 1–3 rokoch, ale **primárny zdroj s číslom sa nenašiel. Zákazníkom to prezentuj ako „interval musí byť zdôvodnený", nie ako „každých N rokov je povinné".**

Report periodického prehľadu má pokryť: (1) popis systému a aktuálnu konfiguráciu proti validovanej baseline (hardvér, firmvér, verzie softvéru, zoznam a polohy senzorov); (2) **validačný stav** — ktoré IQ/OQ/PQ dokumenty sú aktuálne, aké sú medzery; (3) **históriu zmien** od posledného prehľadu — každá zmena/upgrade, jej referencia v riadení zmien a či bolo regresné testovanie primerané; (4) **odchýlky, incidenty a alarmy** — zoznam, príčiny, CAPA, otvorené položky; štatistika alarmov a miera falošných alarmov; (5) **prehľad audit trailu** — dôkaz, že periodický prehľad prebehol, a jeho zistenia; (6) **prehľad používateľských prístupov** — aktuálny zoznam používateľov proti zoznamu oprávnení; odídení zamestnanci odstránení; plazivé narastanie oprávnení; zdieľané účty; (7) **stav kalibrácií** — každý senzor v platnosti; trend driftu; nálezy mimo tolerancie a ich retrospektívne posúdenie dopadu na dáta; (8) **záloha a obnova** — záznam úspešnosti záloh za obdobie a dôkaz o vykonanom teste obnovy; (9) **bezpečnosť** — úroveň patchov, zraniteľnosti, bezpečnostné incidenty, stav penetračného testu (v anticipácii draft §15); (10) **výkon/dostupnosť** — uptime, percento úplnosti dát, komunikačné medzery; (11) **SOP a školenia** — sú procedúry aktuálne, sú používatelia zaškolení na aktuálnu verziu; (12) **stav dodávateľa** — dodávateľ stále podniká a poskytuje podporu, zmluva/dohoda o kvalite je aktuálna, audit dodávateľa je splatný, verzia je stále podporovaná (nie EOL); (13) **záver** — systém zostáva vo validovanom stave / vyžaduje opatrenia; či je spustená rekvalifikácia (čiastočná alebo plná); opatrenia s vlastníkmi a termínmi.

**Spúšťače rekvalifikácie, ktoré napíš do SOP zákazníka:** major upgrade softvéru; migrácia servera alebo zmena hostingu; zmena typu senzora alebo firmvéru; premiestnenie alebo zmena monitorovaného priestoru; zmeny dispozície skladu/HVAC ovplyvňujúce teplé a studené body; opakované nevysvetlené odchýlky alebo dátové medzery; kalibrácia mimo tolerancie; zistenia z periodického prehľadu; uplynutie zdôvodneného rekvalifikačného intervalu.

---
## Dodávateľský dotazník: čo sa ťa budú pýtať a dve otázky, ktoré sú pre jednotlivca najťažšie

Annex 11 §3.2 robí z kompetentnosti a spoľahlivosti dodávateľa základ výberu, s potrebou auditu určenou rizikom — takže malý dodávateľ v kategórii GAMP 5 dostane **dodávateľský dotazník kvality rutinne**, a pri bespoke systéme môže dostať aj **plný dodávateľský audit** (vzdialený alebo na mieste). Priprav si trvalý **dossier kvalifikácie dodávateľa** a každý audit sa zmení z dní na hodiny.

### Deväť povinností, ktoré z klauzúl plynú priamo

1. **Písomná zmluva o kvalite / technická dohoda s každým zákazníkom** (2011 kl. 3.1 *„must exist"*; draft 7.5). Musí explicitne uvádzať zodpovednosti každej strany. **Najvyššia priorita zo všetkých dokumentov.** Navrhni štandardnú dvojjazyčnú (SK/EN) zmluvu pokrývajúcu všetkých deväť prvkov draft 7.5 už teraz.
2. **Zdokumentovaný QMS pokrývajúci vývoj softvéru** (2011 kl. 4.5/4.6; draft 3.1). Aj odľahčený: SOP pre správu požiadaviek, návrh, kódovacie štandardy, code review, testovanie, release/riadenie zmien, incidenty/CAPA, zálohy a bezpečnosť. ISO 9001 alebo ISO 27001 **nie je Annexom 11 vyžadovaná**, ale robí posúdenie podľa 3.2/4.5 triviálnym a je to silný predajný argument proti väčším konkurentom (LogHub obe deklaruje).
3. **Akceptovanie dodávateľských auditov** (2011 kl. 3.2; draft 7.2, 7.5 iv). Musíš byť ochotný a zmluvne zaviazaný hostiť audit zákazníka — vzdialene alebo na mieste.
4. **Dokumentačný balík na preskúmanie zákazníkom** (2011 kl. 3.3; draft 6.x, 9.x). Keďže je telemetry.digital bespoke a nie COTS, má obsahovať: popis systému, funkčnú špecifikáciu, šablónu URS, RTM (draft 6.5), IQ/OQ protokoly a šablóny, testovacie dôkazy vrátane snímok obrazovky (draft 9.4), release notes, administrátorský a používateľský manuál, a **vyhlásenie o integrite dát / súlade s Annexom 11 mapujúce každú klauzulu na to, ako ju produkt spĺňa.** **Tá klauzulová matica zhody je najpredajnejší jediný dokument, aký vieš vyrobiť.**
5. **Systém kvality a informácie o auditoch dostupné inšpektorom na požiadanie** (2011 kl. 3.4). Podpora počas regulačných inšpekcií je zmluvným prvkom v návrhu (7.5 v).
6. **SLA s KPI** (draft 7.3). Dostupnosť, latencia doručenia alarmu, časy reakcie a notifikácie pri incidente, časy reakcie podpory, časy nasadenia patchov. Definuj ich teraz — je to aj spôsob, ako sa odlíšiš.
7. **Exit strategy** (draft 7.5 viii). Viď nižšie.
8. **Proces vydávania verzií** (draft 7.5 ix). Zdokumentovaná, so zákazníkom odsúhlasená procedúra: predbežné oznámenie, release notes, dôkazy o regresnom testovaní, posúdenie dopadu a schválenie zákazníkom pred nasadením do produkcie.
9. **Povinnosti oznamovania incidentov** (2011 kl. 13; draft 7.5 iii a vii). Definované kanály a lehoty na hlásenie incidentov **vrátane bezpečnostných** (draft 7.5 vii) zákazníkovi.

### Typický obsah dotazníka

⚠️ **Poctivosť:** skutočnú sadu otázok sa nepodarilo získať. Stránka ECA Academy *„Appendix 4: Supplier Quality Questionnaire — Extended Quality Questionnaire"* bola blokovaná robots.txt a obsah dotazníka **Rx-360 Supplier Assessment Questionnaire** sa nenačítal. Skupiny nižšie sú **odvodené z požiadaviek klauzúl a zo zoznamu dodávateľskej dokumentácie**, nie skopírované z publikovaného dotazníka. Ak chceš reálny príklad, sú to dve konkrétne stopy: [Rx-360](https://rx-360.org/supplier-assessment-questionnaire/) a [ECA Academy Appendix 4](https://www.gmp-compliance.org/guidelines/gmp-guideline/appendix-4-supplier-quality-questionnaire-instructions-for-completion-e-extended-quality-questionnaire).

- **Firma a kontinuita:** právna forma, roky pôsobenia, počet zamestnancov, **závislosť od kľúčovej osoby**, finančná stabilita, poistenie, escrow zdrojového kódu, čo sa stane, ak dodávateľ prestane podnikať.
- **Systém kvality:** existuje zdokumentovaný QMS; certifikácia ISO 9001 / 27001 / 13485; príručka kvality; zoznam SOP; kto je zodpovedný za kvalitu; interné audity; management review.
- **Regulačné povedomie:** znalosť EU GMP Annex 11 a 15, GAMP 5, 21 CFR Part 11, data integrity/ALCOA+; predchádzajúce regulačné inšpekcie na lokalitách zákazníkov, ktoré sa dotkli produktu.
- **Vývojový životný cyklus:** zdokumentovaný SDLC; správa požiadaviek; návrhová dokumentácia; kódovacie štandardy; code review; systém správy verzií; vetvenie/release stratégia; reprodukovateľnosť buildu; použitie komponentov tretích strán/open source a ako sa sledujú.
- **Testovanie:** testovacia stratégia; dôkazy unit/integračných/systémových testov; testovacie prostredia; kto testuje; systém správy defektov; ako sa známe defekty komunikujú zákazníkom; regresná sada; automatizované testovanie.
- **Release a riadenie zmien:** schéma verziovania; schvaľovanie releasu; release notes; upgrade/rollback; ako sa notifikujú zákazníci; podpora starších verzií; EOL politika.
- **Integrita dát a technické kontroly:** návrh audit trailu; je raw data nemeniteľné; zdroj času; model používateľov a rolí; politika hesiel; šifrovanie v pokoji aj pri prenose; riadenie prístupu do databázy; záloha a obnova; retencia a archivácia dát; export dát pri ukončení zmluvy.
- **Hosting/cloud (ak SaaS):** lokalita dátového centra (v EÚ?), subprocesory, GDPR/DPA, SLA dostupnosti, disaster recovery, RPO/RTO, penetračné testovanie, správa zraniteľností, reakcia na incidenty.
- **Personál:** CV a záznamy o školeniach vývojárov a validačného personálu; školenie na GMP a integritu dát; popisy pracovných pozícií.
- **Subdodávatelia:** kto ďalší sa dotýka kódu alebo dát; ako sú kvalifikovaní.
- **Poskytovanie dokumentácie:** akú validačnú dokumentáciu dodávaš; môže zákazník preskúmať tvoju internú vývojovú dokumentáciu; povoľuješ audit na mieste alebo vzdialene; akceptuješ audítorské práva v zmluve.
- **Podpora:** SLA, časy reakcie, kanály, eskalácia, **kontroly vzdialeného prístupu dodávateľa** (ako je vzdialený prístup autorizovaný, logovaný a obmedzený — častý inšpekčný nález).

### Najťažšia otázka č. 1: oddelenie povinností (draft 11.10)

Draft 11.10 požaduje, aby používatelia *„do not have administrative privileges"* a aby platil princíp najmenších oprávnení. WHO Appendix 1 žiada obmedziť rozšírené oprávnenia *„only to persons independent of those responsible for the content of electronic records"*. Draft 12.6 žiada, aby prehľady audit trailu robil personál *„not directly involved in the activities reviewed"*.

**Pre jednoosobovú firmu je to fyzicky nesplniteľné, a ⚠️ nenašlo sa žiadne európske usmernenie, ktoré by riešilo, ako má mikropodnik alebo samostatne zárobkovo činná osoba splniť očakávania oddelenia povinností. Je to skutočná medzera v usmerneniach.**

**Riešenie, ktoré je obhájiteľné a ktoré musíš napísať, nie zamlčať:**

- **Právny základ je draft klauzula 2.3:** *„Practices which constitute alternatives to the activities required in this document may be used, if they have been proven and documented to provide the same or higher level of control."* Toto je tvoj oporný bod a treba ho v odpovedi na dotazník citovať doslova.
- **Kompenzačné opatrenie musí byť technické, nie organizačné.** Konkrétne: databázová rola bez `UPDATE`/`DELETE` na tabuľkách meraní a auditu; hash-chain riadkov audit trailu, aby bola manipulácia detekovateľná; žiadny konfiguračný prepínač na vypnutie audit trailu; oddelené účty pre administratívne a prevádzkové operácie aj u teba samého, s auditom oboch.
- **Priznanie v dokumentácii:** napíš výslovne, že v prostredí jednoosobového dodávateľa je administrátor zároveň používateľom, a vymenuj konkrétne technické kontroly, ktoré to kompenzujú, plus dôkazy o periodickom prehľade.
- **Kde je to možné, prenes nezávislosť na zákazníka.** Rolu nezávislého recenzenta audit trailu (draft 12.6) plní **QA zákazníka, nie ty.** Tvoj softvér ju len musí podporovať — read-only rola s prístupom k audit trailom bez možnosti meniť podkladové dáta a záznam o tom, že prehľad prebehol, kým, kedy a s akým výsledkom.
- **Externý recenzent za peniaze.** Zdokumentovaná periodická externá revízia kódu (napr. raz ročne, nezávislým vývojárom so zmluvou o mlčanlivosti) je lacná a v dotazníku vyzerá veľmi dobre. Rovnako externý penetračný test (draft 15.19).

### Najťažšia otázka č. 2: exit strategy a bus factor jedna (draft 7.5 viii)

*„Defines an exit strategy by which the regulated user may retain control [of data]"* ⚠️. **Pre jednotlivca je to existenčná zákaznícka námietka** — zákazník sa reálne pýta: čo sa stane s ôsmimi rokmi mojich GMP dát, ak ťa zrazí auto?

Otázka nie je nespravodlivá a **odpovedať na ňu proaktívne je najúčinnejší spôsob, ako neutralizovať hlavnú komerčnú námietku voči nákupu od jednoosobovej firmy.** Opatrenia, ktoré zároveň spĺňajú 7.5(viii):

- **Zdokumentovaný plný export dát v otvorenom, prehľadávateľnom formáte** — krížovo s draft 12.9 („Flat and locked files are not acceptable") a draft 17.5 (archivované dáta musia zostať prehľadávateľné a triediteľné). Prakticky: CSV/JSON export meraní, audit trailu, alarmového logu, konfigurácie a kalibračných záznamov, plus PostgreSQL dump. Je to lacné a rozhoduje to.
- **Escrow zdrojového kódu.** Ponuka escrow je pre jednotlivca neúmerne silný signál dôvery a stojí málo.
- **Zdokumentovaný plán odovzdania a kontinuity** — kto prevezme prevádzku, aké sú prístupové údaje, kde je dokumentácia.
- **Zmluvné právo zákazníka na plný dump databázy na požiadanie**, kedykoľvek, nielen pri ukončení.
- **Fakt, že produkt je jediný binár na Linux aj Windows Server, s PostgreSQL a bez cloudu, je tu obrovská výhoda** — zákazník, ktorý to prevádzkuje on-premise, nie je na tebe závislý na úrovni infraštruktúry vôbec. **Toto predávaj explicitne proti SaaS konkurencii.** Je to najsilnejšia odpoveď na exit strategy, akú v tomto segmente vieš dať, a väčšina konkurentov ju dať nevie.

### Ako formulovať tvrdenia o zhode — a ako nie

Dodávatelia dôsledne používajú opatrný, podporný jazyk. ELPRO ⚠️ (citácie boli fetchom skrátené a treba ich pred reprodukciou overiť):

> "ELPRO supports EU GMP Annex 11 and 21 CFR Part 11 requirements through a combination of secure system design, controlled access, traceable records, and documented validation support."

> "Audit trails support data integrity by showing who did what, when, and what changed."

> "ELPRO provides structured documentation packages to support implementation,"

— [ELPRO, Documentation and Computerized Systems Regulations](https://www.elpro.com/en/learn/documentation-computerized-regulations)

Na produktovej stránke však ten istý dodávateľ tvrdí podstatne silnejšie: *„Fully compliant with FDA 21 CFR Part 11 and GAMP® 5 standards"* ([ELPRO pharmacy monitoring](https://www.elpro.com/en/pharmacy-temperature-monitoring)).

**Ten rozdiel je poučný: obhájiteľná formulácia žije v technickej dokumentácii, agresívna v predajnom texte.** Malý dodávateľ by mal používať **obhájiteľný register všade**, pretože jednotlivec nemá právne oddelenie, ktoré by prehnané tvrdenie absorbovalo, a pretože inšpektor ŠÚKL, ktorý spochybní odvážne tvrdenie, nenávratne poškodí vzťah so zákazníkom.

**Použiteľná šablóna vyhlásenia o zhode:**

> *„[Produkt] podporuje požiadavky [EU GMP Annex 11 / vyhlášky 129/2012 § 30 ods. 8] prostredníctvom [menovaných technických funkcií], a [dodávateľ] dodáva [kalibračné certifikáty / dokumentáciu IQ-OQ / odpovede na dodávateľský dotazník] na podporu vlastnej validácie zákazníka. Zodpovednosť za validáciu systému v zamýšľanom použití zostáva na zákazníkovi."*

Vaisala svoje tvrdenia substancuje inak a lepšie: **publikuje skutočný 161-stranový vzorový IQ/OQ protokol pre viewLinc** namiesto toho, aby zhodu tvrdila v próze ([Vaisala viewLinc 5.0 IQ/OQ SAMPLE PDF](https://www.vaisala.com/sites/default/files/documents/VIM-G-Vaisala-viewLinc-5_0-IQOQ-SAMPLE.pdf)). **To je model, ktorý stojí za nasledovanie v malom** — publikovaná ukážka protokolu je lepší predajný materiál než brožúra.

⚠️ Nepodarilo sa získať plné doslovné vendorské *vyhlásenie o zhode* ani vyplnený dodávateľský dotazník — len parafrázované a skrátené znenie od ELPRO. Nezískalo sa žiadne znenie od Vaisala, Testo, Comet ani LogHub. **A nenašiel sa žiadny príklad, ako ktorýkoľvek dodávateľ formuluje zhodu so slovenskými vyhláškami — čo je konzistentné so záverom, že to nerobí nikto.**

---

## Komerčná časť: čo sa dá predať, za koľko, a proti komu

### Čo sa dá predávať

| Položka | Regulačná opora | Typ tržby |
|---|---|---|
| Kalibrované senzory s protokolom z akreditovaného laboratória | vyhl. 129/2012 § 30 ods. 8 („kalibrované meradlo"); vyhl. 128/2012 § 13; GDP 3.3 („traceable to a primary standard") | jednorazová |
| Ročná rekalibrácia / výmena senzora | **nie je zákonná povinnosť** — interval si určuje prevádzkovateľ; predávaj ako službu správy životného cyklu certifikátov | **opakovaná** |
| Mapovacia štúdia | GDP 3.2.1 („initial temperature mapping … before use"); WHO TRS 961 Annex 9 Supp 8 | projektová, opakovaná každé ~3 roky |
| Inštalácia a uvedenie do prevádzky | Annex 15 §3.8–3.9 (IQ) | jednorazová |
| Validačný balík IQ/OQ/PQ | **Annex 15 clause 2.6** — právny základ pre protokoly tretej strany | projektová |
| Kombinované IOQ pre malé inštalácie | Annex 15 clause 2.5 | projektová, nižší cenový bod |
| Rekvalifikácia pri upgrade (regresná sada) | Annex 15 §11.4; Annex 11 §10 | **opakovaná** |
| Ročný dátový balík pre periodický prehľad | Annex 11 §11; draft kap. 14 | **opakovaná** |
| Mesačný slovenský report so sign-off a poľom pre odchýlku | vakcínové usmernenie (meno + nápravné opatrenie); GDP 4.2 | **opakovaná** |
| Vyhlásenie o zhode s vyhláškou 129/2012 po bodoch | — (nikto to nepublikuje) | diferenciátor zadarmo |
| Odpovede na dodávateľské dotazníky | Annex 11 §3.2–3.4 | dôveryhodnostný signál, zadarmo |

### Cenové kotvy, ktoré skutočne existujú

**Hardvér, slovenská maloobchodná cena vrátane kalibračného listu** — všetko z [Metershop, kategória pre lekárne](https://metershop.sk/kategoria/teplomery/s-certifikatom-o-kalibracii/lekarne-zdravotnictvo/s-automatickym-zaznamom-pre-lekarne/) (ceny **s DPH**):

- Kalibrovaný datalogger **Elitech RC-5+** so sondou — **€108,00**
- Kalibrovaný datalogger **RC-5** — **€79,95**
- Kalibrovaný datalogger **TFA 30.3039.IT KLIMALOGG PRO** — **€147,60**
- Snímač **TFA 30.3180.IT** — **€73,80**; **TFA 30.3181.IT** so sondou — **€66,42**
- Snímač **TFA 30.3312** (WeatherHub Observer) — **€105,78**; **TFA 30.3313** do chladničky — **€84,87**
- **TFA 31.4012 WEATHERHUB OBSERVER** štartovacia sada — **€153,75**
- **TFA 30.3900.02** — **€71,34**; **TFA 30.3902.02** sonda — **€67,65**
- **TFA 35.8103.02** WiFi gateway s integrovaným teplomerom — **€98,40**
- Kalibrovaný datalogger **DS102** — **€92,40**

Marketingové tvrdenie tej istej stránky: zariadenia sú *„špeciálne navrhnuté pre potreby lekární a ďalších zdravotníckych zariadení v súlade s platnou legislatívou"*.

**Comet, publikované ceny** ([meraciepristroje.sk](https://www.meraciepristroje.sk/znacky/comet~m565), s uplatnenou 5 % zľavou):

- Datalogger **S0110E** (vnútorné čidlo, −30 až +70 °C) — **€94,24 bez DPH / €114,03 s DPH**
- Teplomer-vlhkomer **S3120E** (economy, −30 až +70 °C + RH) — **€115,52 bez DPH / €139,78 s DPH**; popísané ako *„ekonomická verzia - nižšia cena, horšia presnosť"*
- **Multilogger M1140** (4 univerzálne vstupy, až 1 000 000 záznamov) — **€702,62 bez DPH / €850,17 s DPH**

**Kalibrácia, publikované slovenské ceny** ([Metershop, Kalibrácia teplomerov](https://metershop.sk/sluzby/kalibracia-teplomerov/), akreditované ISO 17025 laboratórium):

- Akreditovaná kalibrácia **€19,68–€72,57 za prístroj** podľa typu a počtu meracích bodov
- Príklad potravinárstvo: základný chladničkový teplomer, **dva teplotné body — €19,68 (€16 bez DPH)**
- Príklad zdravotníctvo: bezkontaktný teplomer pre lekáreň, **tri body — €72,57 (€59 bez DPH)**
- Neakreditovaná kalibrácia od **€14,76** (základný interiérový/exteriérový teplomer, štyri body)
- **Za každý ďalší kalibračný bod: €4,92 akreditovane / €3,08 neakreditovane**
- Doba vybavenia **5–10 pracovných dní**; vyzdvihnutie a vrátenie zadarmo nad €75, inak €4,90 manipulačný poplatok

**Predplatné / systémy:** **Eupry je jediný nájdený GxP dodávateľ, ktorý publikuje číslo** — CLOUD **„From €200/yearly"**, zahŕňa cloudový softvér, WiFi datalogger s teplotným a vlhkostným senzorom, **pay-per-use akreditovanú ISO 17025 kalibráciu**, community SLA a online onboarding. Tier **GxP COMPLIANCE**: cena **nepublikovaná**; pridáva **patentovanú automatizovanú kalibráciu** a **IOQ protocols service**. Tier **ENTERPRISE**: cena na požiadanie, pre inštalácie **presahujúce 500 meracích bodov**; pridáva enterprise softvér s API integráciou, **ročnú ISO 17025 kalibráciu**, business case a **quality questionnaire** ([Eupry pricing](https://eupry.com/temperature-monitoring/monitoring-price/)).

**Mapovanie: cena nie je publikovaná nikde v EÚ.** Eupry na priamu otázku odpovedá len: *„The services come with flexible pricing tailored to your specific requirements."* ([Eupry mapping pricing](https://eupry.com/temperature-monitoring/mapping-price/)). Ponúka mapovanie tromi spôsobmi: krátkodobý prenájom vybavenia (3–4 mesiace) pre jednorazové interné mapovanie, dlhodobý prenájom (1–2 roky), a plnú službu na mieste s *„Accredited/traceable start calibration of mapping sensors"*, prístupom k validačnému softvéru, vzdialenou podporou špecialistu a voliteľným vývojom protokolu a vlastným reportingom.

### Čo z tých čísel vyplýva

**Hardvérová podlaha je zhruba €70–€150 na kalibrované meracie miesto na Slovensku a akreditovaná rekalibrácia zhruba €20–€75 na prístroj na cyklus.** Sú to jediné tvrdé kotvy, ktoré existujú, a sú nízke. **Ohraničujú, čo lekáreň akceptuje: dvojbodová lekáreň sa dokáže stať doslovne súladnou za výrazne menej než €300 hardvéru**, takže akékoľvek predplatné výrazne nad tým musí byť ospravedlnené službou, nie nevyhnutnosťou súladu. Nesnaž sa predať lekárni compliance-predplatné za €600/rok — ona si vie prečítať vyhlášku a vie, že jej stačí logger.

**Pozor na porovnateľnosť jednotiek:** Metershop uvádza ceny **s DPH**, meraciepristroje.sk uvádza **obe** s uplatnenou 5 % zľavou. Každá konkurenčná cenová tabuľka musí byť normalizovaná na ceny **bez DPH**. (Pre teba, ako neplatiteľa DPH, je to zároveň konkurenčná výhoda voči platiteľom pri predaji lekárňam, ktoré DPH neodpočítavajú — ale iba do výšky obratového limitu, na čo si daj pozor pri plánovaní rastu.)

**Eupryho „€200/rok" je najbližšie k publikovanému medzinárodnému benchmarku a je to *lacnejšie*, než by väčšina čakala** — slovenský dodávateľ teda nemôže predpokladať, že inkumbenti sú na predplatnom drahí. Drahé sú GxP/validačné tiery, a práve tie nemajú publikovanú cenu.

**Nepriehľadnosť cien je priemyselná norma a je to sama o sebe využiteľná slabina.** Publikovanie transparentného cenníka pre balík veľkosti lekárne by bolo skutočným diferenciátorom na trhu, kde každý konkurent vyžaduje obchodný rozhovor — a znižuje to bariéru dôvery voči jednotlivcovi bez značky.

**A to, že Eupryho Enterprise tier začína nad 500 meracích bodov, potvrdzuje, že globálni dodávatelia sú architektovaní pre zákazníka o dva až tri rády väčšieho než nezávislá slovenská lekáreň.**

**Mapovanie má neobyčajne priaznivú nákladovú štruktúru pre jednotlivca.** Trvanie 48–72 h a mriežka 5–10 m znamenajú, že stredne veľký slovenský distribučný sklad reálne potrebuje rádovo desiatky loggerov na pár dní, dvakrát ročne. Pri lokálnej cene kalibrovaných loggerov €70–€150 je **vstupná investícia do mapovacej služby pravdepodobne v nízkych tisícoch eur**, zatiaľ čo fakturovateľným výstupom je protokol a report. **Neexistencia publikovaných cien mapovania znamená, že zákazníci nemajú s čím porovnávať — čo chráni maržu, ale robí kupujúcich opatrnými. Publikovaná fixná cena mapovania pre malé slovenské lokality by bola skutočne nová.**

### Konkurencia: pozicionuj sa proti LogHubu, nie proti Vaisale

Trh sa delí na tri vrstvy. **Globálni GxP špecialisti** s konzultačne ťažkými balíkmi: ELPRO ponúka ECOLOG-PRO xG (celulárny IoT logger, LTE-M/NB-IoT) a ECOLOG-PRO Wireless s elproCLOUD, plus služby **GxP consulting & qualification, temperature mapping, on-/off-site calibration, project management a „Monitoring as a Service"** ([ELPRO](https://www.elpro.com/en/pharmacy-temperature-monitoring); [mapping services](https://www.elpro.com/en/gxp-services/temperature-mapping)); Vaisala publikuje vzorový IQ/OQ protokol; Testo predáva samostatnú **slovenskú** službu *„Kvalifikácia/validácia"* pre Saveris ([Testo SK](https://www.testo.com/sk-SK/services/saveris-kvalifikacia)); Eupry predáva tiery vrátane IOQ. **Regionálni výrobcovia prístrojov**: Comet System (CZ) so slovenskými stránkami ([Comet SK](https://sk.cometsystem.com/)), TFA/Elitech cez predajcov. **Lokálni integrátori**: LogHub, Metershop, KALIST AKL, Meracie prístroje.sk, Meratex, vahy-vahy.sk.

**LogHub je konkurent, ktorého treba poraziť.** Je slovenský výrobca bezdrôtových senzorov + centrálnych jednotiek + cloudového softvéru, −80 °C až +120 °C, cieli na lekárne, výrobu a distribúciu liekov, nemocnice, potravinárstvo/HACCP, chladiarne a prepravu. Deklaruje **ISO 9001 a ISO 27001**, šifrovanie dát, **automatickú rekalibráciu výmenou senzora**, odkazy na požiadavky **ŠÚKL**, a uvádza **400+ slovenských prevádzok** vrátane Nemocnice Poprad a lekárenských sietí. Výslovne marketuje *„Nová legislatíva 2026 – elektronické meranie teplôt"* ([LogHub.sk](https://www.loghub.sk/)). Česká sesterská firma predáva [teplotní mapování](https://loghub.cz/teplotni-mapovani/).

**Akákoľvek pozicionovacia úvaha, ktorá LogHub ignoruje, bude nesprávna.** LogHub už vlastní pozíciu „slovenský + ŠÚKL + legislatíva 2026" a má referencie. Diferenciácia musí byť voči nemu konkrétne:

- **Cena a veľkosť zákazníka.** LogHub má referencie nemocníc a sietí; ty cieľ na **nezávislú lekáreň a malý sklad**, ktorý je enterprise ponukami prekročený a holým e-shopovým hardvérom nedostatočne obslúžený.
- **On-premise, bez cloudu, jediný binár.** LogHub je cloudový. Pre zákazníka, ktorý nechce dáta v cudzom cloude, alebo pre ktorého je exit strategy problém, je tvoja architektúra objektívne lepšia odpoveď. **Toto je tvoj najsilnejší technický rozdiel a v dotazníkoch ho vieš dokázať.**
- **Vlastný hardvér s Pt1000 a self-identifying sondou.** Guard-band argument ISPE (±2 °C nevyhovuje, ±0,3 °C vyhovuje pre 2–8 °C) je technicky presne tá diskusia, v ktorej Pt1000 vyhráva nad generickým senzorom. Kalibračné koeficienty v DS28E07 znamenajú, že **výmena sondy neruší kalibráciu systému** — to je priamy konkurenčný náprotivok LogHubovho „rekalibrácia výmenou senzora", ale čistejší.
- **Klauzulová matica zhody so slovenskými vyhláškami.** Nikto ju nepublikuje. Ty áno.
- **Publikovaný cenník.** Nikto ho nemá.
- **Validačný balík podľa Annexu 15 clause 2.6.** LogHub podľa verejne dostupného nepublikuje IQ/OQ deliverables.

Dve poznámky na doplnenie. **LogHubova „automatická rekalibrácia výmenou senzora" je najčistejšia prevádzková odpoveď na problém opakovanej kalibrácie a pravdepodobne sa stane normou kategórie** — konkurenčná ponuka by to mala vedieť dorovnať, inak je odstávka prístroja počas kalibrácie reálnym trením. A **ELPRO má v pozicionovaní medzeru, ktorá je tvojou príležitosťou:** jeho lekárenská stránka stavia do popredia **21 CFR Part 11 a GAMP 5** (americké rámce) a v získanom obsahu **nespomína Annex 11 ani GDP**. Pre slovenského kupujúceho kontrolovaného ŠÚKL-om proti slovenským vyhláškam sú americky rámcované tvrdenia takmer irelevantné.

⚠️ **Nezískané informácie o konkurencii:** nič sa nezistilo o **Ellab, Sensitech, Berlinger, Hanwell, Onset InTemp, Tempmate, Kaye, XiltriX, Sensorix** ani o monitorovacej línii Rotronicu (okrem jediného mapovacieho PDF). Komerčné balíčkovanie Vaisaly a Testa (čo IQ/OQ stojí, čo je v cene) sa nezistilo — slovenská stránka Testo vrátila HTTP 429. **Ceny, zmluvné podmienky ani presné compliance deliverables LogHubu nie sú zverejnené.** Podiely na trhu, inštalovaná báza ani tržby žiadneho dodávateľa na slovenskom trhu sa nenašli. ⚠️ Nezistilo sa ani, či hardvér uvedený u Metershopu skutočne spĺňa novú požiadavku ≤ 2 h automatického záznamu **pre každý konkrétny model** — kategória je tak nazvaná, ale stránka v získanom obsahu neodkazuje na vyhlášku 129/2012, na dvojhodinový interval ani na termín 1. 7. 2026.

### Trhová príležitosť, realisticky

**Pre:** existuje tvrdý, datovaný zákonný spúšťač; regulátor si sám zapisuje presne tieto zlyhania (*„teplomery nie sú kalibrované"*, *„teplota a vlhkosť sa neevidujú denne"*, *„nie je teplomer v chladničke"*, a na distribučnej úrovni *„chýbajúce kalibrácie kľúčových meradiel"*); inšpekčná aktivita je reálna (478 + 73 v roku 2024); inkumbenti sú škálovo aj rámcovo nezosúladení; ceny nikto nepublikuje; akreditáciu netreba vlastniť; vstupné náklady sú nízke; **a slovenské národné usmernenie k monitoringu/mapovaniu neexistuje**, na rozdiel od českého DIS-15 v4.

**Proti:** **termín 1. 7. 2026 už uplynul** a prvá vlna je pravdepodobne za nami; LogHub pozíciu už obsadil a má referencie; hardvér je komodita dostupná komukoľvek cez distribúciu; súladná hardvérová podlaha je pod €300 na lekáreň, čo stropuje silu predplatného; a akreditáciu, ktorú nemáš, nesmieš tvrdiť.

**Životaschopná stratégia teda nie je hardvérová ani platformová hra.** Je to **služba a dokumentácia okolo subdodanej akreditovanej kalibrácie, mierená na nezávislé lekárne (nie siete) a na malé až stredné distribučné sklady, kde je pravidlo vágne a kupujúci sofistikovaný.** Obhájiteľné medzery v zostupnom poradí sily: (1) dokumentácia zhody so slovenskými vyhláškami; (2) poriadne obslúžený zákazník s 1–5 senzormi; (3) publikovaná fixná cena; (4) kalibračný životný cyklus ako služba, nie hardvér; (5) inšpekčne pripravené artefakty (mesačný slovenský report so sign-off a poľom na nápravné opatrenie, ktorý zodpovedá vakcínovému usmerneniu a ktorý generický export z loggera neposkytuje); (6) mapovanie ako druhý akt.

---

## Kontrolný zoznam: čo si musíš overiť sám

Zoradené podľa toho, koľko to môže pokaziť, keď to neurobíš.

**Priorita 1 — bez tohto nepredávaj:**

1. **Vyhláška 129/2012 § 30 ods. 8 a § 31 ods. 3 v konsolidovanom znení po novele 419/2025 — v Slov-Lexe.** Celý komerčný príbeh stojí na tejto vete a máš ju zo zakonypreludi.sk, nie zo Zbierky.
2. **Či existuje prechodné ustanovenie k termínu 1. 7. 2026** pre existujúce lekárne. Po termíne to mení obchodnú argumentáciu.
3. **Zákon 362/2011 § 18 — doslovné znenie na úrovni písmen**, vrátane toho, ktoré písmeno nesie povinnosť uchovávania vrátane teploty a či sa 5-ročná lehota vzťahuje na teplotné záznamy.
4. **Či bola vyhláška 128/2012 niekedy novelizovaná.** Absencia noviel na zakonypreludi je indícia, nie dôkaz.
5. **PIC/S PI 041-1 §9.6 a §9.7** — otvor PDF a prepíš bunky tabuliek Item/Expectation. Sú to najdôležitejšie sekcie pre tvoj prípad a sú to presne tie, kde nástroj fabrikoval.

**Priorita 2 — pred vložením do validačného dokumentu:**

6. **Annex 11 (2011) klauzuly 1 (2. veta), 2 (2. veta), 3.2 (zvyšok), 4.1–4.4, 4.6, 4.7, 10 a Glossary** — stiahni PDF a doplň.
7. **Annex 15 klauzuly 3.2 a 4.2 (pokračovania) a či sekcia 11 pokračuje za 11.5.**
8. **Draft Annex 11 kapitoly 7, 8, 11, 12, 14 a 15** — všetky boli skrátené na 125 znakov, vrátane presného znenia deviatich zmluvných prvkov 7.5 (najmä koncoviek bodov vii a viii) a dvanástich bodov 14.2.
9. **GDP 3.2.1 a 3.3 ako celé neprerušené odseky** z OJ PDF, najmä dve vety o umiestnení monitorov podľa výsledkov mapovania a o opakovaní mapovania.
10. **Smernica 2001/83/ES články 80 písm. e)/f), 84 a 85b ods. 3** — doslovne z EUR-Lexu.
11. **Či 2013/C 343/01 skutočne krížovo odkazuje na Annex 11.** Netvrdiť to, kým to nie je overené.
12. **21 CFR Part 11 §11.10(e), §11.50, §11.70, §11.100–11.300** z ecfr.gov, ak budeš robiť tvrdenia o duálnom súlade.

**Priorita 3 — pred zákazníckym materiálom:**

13. **Príloha 1 vyhlášky 161/2019** — potvrdiť, že teplomery nie sú určené meradlá.
14. **Odborné usmernenie MZ SR o chladovom reťazci vakcín** — identifikátor, dátum, aktuálna platnosť, a či trojročná lehota stále platí. Zdroj bol sekundárny (Solen) a môže byť zastaraný.
15. **Či dvakrát denne vykonávaný manuálny vakcínový záznam prežíva popri novom automatickom pravidle.**
16. **Čísla z Výročnej správy ŠÚKL 2024 (73 / 478)** — čítať priamo z PDF, nie zo sumarizácie. A pokúsiť sa nájsť správu za rok 2025.
17. **Počet registrovaných lekární a licencovaných veľkodistribútorov v SR** — veľkosť adresovateľného trhu, pravdepodobne z registrov ŠÚKL. **Je to najdôležitejší chýbajúci vstup pre sizing príležitosti.**
18. **Či ŠÚKL začal od júla 2026 vynucovať nové § 30 ods. 8**, a koľko lekární je ešte v nesúlade.
19. **Sankcie za nesúlad** (boli za platobnou bránou mediPRÁVNIKa).
20. **EMA guideline on declaration of storage conditions — príloha**, konkrétne či sa tam objavuje „Store below −15 °C".
21. **ICH Q1A(R2)** a **USP <1079.2>** pre MKT — druhé menované je najcennejšia chýbajúca citácia celého materiálu.
22. **Dátum a názov údajného FDA CSA guidance zo septembra 2025** na fda.gov.
23. **Aktuálny stav revízie Annexu 11** — kontroluj [EudraLex Volume 4](https://health.ec.europa.eu/medicinal-products/eudralex/eudralex-volume-4_en) kvartálne. Je to jediný spoľahlivý spôsob, ako sa dozvedieť, že sa niečo zmenilo.

**Materiály, ktoré sa oplatí kúpiť:**

- **ISPE GAMP 5, 2. vydanie (2022)** — celý GAMP obsah v tomto reporte je zo sekundárneho komentára.
- **ISPE GAMP Records and Data Integrity Guide** a jeho companion *Data Integrity by Design* — **najpravdepodobnejší zdroj praktického usmernenia k bufferovaným dátam z prístrojov**, čo je presne tvoja najväčšia regulačná medzera.
- **ISPE Good Practice Guide: Controlled Temperature Chambers** (2. vyd.) — obsahuje tabuľky počtov senzorov (Figure 4.1) a odkaz na NF X15-140, ktoré sa nepodarilo prečítať.

---

## Záver: čo z toho naozaj plynie

Najhodnotnejší výstup tohto výskumu nie je zoznam klauzúl, ale **rozdelenie sveta na tri zákaznícke segmenty, z ktorých každý má iný právny rámec, iný nákupný spúšťač a inú predajnú konverzáciu** — a poznanie, že slovenský lekárenský segment má od 1. júla 2026 prescriptívne pravidlo, kým distribučný segment nemá, a že práve táto asymetria určuje, čo sa komu predáva. Lekárni sa predáva splnenie konkrétnej vety vyhlášky za cenu, ktorú vyhláška implicitne strop­uje na pár stoviek eur hardvéru plus službu. Veľkoskladu a výrobcovi sa predáva schopnosť obhájiť vlastné rozhodnutia pred inšpektorom, pretože EU GDP aj Annex 11 sú v podstate rámce, ktoré hovoria „zdôvodni si to sám" — a väčšina malých slovenských prevádzok to zdôvodniť nevie. **Dokumentácia, nie hardvér, je produkt.**

Druhý záver je časový a mal by zmeniť tvoje priority. Revidovaný Annex 11 nie je publikovaný, prechodné obdobie ešte nezačalo a reálne povinné dodržiavanie príde okolo roku 2028. To znamená dve veci naraz: **validuj proti roku 2011, ale stavaj proti návrhu z roku 2025** — a máš na to približne dva roky, čo je presne toľko, koľko potrebuje jeden človek, aby postavil nemeniteľný audit trail, alarmový log podľa kapitoly 8, IAM podľa kapitoly 11, štruktúrovaný export podľa 12.9 a zálohovaciu disciplínu podľa kapitoly 16. Retrofit týchto vecí, najmä nemennosti audit trailu, je rádovo drahší než ich zabudovanie. Zároveň platí, že zákazníci začnú klásť otázky formulované jazykom návrhu **skôr, než bude návrh záväzný** — takže pripravenosť na návrh je predajným aktívom už dnes, nie až v roku 2028.

Tretí záver je o tom, čo ťa naozaj odlíši, a je nepohodlný: **tvoje najsilnejšie konkurenčné výhody sú tie, o ktorých by sa väčšine dodávateľov nechcelo hovoriť.** On-premise jediný binár bez cloudu je odpoveď na exit strategy podľa draft 7.5(viii), ktorú SaaS konkurencia dať nevie. Priznanie, že jednoosobová firma nemôže splniť oddelenie povinností, a doloženie technických kompenzačných opatrení podľa draft 2.3, je silnejšie než predstieranie, že problém neexistuje. A povedať lekárni, ktorá potrebuje dva senzory, že nepotrebuje validáciu počítačového systému, pretože vyhláška 129/2012 hovorí o vlastnosti *meradla* a nie o softvéri, je najlepší spôsob, ako si od nej o rok kúpiť aj mapovanie a monitoring. **V trhu, kde každý konkurent vyžaduje obchodný rozhovor a nikto nepublikuje cenu, je presnosť o tom, čo zákon vyžaduje a čo nie, samo o sebe konkurenčná stratégia.**

---

## Zoznam zdrojov

### Slovenské právo a trh

| Dokument | URL |
|---|---|
| Zákon č. 362/2011 Z. z. (Slov-Lex) | https://www.slov-lex.sk/ezbierky-fe/pravne-predpisy/SK/ZZ/2011/362 |
| Zákon č. 362/2011 Z. z., statické konsolidované znenie k 31.01.2025 | https://static.slov-lex.sk/static/SK/ZZ/2011/362/20250131.html |
| Zákon č. 362/2011 Z. z. (zakonypreludi) | https://www.zakonypreludi.sk/zz/2011-362 |
| Vyhláška č. 128/2012 Z. z. (Slov-Lex) | https://slov-lex.sk/pravne-predpisy/SK/ZZ/2012/128 |
| Vyhláška č. 128/2012 Z. z., vyhlásené znenie | https://static.slov-lex.sk/static/SK/ZZ/2012/128/vyhlasene_znenie.html |
| Vyhláška č. 128/2012 Z. z. (zakonypreludi) | https://www.zakonypreludi.sk/zz/2012-128 |
| Vyhláška č. 128/2012 Z. z. (ASPI) | https://www.aspi.sk/products/lawText/1/77336/0/2/vyhlaska-c-128-2012-zz-o-poziadavkach-na-spravnu-vyrobnu-prax-a-poziadavkach-na-spravnu-velkodistribucnu-prax |
| Vyhláška č. 129/2012 Z. z., PDF hosťované ŠÚKL (pôvodné znenie) | https://www.sukl.sk/buxus/docs/Inspekcia/Legislativa/vyhlaska_129_2012.pdf |
| Vyhláška č. 129/2012 Z. z., konsolidované znenie | https://www.zakonypreludi.sk/zz/2012-129 |
| **Vyhláška č. 419/2025 Z. z. (novela)** | https://www.zakonypreludi.sk/zz/2025-419 |
| mediPRÁVNIK: Automatické zaznamenávanie teploty v lekárňach od 1. 7. 2026 | https://www.medipravnik.sk/pravne-clanky/automaticke-zaznamenavanie-teploty-v-lekarnach-ako-nova-povinnost-od-1-jula-2026-2091/ |
| ŠÚKL Výročná správa 2024 | https://www.sukl.sk/buxus/docs/rozhodnutia/Vyrocna_sprava_2024_final.pdf |
| ŠÚKL: Zistené nedostatky pri priebežných inšpekciách v lekárňach | https://www.sukl.sk/hlavna-stranka/slovenska-verzia/inspekcia/lekarenstvo/zistene-nedostatky-pri-priebeznych-inspekciach-v-lekarnach?page_id=1724 |
| ŠÚKL: Zoznam osvedčení o dodržiavaní SVDP | https://www.sukl.sk/hlavna-stranka/slovenska-verzia/inspekcia/distribucia/zoznam-vydany-osvedceni-o-dodrziavani-spravnej-velkodistribucnej-praxe |
| ŠÚKL: Doklady pre vstupnú inšpekciu distribútora | https://sukl.sk/hlavna-stranka/slovenska-verzia/inspekcia/distribucia/doklady-potrebne-pre-vykonanie-vstupnej-inspekcie?page_id=1248 |
| ŠÚKL: Legislatíva (inšpekcia) | https://www.sukl.sk/hlavna-stranka/slovenska-verzia/inspekcia/legislativa?page_id=900 |
| SÚKL ČR: DIS-15 verze 4 (teplota pri skladovaní a preprave) | https://sukl.gov.cz/wp-content/uploads/2025/02/DIS-15_verze4_Sledovani-a-kontrola-teploty-pri-skladovani-a-preprave-leciv.pdf |
| Solen: Uchovávanie a distribúcia očkovacích látok | https://www.solen.sk/storage/file/article/Lancova.pdf |
| ÚVZ SR | https://www.uvzsr.sk/web/uvz |
| ÚNMS SR: FAQ metrológia | https://www.normoff.gov.sk/stranka/247/faq-metrologia/ |
| Vyhláška č. 161/2019 Z. z. o meradlách (ASPI) | https://www.aspi.sk/products/lawText/1/92023/0/2/vyhlaska-c-161-2019-zz-o-meradlach-a-metrologickej-kontrole |
| SNAS: Kalibračné laboratóriá | https://www.snas.sk/akreditacia/kalibracne-laboratoria |
| SNAS: Oblasť a rozsah akreditácie (PDF) | https://www.snas.sk/storage/app/uploads/public/690/b52/44e/690b5244e1cd6236226642.pdf |
| SNAS: MSA-L/07 | https://www.snas.sk/storage/app/uploads/public/61a/3a6/ca1/61a3a6ca14371704010175.pdf |
| KALIST AKL: Kalibrácia teplomerov | https://www.kalistakl.sk/kalibracia-teplomerov |
| KALIST AKL: Kalibrované prístroje pre ordinácie a lekárne | https://www.kalistakl.sk/kalibrovane-pristroje-pre-ordinacie-a-lekarne |
| Metershop: kategória pre lekárne | https://metershop.sk/kategoria/teplomery/s-certifikatom-o-kalibracii/lekarne-zdravotnictvo/s-automatickym-zaznamom-pre-lekarne/ |
| Metershop: Kalibrácia teplomerov (cenník) | https://metershop.sk/sluzby/kalibracia-teplomerov/ |
| Meracie prístroje: Comet | https://www.meraciepristroje.sk/znacky/comet~m565 |
| LogHub.sk | https://www.loghub.sk/ |
| LogHub CZ: teplotní mapování | https://loghub.cz/teplotni-mapovani/ |
| Lewik: Povinnosti držiteľa povolenia na veľkodistribúciu | https://www.lewik.org/term/2498/povinnosti-drzitela-povolenia-na-velkodistribuciu-liekov-zakon-o-liekoch-a-zdravotnickych-pomockach/ |

### Európske predpisy a usmernenia

| Dokument | URL |
|---|---|
| **EudraLex Volume 4 — oficiálny index (kontrola stavu 21. 9. 2026)** | https://health.ec.europa.eu/medicinal-products/eudralex/eudralex-volume-4_en |
| **EU GMP Annex 11 Computerised Systems, rev. január 2011 (V PLATNOSTI)** | https://health.ec.europa.eu/system/files/2016-11/annex11_01-2011_en_0.pdf |
| **EU GMP Annex 15 Qualification and Validation (od 1. 10. 2015)** | https://health.ec.europa.eu/system/files/2016-11/2015-10_annex15_0.pdf |
| EU GMP Chapter 4 Documentation (2011) | https://health.ec.europa.eu/system/files/2016-11/chapter4_01-2011_en_0.pdf |
| **Draft revidovaný Annex 11, konzultácia 7. 7. 2025 (NIE JE V PLATNOSTI)** | https://health.ec.europa.eu/document/download/40231f18-e564-4043-94de-c031f813d38b_en?filename=mp_vol4_chap4_annex11_consultation_guideline_en.pdf |
| Draft revidovaná Chapter 4 Documentation (NIE JE V PLATNOSTI) | https://health.ec.europa.eu/document/download/fa336a6e-753b-46fc-b53b-9178bacd8878_en?filename=mp_vol4_chap4_consultation_guideline_en.pdf |
| Draft nový Annex 22 Artificial Intelligence (neanalyzovaný) | https://health.ec.europa.eu/document/download/5f38a92d-bb8e-4264-8898-ea076e926db6_en?filename=mp_vol4_chap4_annex22_consultation_guideline_en.pdf |
| Konzultačná stránka EudraLex Vol. 4 (dátumy, stav) | https://health.ec.europa.eu/consultations/stakeholders-consultation-eudralex-volume-4-good-manufacturing-practice-guidelines-chapter-4-annex_en |
| **EU GDP 2013/C 343/01, OJ C 343, 23.11.2013 (PDF)** | https://eur-lex.europa.eu/LexUriServ/LexUriServ.do?uri=OJ:C:2013:343:0001:0014:EN:PDF |
| EU GDP — CELEX 52013XC1123(01) | https://eur-lex.europa.eu/legal-content/EN/TXT/?uri=CELEX:52013XC1123(01) |
| EU GDP — kópia hostovaná Komisiou (číslovanie neoverené) | https://health.ec.europa.eu/document/download/9f28179a-a6f8-418a-a5fc-ecfbb3ae3ba8_en |
| EU GDP — ďalšia kópia Komisie | https://health.ec.europa.eu/system/files/2016-11/2013_c343_01_en_0.pdf |
| EMA: Good distribution practice (potvrdenie platnosti 2013/C 343/01) | https://www.ema.europa.eu/en/human-regulatory-overview/post-authorisation/compliance-post-authorisation/good-distribution-practice |
| **C(2026) 4135 final — GMP Annex 19, NIE GDP (falošná zhoda)** | https://health.ec.europa.eu/document/download/831d392e-bcc4-4552-8011-2b724b8655de_en?filename=c2026-4135_en_act_en.pdf |
| Smernica 2001/83/ES (EUR-Lex) | https://eur-lex.europa.eu/eli/dir/2001/83/oj/eng |
| EMA: Guideline on declaration of storage conditions, CPMP/QWP/609/96 Rev 2 | https://www.ema.europa.eu/system/files/documents/scientific-guideline/wc500003468_en.pdf |
| EMA: Guideline on declaration of storage conditions (aktuálna) | https://www.ema.europa.eu/en/documents/scientific-guideline/guideline-declaration-storage-conditions_en.pdf |

### Integrita dát a inšpekčné usmernenia

| Dokument | URL |
|---|---|
| **PIC/S PI 041-1 (1. 7. 2021) — ALCOA+ §7.5** | https://picscheme.org/docview/4234 |
| PIC/S PI 047-1 (účinné látky, 1. 7. 2018) | https://picscheme.org/docview/2466 |
| **MHRA GXP Data Integrity Guidance and Definitions (marec 2018)** | https://assets.publishing.service.gov.uk/media/5aa2b9ede5274a3e391e37f3/MHRA_GxP_data_integrity_guide_March_edited_Final.pdf |
| MHRA: landing page | https://www.gov.uk/government/publications/guidance-on-gxp-data-integrity |
| MHRA Inspectorate blog: Temperature mapping – an introduction (14. 7. 2016) | https://mhrainspectorate.blog.gov.uk/2016/07/14/temperature-mapping-an-introduction/ |
| WHO TRS 996 Annex 5 (2016) — čas, oneskorené zápisy | https://rx-360.org/wp-content/uploads/2018/08/WHO_TRS_996_annex05.pdf |
| FDA: Data Integrity and Compliance With Drug CGMP Q&A (dec. 2018) | https://www.fda.gov/media/119267/download |
| 21 CFR Part 11 (nezískané) | https://www.ecfr.gov/current/title-21/chapter-I/subchapter-A/part-11 |

### Mapovanie a kvalifikácia

| Dokument | URL |
|---|---|
| **WHO TRS 961 Annex 9 Supplement 8 — Temperature mapping of storage areas** | https://cdn.who.int/media/docs/default-source/medicines/norms-and-standards/guidelines/distribution/trs961-annex9-supp8.pdf |
| **WHO TRS 961 Annex 9 Supplement 7 — Qualification of temperature-controlled storage areas** | https://cdn.who.int/media/docs/default-source/medicines/norms-and-standards/guidelines/distribution/trs961-annex9-supp7.pdf?sfvrsn=3c9a94ed_2 |
| WHO TRS 992 Annex 5 (konsolidované technické doplnky) | https://www.who.int/publications/m/item/trs992-annex5 |
| WHO: How to temperature map cold chain equipment and storage areas (2022) | https://www.coldchainplatform.com/resources/pdf/en/How-to-temperature-map-cold-chain-equipment-and-storage-areas_WHO_20220411.pdf |
| WHO supplement 13 (prepravné kontajnery — nie fixné sklady) | https://cdn.who.int/media/docs/default-source/medicines/norms-and-standards/guidelines/distribution/trs961-annex9-supp13.pdf |
| ISPE: Controlled Temperature Chamber Mapping (white paper) | https://ispe.org/pharmaceutical-engineering/white-papers/controlled-temperature-chamber-mapping |
| Vaisala: Guidance on warehouse mapping sensor placement | https://www.vaisala.com/en/blog/2020-10/video-qa-guidance-warehouse-mapping-sensor-placement |
| Vaisala: Managing GxP Monitoring System Alarms | https://www.vaisala.com/en/blog/2023-08/managing-gxp-monitoring-system-alarms |
| Vaisala viewLinc 5.0 IQ/OQ SAMPLE (161 strán) | https://www.vaisala.com/sites/default/files/documents/VIM-G-Vaisala-viewLinc-5_0-IQOQ-SAMPLE.pdf |
| Kaye Instruments: Sensor quantity and placement | https://www.kayeinstruments.com/en/news/blog-post/guidelines-for-selecting-sensor-quantity-and-optimal-placement-in-area-temperature-mapping |
| NHS Specialist Pharmacy Service: Establishing alerts for temperature monitoring systems | https://sps.nhs.uk/articles/establishing-alerts-for-temperature-monitoring-systems/ |
| CDC: Temperature Monitoring Equipment | https://www.cdc.gov/vaccines/hcp/storage-handling/monitoring-equipment.html |
| Rotronic: Temperature Mapping & GDP-Compliant Warehouse Qualification | https://www.rotronic.com/pub/media/productattachments/files/5/3/53024e_portground.pdf |
| Rotronic RMS: Mean Kinetic Temperature | https://service.rotronic.com/manual/mean-kinetic-temperature.html |
| LogTag Analyzer Help: Mean Kinetic Temperature | https://help.logtagrecorders.com/LTA3/Topics/UserGuide/Chapter2-Using_Analyzer/Mean_Kinetic_Temperature.htm |
| PDA TR 39 (2021) — obsah (platený, prevažne o preprave) | https://store.pda.org/TableOfContents/2021TR3921_TOC.pdf |

### Validácia, GAMP, dodávatelia

| Dokument | URL |
|---|---|
| ECA Academy: Annex 11 Draft – First Analysis | https://www.gmp-compliance.org/gmp-news/annex-11-draft-first-analysis |
| qmsdoc.com: Revision of PIC/S GMP Annex 11 (časový plán) | https://qmsdoc.com/2026/01/15/revision-of-pic-s-gmp-annex-11-computerised-systems/ |
| Epista: Preparing for EMA's 2026 GMP revisions | https://www.epista.com/knowledge/preparing-for-emas-2026-good-manufacturing-practice-gmp-revisions-chapter-4-annex-11-annex-22 |
| Investigations of a Dog: Draft Annex 11 Section 13 (29. 7. 2025) | https://investigationsquality.com/2025/07/29/draft-annex-11-section-13-what-the-proposed-electronic-signature-rules-mean/ |
| SimplerQMS: 21 CFR Part 11 vs EU Annex 11 | https://simplerqms.com/21-cfr-part-11-vs-eu-annex-11/ |
| GoValidation: GAMP 5 software category examples | https://govalidation.com/blog/gamp5-software-category-examples/ |
| Kneat: What is GAMP 5 | https://kneat.com/article/what-is-gamp-5/ |
| IntuitionLabs: GAMP 5 computerized system validation | https://intuitionlabs.ai/articles/gamp-5-computerized-system-validation-pharma |
| Osborne: GAMP 5 and the supplier (PDF) | https://secure.toolkitfiles.co.uk/clients/20735/sitedata/files/GAMP_5_and_the_supplier.pdf |
| Rx-360: Supplier Assessment Questionnaire | https://rx-360.org/supplier-assessment-questionnaire/ |
| ECA Academy: Appendix 4 Supplier Quality Questionnaire | https://www.gmp-compliance.org/guidelines/gmp-guideline/appendix-4-supplier-quality-questionnaire-instructions-for-completion-e-extended-quality-questionnaire |
| SenseAnywhere: IQ OQ PQ phases of temperature monitoring qualification | https://www.senseanywhere.com/what-are-the-iq-oq-and-pq-phases-of-temperature-monitoring-system-qualification/ |
| Eupry: IQ OQ PQ for warehouses | https://eupry.com/iq-oq-pq/warehouses/ |
| Eupry: ULT & freezer qualification | https://eupry.com/iq-oq-pq/freezers/ |
| Eupry: Warehouse mapping guide | https://eupry.com/temperature-mapping/warehouse-mapping-guide/ |
| Eupry: GDP mapping guide | https://eupry.com/gdp/gdp-mapping/ |
| Eupry: Monitoring pricing | https://eupry.com/temperature-monitoring/monitoring-price/ |
| Eupry: Mapping pricing | https://eupry.com/temperature-monitoring/mapping-price/ |
| ELPRO: Pharmacy temperature monitoring | https://www.elpro.com/en/pharmacy-temperature-monitoring |
| ELPRO: Documentation and Computerized Systems Regulations | https://www.elpro.com/en/learn/documentation-computerized-regulations |
| ELPRO: Qualification & Mapping in Temperature Monitoring | https://www.elpro.com/en/temperature-monitoring/qualification-mapping |
| ELPRO: GxP temperature mapping services | https://www.elpro.com/en/gxp-services/temperature-mapping |
| Testo SK: Saveris kvalifikácia | https://www.testo.com/sk-SK/services/saveris-kvalifikacia |
| Comet System SK | https://sk.cometsystem.com/ |
