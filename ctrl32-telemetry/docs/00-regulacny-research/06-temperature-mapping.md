# Temperature Mapping and Permanent Temperature Monitoring of Pharmaceutical Storage Areas — Practical Methodology

Scope note: EU jurisdiction (EU GDP 2013/C 343/01), current as of September 2026. Primary sources used: WHO TRS 961 Annex 9 technical supplements 7 and 8, EU GDP guidelines, MHRA Inspectorate blog, EMA declaration-of-storage-conditions guideline, ISPE Good Practice Guide (public white paper extract), and vendor application notes (Vaisala, Kaye, LogTag, Rotronic, Eupry). Vendor sources are flagged as such — they are not regulatory guidance, but they are the only public source of several concrete numbers (sensor counts, grid spacing) that regulators deliberately leave open.

---

## Q1. How many data loggers are needed, as a function of room volume?

### Takeaway
No regulator publishes a "loggers per m³" formula — all of them require a **documented, risk-based rationale** instead. The only quantitative regulatory anchor is WHO's **grid spacing rule (one logger every 5–10 m horizontally, three or more vertical levels)**; vendor guides convert this into practical counts (16 sensors for a chamber <20 m³; roughly 1 logger per 25–50 m² for warehouses; 9–15 for small chambers, 15–30 for cold rooms).

### Cited Findings
- WHO horizontal grid rule: "EDLMs should be arranged in a grid fashion along the width and length of the area so that the area is reasonably covered, with EDLMs located every 5–10 metres"; very large facilities may justify 20–30 m spacing — [WHO TRS 961 Annex 9, Supplement 8, "Temperature mapping of storage areas", §2.2.6 Step 5](https://cdn.who.int/media/docs/default-source/medicines/norms-and-standards/guidelines/distribution/trs961-annex9-supp8.pdf)
- WHO vertical rule: for rooms with ceiling height **≤3.6 m**, loggers at **three height levels — floor, 1.2 m and 3.0 m**. For rooms **>3.6 m**, multiple vertical arrays; the worked example for a 6 m high room uses **0.3 m, 1.8 m, 3.6 m and 5.4 m** — [WHO TRS 961 Annex 9 Supplement 8, §2.2.6 Step 5](https://cdn.who.int/media/docs/default-source/medicines/norms-and-standards/guidelines/distribution/trs961-annex9-supp8.pdf)
- ISPE-derived count for small chambers (<20 m³ / ~700 cu ft): **16 sensors = 1 centre + 8 corners + 6 face midpoints (one per side) + 1 adjacent to the control sensor** — [Vaisala, "Video Q&A: Guidance on warehouse mapping sensor placement"](https://www.vaisala.com/en/blog/2020-10/video-qa-guidance-warehouse-mapping-sensor-placement), citing the ISPE Good Practice Guide for Controlled Temperature Chamber Mapping
- Vertical stacks: **minimum 3 sensors per stack (low, middle, high)**; 2-sensor stacks only acceptable if ceiling height is **under 2.5 m (8 ft)**. "Sensors should be placed in three planes in three dimensions" (attributed to USP General Chapter <1079>) — [Vaisala](https://www.vaisala.com/en/blog/2020-10/video-qa-guidance-warehouse-mapping-sensor-placement)
- Lateral spacing between stacks: **max 20 m for ambient warehouses** (WHO permits up to 30 m for very large facilities); **max 8 m for cold rooms and freezers** — [Vaisala](https://www.vaisala.com/en/blog/2020-10/video-qa-guidance-warehouse-mapping-sensor-placement)
- Vendor baseline counts by chamber class: **small chambers and equipment 9–15 loggers** (structured grid); **cold rooms and controlled areas 15–30 loggers** depending on usable volume and airflow complexity; **warehouses and large storage ≈ one logger per 25 to 50 square metres**, refined by risk factors — [Kaye Instruments, "Guidelines for Selecting Sensor Quantity and Optimal Placement in Area Temperature Mapping"](https://www.kayeinstruments.com/en/news/blog-post/guidelines-for-selecting-sensor-quantity-and-optimal-placement-in-area-temperature-mapping)
- Standard grid layouts: **9-point layout** = 3×3 matrix for small chambers (4 corners, 4 mid-wall, 1 centre), with vertical replication required if chamber height exceeds 2–3 m; **27-point layout** = 3×3×3 for larger cold rooms, spacing set by dividing each room dimension into three equal segments — [Kaye Instruments](https://www.kayeinstruments.com/en/news/blog-post/guidelines-for-selecting-sensor-quantity-and-optimal-placement-in-area-temperature-mapping)
- Kaye's own caveat: "There is no universal calculation model; defensible studies combine grid-based layouts with risk-based placement strategies." — [Kaye Instruments](https://www.kayeinstruments.com/en/news/blog-post/guidelines-for-selecting-sensor-quantity-and-optimal-placement-in-area-temperature-mapping)
- ISPE requires a written rationale rather than a fixed count: "A rationale that describes the location and number of sensors to be used should be developed" and approved by Quality and the system owner. ISPE cites **French Standard NF X15-140** for chambers up to ~700 cu ft (~20 m³) — [ISPE, "Controlled Temperature Chamber Mapping", Pharmaceutical Engineering white paper](https://ispe.org/pharmaceutical-engineering/white-papers/controlled-temperature-chamber-mapping)
- MHRA states only: "Use calibrated monitoring probes in sufficient numbers dependant on the size of the storage area." — [MHRA Inspectorate blog, "Temperature mapping — an introduction" (14 July 2016)](https://mhrainspectorate.blog.gov.uk/2016/07/14/temperature-mapping-an-introduction/)
- Freezer-specific vendor grids: small freezers "minimum 3×3 vertical grid (9 loggers)"; medium walk-in freezers "5×3 to 5×4 grid (15–20 loggers)"; large freezer rooms grid spacing every 5–10 m horizontally with vertical layers — [Eupry, "ULT & freezer qualification for pharma"](https://eupry.com/iq-oq-pq/freezers/) (vendor source, not regulatory)

### Inferences
- A defensible commercial offer can be priced off a simple deterministic model derived from WHO: `N ≈ (ceil(L/8) × ceil(W/8)) × vertical_levels`, where vertical_levels = 3 for rooms ≤3.6 m and 4+ above, plus extra sensors at identified risk points (doors, HVAC outlets, external walls, heaters, control sensor). This reproduces the ~1 logger / 25–50 m² figure for a typical warehouse with 3 levels.
- Spacing should be tightened from 10 m to ~8 m (and to 5 m in cold rooms/freezers) because cold rooms have steeper gradients and a smaller absolute tolerance band.
- For a 2–8 °C pharmacy fridge the 9-point grid (3 shelves × corners+centre) is the practical minimum; for a walk-in cold room the 27-point layout is the defensible default.

### Gaps
- The full ISPE Good Practice Guide "Controlled Temperature Chambers — Commissioning and Qualification, Mapping and Monitoring (2nd Ed.)" and PDA TR 39 are paywalled; the exact sensor-count tables in ISPE Figure 4.1 and NF X15-140 could not be read. The PDF at usp.org hosting TR 39 returned HTTP 403.
- No published guidance expresses logger count per **m³** directly; all quantitative guidance is either grid-spacing or per-m² vendor heuristics.

---

## Q2. How long should a mapping study run?

### Takeaway
WHO sets the two hard numbers: **24–72 h for temperature-controlled spaces (cold rooms, freezers, chambers)** and a **minimum of 7 consecutive days including 5 working days and 2 weekend days for ambient/warehouse storage**. Seasonal studies (warmest and coldest season) apply mainly to ambient warehouses, not usually to cold/freezer rooms.

### Cited Findings
- "for between 24 and 72 hours, or longer if justified" — mapping duration for temperature-controlled storage areas — [WHO TRS 961 Annex 9 Supplement 8, §2.2.6 Step 9](https://cdn.who.int/media/docs/default-source/medicines/norms-and-standards/guidelines/distribution/trs961-annex9-supp8.pdf)
- Ambient storage: "minimum of seven consecutive days – including five working days and two weekend days" — [WHO TRS 961 Annex 9 Supplement 8, §2.2.6 Step 9](https://cdn.who.int/media/docs/default-source/medicines/norms-and-standards/guidelines/distribution/trs961-annex9-supp8.pdf)
- Seasonal studies: "at least two temperature-mapping studies may be needed in each area to observe the effect of seasonal variation … during the warmest season and one during the coldest season"; WHO notes this is typically **not required for cold rooms and freezer rooms** — [WHO TRS 961 Annex 9 Supplement 8, §2.2.4](https://cdn.who.int/media/docs/default-source/medicines/norms-and-standards/guidelines/distribution/trs961-annex9-supp8.pdf)
- Dual/redundant refrigeration units: the study must include a period of operation with **each unit running separately** — [WHO TRS 961 Annex 9 Supplement 8, §2.2.6](https://cdn.who.int/media/docs/default-source/medicines/norms-and-standards/guidelines/distribution/trs961-annex9-supp8.pdf)
- Qualification-stage minimum: "The whole area should be monitored for a period of at least 24 hours using EDLMs" (OQ mapping), with PQ repeating the same protocol **when the facility is fully loaded** — [WHO TRS 961 Annex 9, Supplement 7, "Qualification of temperature-controlled storage areas", §2.5.6 Table 8 and §2.6.2](https://cdn.who.int/media/docs/default-source/medicines/norms-and-standards/guidelines/distribution/trs961-annex9-supp7.pdf?sfvrsn=3c9a94ed_2)
- ISPE: mapping requires "at least 24 hours (include time to capture defrost cycles and switchover of redundant systems)"; seasonal testing is decided by risk assessment comparing external conditions against **95% summer/winter design conditions** — [ISPE white paper](https://ispe.org/pharmaceutical-engineering/white-papers/controlled-temperature-chamber-mapping)
- ISPE empty vs loaded: two scenarios should be considered — "empty store with maximum specified load arriving" and "store with maximum load" restricting airflow — [ISPE white paper](https://ispe.org/pharmaceutical-engineering/white-papers/controlled-temperature-chamber-mapping)
- MHRA: "An initial temperature mapping exercise should be carried out on the storage area before use, under representative conditions"; a 7-day period is the common industry norm but must be justified by risk assessment; "Repeat the exercise to cover seasonal variations or if the storage area is subsequently reconfigured" — [MHRA Inspectorate blog](https://mhrainspectorate.blog.gov.uk/2016/07/14/temperature-mapping-an-introduction/). MHRA's blog notes mapping is typically run in **January/February and July/August** to capture extremes.
- Vendor practice for freezers: empty chamber (OQ) 24 h minimum; loaded chamber (PQ) 48–72 h; must include at least one defrost cycle on auto-defrost models — [Eupry](https://eupry.com/iq-oq-pq/freezers/) (vendor source)

### Inferences
- A commercially clean package is: **cold rooms/fridges/freezers = 72 h (covers ≥2 defrost cycles and a weekend-free operating cycle); ambient warehouse = 7 days, twice a year (summer + winter)**. This matches WHO minima with margin and is easy to defend to an inspector.
- Because WHO requires both empty (OQ) and loaded (PQ) studies, a full first-time qualification of a new warehouse realistically means 2 × 7 days plus 2 seasonal repeats — a meaningful billable scope, not a one-visit job.

### Gaps
- No regulatory source gives a maximum duration or a statistical rule (e.g. number of defrost cycles) for when 24 h is sufficient versus 72 h; it is left to justification.

---

## Q3. Where do the sensors go — worst-case locations

### Takeaway
Grid coverage establishes the baseline; **risk-based additions at doors, HVAC supply/return, external walls, heaters, roof space and the top of racking** find the actual hot/cold spots. Both EU GDP and MHRA explicitly require the permanent monitors to be placed at the **extremes of fluctuation** found by the mapping.

### Cited Findings
- EU GDP, §3.2.1: "Temperature monitoring equipment should be located according to the results of the mapping exercise, ensuring that monitoring devices are positioned in the areas that experience the extremes of fluctuations." — [EU Guidelines on Good Distribution Practice of medicinal products for human use (2013/C 343/01)](https://eur-lex.europa.eu/LexUriServ/LexUriServ.do?uri=OJ:C:2013:343:0001:0014:EN:PDF)
- EU GDP, §3.2.1, small premises: "For small premises of a few square meters which are at room temperature, an assessment of potential risks (e.g. heaters) should be conducted and temperature monitors placed accordingly." — [EU GDP 2013/C 343/01](https://eur-lex.europa.eu/LexUriServ/LexUriServ.do?uri=OJ:C:2013:343:0001:0014:EN:PDF)
- MHRA: positioning should focus on "areas that experience the extremes of fluctuations" and include potential problem areas such as **heaters, windows, doors and high-storage locations**; data "should be recorded … to determine the most appropriate positions for the permanent monitoring probes and should cover the areas that have the widest temperature fluctuations" — [MHRA Inspectorate blog](https://mhrainspectorate.blog.gov.uk/2016/07/14/temperature-mapping-an-introduction/)
- ISPE: place sensors "where product will be stored" and where "changes in the conditions are likely to be found during use"; at locations representing "maximum and minimum temperatures (and/or maximum or minimum humidity)"; and adjacent to the control sensor "where appropriate" — [ISPE white paper](https://ispe.org/pharmaceutical-engineering/white-papers/controlled-temperature-chamber-mapping)
- Small equipment (fridges, freezers, cabinets): "EDLMs should be used to record temperatures at the top and bottom of the cabinet as well as at the rear and front" — [WHO TRS 961 Annex 9 Supplement 7, §2.7](https://cdn.who.int/media/docs/default-source/medicines/norms-and-standards/guidelines/distribution/trs961-annex9-supp7.pdf?sfvrsn=3c9a94ed_2)
- Kaye risk-density rule: "Concentrate density in high-risk zones (doors, HVAC interfaces, perimeter walls)"; position loggers **at representative product storage heights**; include vertical stratification assessment; document all placement decisions with traceability — [Kaye Instruments](https://www.kayeinstruments.com/en/news/blog-post/guidelines-for-selecting-sensor-quantity-and-optimal-placement-in-area-temperature-mapping)
- Freezer high-risk zones: "door area, compressor side, top shelf, load center" — [Eupry](https://eupry.com/iq-oq-pq/freezers/) (vendor source)
- CDC probe placement for fridges/freezers (product-temperature approach): "Place the buffered probe of the DDL in the center of the unit with the vaccines surrounding it"; avoid placement "near the unit's walls, floor, vent, ceiling, or door"; keep "the DDL's active digital display outside the unit." — [CDC, Temperature Monitoring Equipment](https://www.cdc.gov/vaccines/hcp/storage-handling/monitoring-equipment.html) (US source; useful practice, not EU law)

### Inferences
- There is a deliberate asymmetry worth selling: **mapping loggers go everywhere including worst cases; permanent monitors go only at the worst cases plus a representative reference point.** The mapping report is the document that converts "we guessed" into "we measured", which is precisely the inspection finding a GDP wholesaler wants to avoid.
- Note the tension between CDC's "centre of the unit, surrounded by product" (protects against nuisance alarms, represents product) and GDP's "extremes of fluctuation" (protects against undetected excursions). For a GDP warehouse the EU rule wins; for a small fridge, a common compromise is one buffered probe centrally plus mapping evidence that the worst shelf stays in range.

### Gaps
- No source gives a numeric minimum distance from walls, evaporator or door for logger placement in ambient warehouses (vendors typically use 20–50 cm but I found no citable published figure).

---

## Q4. Deliverables of a mapping study

### Takeaway
Two controlled documents: an **approved mapping protocol before the study** and a **peer-reviewed mapping report after it**, whose central output is a **zoning diagram of safe/unsafe storage locations** and a **justified list of permanent monitoring positions**.

### Cited Findings
- WHO report structure (§2.2.7): Introduction; Summary with deviations; Conclusions and recommendations; Annexes containing the **site survey, raw data sheets, graphs, analysis results, calibration certificates, and CAPA forms** if needed — [WHO TRS 961 Annex 9 Supplement 8, §2.2.7](https://cdn.who.int/media/docs/default-source/medicines/norms-and-standards/guidelines/distribution/trs961-annex9-supp8.pdf)
- WHO key deliverables: (1) a documented mapping protocol **before the study begins**; (2) a completed mapping report **with peer review approval**; (3) a **visual zoning diagram showing safe and unsafe storage locations** — [WHO TRS 961 Annex 9 Supplement 8](https://cdn.who.int/media/docs/default-source/medicines/norms-and-standards/guidelines/distribution/trs961-annex9-supp8.pdf)
- The regulatory purpose of the report is to position permanent monitoring: EU GDP §3.2.1 and MHRA both make the permanent probe positions **an output of the mapping**, not an independent decision — [EU GDP 2013/C 343/01](https://eur-lex.europa.eu/LexUriServ/LexUriServ.do?uri=OJ:C:2013:343:0001:0014:EN:PDF); [MHRA Inspectorate blog](https://mhrainspectorate.blog.gov.uk/2016/07/14/temperature-mapping-an-introduction/)
- Calibration certificates are part of the deliverable and their validity is load-bearing: "Expired certificates invalidate associated data" — [Eupry](https://eupry.com/iq-oq-pq/freezers/) (vendor source, consistent with WHO's calibration-certificate requirement)

### Inferences
- A saleable deliverable set for a sole trader: (1) mapping protocol with acceptance criteria and sensor layout drawing; (2) raw data export (CSV) plus per-logger graphs; (3) min/max/mean/MKT table per logger; (4) hot/cold spot heat map or zoning drawing; (5) permanent-sensor placement recommendation with rationale; (6) deviations/CAPA; (7) calibration certificate annex; (8) approval/peer-review page. Items 4 and 5 are the ones that directly justify the subsequent LoRaWAN sensor sale.

### Gaps
- WHO does not prescribe a template for the hot/cold-spot heat map; no regulator specifies statistical treatment (e.g. whether to report standard deviation or only min/max).

---

## Q5. When must re-mapping be done?

### Takeaway
Trigger-based plus periodic: **WHO suggests a periodic re-map "for example, every three years"**, while EU GDP and MHRA make re-mapping mandatory on **significant modification of the facility or temperature-controlling equipment**, with the interval otherwise set by risk assessment.

### Cited Findings
- WHO: "subsequent mapping exercises may also be required periodically – for example, every three years"; additional triggers listed are **significant modifications, changes in loading pattern, equipment alterations, and unexplained variability** — [WHO TRS 961 Annex 9 Supplement 8, §1.1](https://cdn.who.int/media/docs/default-source/medicines/norms-and-standards/guidelines/distribution/trs961-annex9-supp8.pdf)
- EU GDP §3.2.1: "The mapping exercise should be repeated according to the results of a risk assessment exercise or whenever significant modifications are made to the facility or the temperature controlling equipment." — [EU GDP 2013/C 343/01](https://eur-lex.europa.eu/LexUriServ/LexUriServ.do?uri=OJ:C:2013:343:0001:0014:EN:PDF)
- MHRA: remapping occurs "whenever significant modifications are made to the facility or the temperature controlling equipment", and a risk assessment "would also define and justify the regularity of any future mapping exercises" — [MHRA Inspectorate blog](https://mhrainspectorate.blog.gov.uk/2016/07/14/temperature-mapping-an-introduction/)
- WHO qualification supplement: "Further qualification exercises should be conducted whenever significant modifications are made to the installation, or to the way in which it is used"; **no calendar-based re-qualification interval is specified** — [WHO TRS 961 Annex 9 Supplement 7, §1.1](https://cdn.who.int/media/docs/default-source/medicines/norms-and-standards/guidelines/distribution/trs961-annex9-supp7.pdf?sfvrsn=3c9a94ed_2)

### Inferences
- The defensible commercial position: **3-year periodic re-mapping as default (WHO's own example), with immediate re-mapping on racking layout change, HVAC/chiller replacement or change, change of stored product temperature class, extension of the area, or a pattern of unexplained excursions.** Framing the 3-year cycle as "WHO's own example" makes it easy to sell as a recurring service rather than a one-off.
- Note that EU GDP does **not** state 3 years; a firm that claims "EU law requires re-mapping every 3 years" would be overstating. The honest framing is "EU GDP requires risk-based periodicity; WHO offers 3 years as the worked example."

### Gaps
- No EU source names a maximum interval. A wholesaler that sets 5 years with a documented risk assessment is not automatically non-compliant.

---

## Q6. Accuracy / uncertainty required of mapping loggers vs permanent monitors

### Takeaway
WHO sets a hard number for mapping loggers: **NIST-traceable 3-point calibration with maximum error ±0.5 °C at each calibration point**. ISPE adds the decisive practical rule: device accuracy must be small relative to the tolerance band — **±0.3 °C is suitable for a 2–8 °C application, ±2 °C is explicitly not**.

### Cited Findings
- WHO EDLM specification: loggers must "have a US National Institute of Standards and Technology (NIST)-traceable 3-point calibration … with a guaranteed error of no more than ± 0.5 °C at each calibration point"; calibration points are **one below the range minimum, one at the midpoint, one above the maximum** — [WHO TRS 961 Annex 9 Supplement 8, §2.1](https://cdn.who.int/media/docs/default-source/medicines/norms-and-standards/guidelines/distribution/trs961-annex9-supp8.pdf)
- WHO recording interval for mapping: "1 minute to 15 minutes (maximum)" — [WHO TRS 961 Annex 9 Supplement 8, §2.1](https://cdn.who.int/media/docs/default-source/medicines/norms-and-standards/guidelines/distribution/trs961-annex9-supp8.pdf)
- WHO EDLM range requirement: measurement capability "from −30.0 °C to +60.0 °C", with "user-programmable data sampling period allowing time intervals to be set in the range from 1 minute to 15 minutes or more" — [WHO TRS 961 Annex 9 Supplement 7, §2.1](https://cdn.who.int/media/docs/default-source/medicines/norms-and-standards/guidelines/distribution/trs961-annex9-supp7.pdf?sfvrsn=3c9a94ed_2)
- ISPE fitness-for-purpose rule: a device with "± 2 °C" accuracy "would not be appropriate", while "± 0.3 °C" is suitable for 2–8 °C ranges — [ISPE white paper](https://ispe.org/pharmaceutical-engineering/white-papers/controlled-temperature-chamber-mapping)
- ISPE guard-band / decision-limit method for handling measurement uncertainty against acceptance criteria: **GB = k × u_system** with k > 2.00; low decision limit **DL ≥ Limit + [GB − Usr]**; high decision limit **DL ≤ Limit − [GB − Usr]** — [ISPE white paper](https://ispe.org/pharmaceutical-engineering/white-papers/controlled-temperature-chamber-mapping)
- Permanent installed sensors in a qualified storage area: all "critical controllers and sensors should have: a current calibration certificate, with the certificate available; a calibration that is traceable to national standards; an attached calibration tag" — [WHO TRS 961 Annex 9 Supplement 7, §2.5.2](https://cdn.who.int/media/docs/default-source/medicines/norms-and-standards/guidelines/distribution/trs961-annex9-supp7.pdf?sfvrsn=3c9a94ed_2)
- Monitoring-device accuracy benchmark for refrigerated product: "Recommended uncertainty of ±0.5 °C (±1 °F)", with calibration certificates confirming the same tolerance; recalibration "every 2 to 3 years, or according to the manufacturer's recommended schedule" — [CDC, Temperature Monitoring Equipment](https://www.cdc.gov/vaccines/hcp/storage-handling/monitoring-equipment.html) (US source)
- ULT calibration: calibration must include the actual operating temperature (−80 °C for ULT units); ISO 17025 traceability; annual or per manufacturer — [Eupry](https://eupry.com/iq-oq-pq/freezers/) (vendor source)

### Inferences
- For the LoRaWAN business this is the key technical gate: a generic ±1 °C LoRaWAN sensor is defensible for a 15–25 °C ambient warehouse (10 K band) but is **hard to defend for a 2–8 °C fridge** under the ISPE ratio argument, and useless at −80 °C without a suitable probe (most LoRaWAN internal sensors do not go below −40 °C). Cold-chain deployments need external Pt100/Pt1000 or thermistor probes calibrated at the operating point.
- The ISPE guard-band formula gives a quotable justification for why cheap sensors cost more in the long run: with ±1 °C uncertainty and k=2, GB = 2 °C, so a 2–8 °C chamber must actually be held within 4–6 °C to demonstrate compliance — an impossible operating window. This is a strong sales argument for better probes.
- WHO's ±0.5 °C is a **mapping-logger** specification. There is no separate published number for permanent monitors in WHO's supplements; the practical industry convention is that permanent monitors should be at least as good as mapping loggers for the same chamber.

### Gaps
- I found no EU guidance stating an explicit accuracy figure for permanent GDP monitoring sensors (EU GDP is silent on numbers). The ±0.5 °C figure comes from WHO (mapping) and CDC (US vaccine monitoring), and should be cited as convention, not EU requirement.
- ISPE's Usr (uncertainty of the site reference) is defined in the paywalled guide; the public white paper gives the formula but not worked values.

---

## Q7. Storage condition definitions (Ph. Eur. / EMA) and per-equipment requirements

### Takeaway
The EMA *Guideline on declaration of storage conditions* (CPMP/QWP/609/96 Rev. 2) links each label statement to the stability data that support it: **25 °C/60%RH long term with satisfactory accelerated data → no statement needed; intermediate data only → "Do not store above 30 °C"; long-term only → "Do not store above 25 °C"; 5 °C ± 3 °C → "Store in a refrigerator" plus the range (2 °C to 8 °C) in SmPC/PL; below zero → "Store in a freezer".** I could **not** confirm "Store below −15 °C" in the EMA core table.

### Cited Findings
- EMA table (as fetched): products stable at **25 °C/60%RH long term with 40 °C/75%RH accelerated** → required statement "None"; additional statement "Do not refrigerate or freeze" — [EMA, Guideline on declaration of storage conditions, CPMP/QWP/609/96 Rev 2 (19 November 2007)](https://www.ema.europa.eu/system/files/documents/scientific-guideline/wc500003468_en.pdf)
- Products stable at 25 °C/60%RH long term **with intermediate conditions** → "Do not store above 30 °C or Store below 30 °C"; additional "Do not refrigerate or freeze" — [EMA CPMP/QWP/609/96 Rev 2](https://www.ema.europa.eu/system/files/documents/scientific-guideline/wc500003468_en.pdf)
- Products stable at 25 °C/60%RH **only** → "Do not store above 25 °C or Store below 25 °C"; additional "Do not refrigerate or freeze" — [EMA CPMP/QWP/609/96 Rev 2](https://www.ema.europa.eu/system/files/documents/scientific-guideline/wc500003468_en.pdf)
- Refrigerated: testing condition **"5 °C ± 3 °C (long term)"** → required statement "Store in a refrigerator or Store and transport refrigerated"; additional statement "Do not freeze"; and "The SPC and Package Leaflet (PL) should include a reference to the temperature range e.g. (2 °C to 8 °C)" — [EMA, Guideline on declaration of storage conditions (current version)](https://www.ema.europa.eu/en/documents/scientific-guideline/guideline-declaration-storage-conditions_en.pdf); [EMA CPMP/QWP/609/96 Rev 2](https://www.ema.europa.eu/system/files/documents/scientific-guideline/wc500003468_en.pdf)
- Frozen: testing condition "Below zero" → required statement "Store in a freezer or Store and transport frozen" — [EMA CPMP/QWP/609/96 Rev 2](https://www.ema.europa.eu/system/files/documents/scientific-guideline/wc500003468_en.pdf)
- Where no special conditions apply the statement is: "This medicinal product does not require any special storage conditions." — [EMA CPMP/QWP/609/96 Rev 2](https://www.ema.europa.eu/system/files/documents/scientific-guideline/wc500003468_en.pdf)
- Equipment-specific practical requirements confirmed by WHO: for **small equipment (fridges, freezers, cabinets)** map top/bottom and rear/front; for **freezer rooms and cold rooms** the OQ mapping is ≥24 h and PQ repeats it fully loaded — [WHO TRS 961 Annex 9 Supplement 7, §2.7, §2.5.6, §2.6.2](https://cdn.who.int/media/docs/default-source/medicines/norms-and-standards/guidelines/distribution/trs961-annex9-supp7.pdf?sfvrsn=3c9a94ed_2)

### Inferences
- The operational bands the LoRaWAN business must serve map onto the EMA statements as: ambient/CRT store **15–25 °C** (serving "Do not store above 25 °C"), fridge **+2 to +8 °C** ("Store in a refrigerator"), freezer **−25 to −15 °C** (typical for "Store in a freezer"), ULT **−80 °C ± 10 K** (product-specific, typically defined in the SmPC rather than by the generic guideline, e.g. mRNA vaccines).
- Because EMA's refrigerated testing condition is "5 °C ± 3 °C", the 2–8 °C band is an absolute tolerance, not a target with margin — which is why fridge monitoring needs tight accuracy and why alarm limits are usually set inside the band (see Q10).

### Gaps
- **"Store below −15 °C" could not be confirmed in the EMA guideline's core table.** Both the current and the Rev 2 versions I fetched show "Store in a freezer" / "Below zero" without a −15 °C threshold. The −15 °C figure is common in USP and in national practice; it should not be attributed to the EMA guideline without verifying the Annex. This is worth a direct check of the EMA Annex document before publishing any client-facing material.
- The Ph. Eur. general-chapter definitions of "cold place", "refrigerator", "deep-freeze" were not retrieved in this pass; the EMA guideline is the citable EU source for label statements.
- ICH Q1A(R2) itself was not fetched directly (see Q8 for the MKT link).

---

## Q8. Mean Kinetic Temperature (MKT)

### Takeaway
MKT is the Haynes/Arrhenius-weighted single equivalent temperature, conventionally calculated with **ΔH (activation energy) = 83.144 kJ/mol** and **R = 0.0083144 kJ·mol⁻¹·K⁻¹**, and its regulatory definition comes from ICH Q1A(R2). It is always **higher than the arithmetic mean** and is a long-run stability metric — it does not excuse a short acute excursion.

### Cited Findings
- Formula basis: MKT follows "the formula of J. D. Haynes (J. Pharm. Sci., 60:927-929, 1971)", with TK = mean kinetic temperature in kelvin, ΔH = activation energy, R = gas constant, T₁…T_N = temperature at each sample point in kelvin, t₁…t_n = time intervals — [Rotronic RMS, Mean Kinetic Temperature](https://service.rotronic.com/manual/mean-kinetic-temperature.html)
- Conventional constants: "LogTag® Analyzer uses an activation energy value of 83.144 KJ/mol for the calculation"; gas constant "0.0083144 KJ/mol K" — [LogTag Analyzer Help, Mean Kinetic Temperature](https://help.logtagrecorders.com/LTA3/Topics/UserGuide/Chapter2-Using_Analyzer/Mean_Kinetic_Temperature.htm)
- Regulatory definition (quoted by vendors from ICH): MKT is "a single derived temperature that, if maintained over a defined period of time, affords the same thermal challenge to a drug substance or drug product as would be experienced over a range of both higher and lower temperatures" — attributed to **ICH Topic Q1A(R2)** — [Rotronic RMS](https://service.rotronic.com/manual/mean-kinetic-temperature.html)
- MKT "exceeds arithmetic mean temperature by accounting for the Arrhenius equation" — [Rotronic RMS](https://service.rotronic.com/manual/mean-kinetic-temperature.html)
- Custom ΔH values are configurable in analysis software, and the software flags when a non-default ΔH was used — [LogTag Analyzer Help](https://help.logtagrecorders.com/LTA3/Topics/UserGuide/Chapter2-Using_Analyzer/Mean_Kinetic_Temperature.htm)

### Written-out formula (standard, matching the constants above)
MKT (in kelvin) =
`ΔH / R` divided by `−ln[ ( Σᵢ tᵢ · e^(−ΔH/(R·Tᵢ)) ) / ( Σᵢ tᵢ ) ]`

with equal time intervals this reduces to
`T_K = (ΔH/R) / −ln( (Σᵢ e^(−ΔH/(R·Tᵢ))) / n )`

where ΔH = 83.144 kJ/mol, R = 0.0083144 kJ·mol⁻¹·K⁻¹ (so ΔH/R = 10 000 K exactly), Tᵢ in kelvin, n = number of readings. Convert to °C by subtracting 273.15.

### Inferences
- The choice of 83.144 kJ/mol is not arbitrary in practice: it makes **ΔH/R = 10 000 K**, which is why this value became the de-facto industry default in logger software. That is a useful detail for a client-facing explainer.
- Practical division of labour in deviation assessment: **MKT answers "was the cumulative thermal load over the storage period acceptable?"** (e.g. a warehouse held at 15–25 °C over a year, or an ambient store that drifted above 25 °C on hot afternoons). **MKT does not answer "did the product freeze?" or "did the fridge hit 15 °C for 4 hours?"** — acute single-point excursions against an absolute limit (2–8 °C, "do not freeze") must be assessed against min/max and product-specific excursion data, not averaged away.
- For a monitoring product, MKT is best positioned as a **reporting feature for ambient/CRT warehouses**, plus min/max/duration-above-limit for cold chain.

### Gaps
- USP General Chapter <1079.2> "Mean Kinetic Temperature in the Evaluation of Temperature Excursions During Storage and Transportation" is the most explicit published text on **when MKT may and may not be used**; the USP-hosted PDF returned HTTP 403 and I could not quote it. This is the single most valuable missing citation for the MKT section and should be obtained (it is also available via the APEC toolkit).
- I did not directly fetch ICH Q1A(R2); the MKT definition above is quoted second-hand from vendor documentation that attributes it to Q1A(R2). Verify against the original before quoting in client material.
- No source found that states a regulatory rule "use MKT instead of simple average in deviation assessment" — the choice appears to be a matter of documented company policy plus product stability data.

---

## Q9. Door opening events

### Takeaway
Door openings are expected, not deviations — WHO makes this explicit with a **30-minute recovery acceptance criterion**: after the door closes, all loggers inside must be back within the specified range (e.g. 2.0–8.0 °C) within 30 minutes. The study should use door-open durations representative of real operations.

### Cited Findings
- WHO mapping criterion: "temperature should be maintained within the defined temperature limits except for a maximum of 30 minutes following the door opening" — [WHO TRS 961 Annex 9 Supplement 8, §2.2.6 Step 4](https://cdn.who.int/media/docs/default-source/medicines/norms-and-standards/guidelines/distribution/trs961-annex9-supp8.pdf)
- WHO qualification acceptance criterion for the door-open test: "the temperature recorded by all the EDLMs located inside the storage area should return to being within the specified temperature range (e.g. 2.0 °C to 8.0 °C) within 30 minutes after the door(s) are closed" — [WHO TRS 961 Annex 9 Supplement 7, §2.6.3 Table 10](https://cdn.who.int/media/docs/default-source/medicines/norms-and-standards/guidelines/distribution/trs961-annex9-supp7.pdf?sfvrsn=3c9a94ed_2)
- Test design: "The door-open period used for the test should represent actual door opening behaviour observed during routine operations" — [WHO TRS 961 Annex 9 Supplement 7, §2.6.3](https://cdn.who.int/media/docs/default-source/medicines/norms-and-standards/guidelines/distribution/trs961-annex9-supp7.pdf?sfvrsn=3c9a94ed_2)
- ISPE: door testing should include "doors held open for the maximum specified time in extreme environmental conditions" — [ISPE white paper](https://ispe.org/pharmaceutical-engineering/white-papers/controlled-temperature-chamber-mapping)
- ISPE mitigation options for door-driven nuisance alarms: introduce "time delays into the alarm system" or place the "sensor into a container of thermal fluid" — [ISPE white paper](https://ispe.org/pharmaceutical-engineering/white-papers/controlled-temperature-chamber-mapping)
- Practical mitigations listed by Vaisala: minimise door-open duration and purpose; place location maps on chamber doors to reduce search time; alarm delays; adjusted setpoints for short-term fluctuations; staff training on door-opening impacts — [Vaisala, "Managing GxP Monitoring System Alarms"](https://www.vaisala.com/en/blog/2023-08/managing-gxp-monitoring-system-alarms)
- Door-left-open alerts: "Audible alerts may be set if a refrigerator or walk-in cold store door has been left open for a set period" — [NHS Specialist Pharmacy Service, "Establishing alerts for temperature monitoring systems"](https://sps.nhs.uk/articles/establishing-alerts-for-temperature-monitoring-systems/)
- Freezer door-open expectation (vendor): "temperature rise ≤10 °C during brief access (30–60 seconds) with recovery to setpoint within 15–30 minutes" — [Eupry](https://eupry.com/iq-oq-pq/freezers/)

### Inferences
- Good practice is **annotate, do not delete**: the 30-minute WHO window gives an objective, citable rule for classifying a door-related rise as an expected event rather than a deviation, provided the event is logged (door sensor or operator log) and recovery within 30 min is demonstrated. Deleting data points would be a data-integrity problem.
- A door contact sensor alongside the temperature sensor is therefore not a gadget but the evidence that makes the 30-minute rule auditable. For a LoRaWAN installer this is a straightforward upsell with a regulatory rationale.

### Gaps
- I found no guidance that permits **excluding** door-opening data from MKT or from excursion statistics. The safe reading is that exclusion is not sanctioned; annotation with justification is.

---

## Q10. Alarm limits and alarm delays

### Takeaway
No regulator publishes numeric alarm delays. The defensible approach published by both NHS SPS and Vaisala is: **set alarm thresholds inside the specification limit (e.g. 7.5 °C for an 8 °C limit) and start with no delay, escalating to 2 minutes then 5 minutes only if nuisance alarms occur** — with each choice documented.

### Cited Findings
- Alarm threshold inside the limit: "For example, a refrigerator should not exceed a temperature of 8 °C, setting the alarm threshold at 7.5 °C allows a user to respond before an excursion happens." — [NHS Specialist Pharmacy Service](https://sps.nhs.uk/articles/establishing-alerts-for-temperature-monitoring-systems/)
- Purpose of a delay: "A delay may be set on the alarm to allow for acceptable small excursions without an alert, e.g. the transient rise in temperature associated with a fridge door opening." — [NHS Specialist Pharmacy Service](https://sps.nhs.uk/articles/establishing-alerts-for-temperature-monitoring-systems/)
- Alarm re-annunciation: "if there is a function to silence an alarm (e.g. while stocking a fridge) the alarm should sound again after a defined period (e.g. 10 mins)." — [NHS Specialist Pharmacy Service](https://sps.nhs.uk/articles/establishing-alerts-for-temperature-monitoring-systems/)
- Vaisala's escalation method: "start with no delays and see if you get nuisance alarms. If you're getting nuisance alarms, move to a two-minute delay. If you're still getting nuisance alarms, move out to a five-minute delay." Vaisala is explicit that **there is no standard alarm delay time** — it depends on product characteristics and risk tolerance — [Vaisala, Managing GxP Monitoring System Alarms](https://www.vaisala.com/en/blog/2023-08/managing-gxp-monitoring-system-alarms)
- The reason delays matter: "if alarms keep going off and they're not important, people begin to ignore them." Vaisala also recommends **consistent alarm delays and buffers across all chambers** to reduce logistical complexity — [Vaisala](https://www.vaisala.com/en/blog/2023-08/managing-gxp-monitoring-system-alarms)
- Alarm testing is a qualification requirement: "For each test, record the alarm settings and trigger the desired alarm event. Confirm that the alarm system is activated"; both **High alarm setting** and **Low alarm setting** must be documented — [WHO TRS 961 Annex 9 Supplement 7, §2.5.5 Table 7](https://cdn.who.int/media/docs/default-source/medicines/norms-and-standards/guidelines/distribution/trs961-annex9-supp7.pdf?sfvrsn=3c9a94ed_2)
- Monitoring devices must have an "Alarm for out-of-range temperatures" — [CDC](https://www.cdc.gov/vaccines/hcp/storage-handling/monitoring-equipment.html)

### Inferences
- The justification chain a sole trader should write into the monitoring design document is: thermal mass of the load (measured during mapping) → observed recovery time after representative door openings (measured during the door-open test) → alarm delay chosen so that it is **shorter than the time for product temperature to reach the specification limit**, but longer than typical door transients. This turns the mapping report into the direct input for alarm configuration — a second commercial justification for buying the mapping.
- Because WHO caps recovery at 30 minutes, an alarm delay should be well under 30 minutes; a 2–5 minute air-sensor delay (or 15–30 min with a glycol-buffered probe, which has its own thermal lag) is the common range implied by these sources.

### Gaps
- No citable source gives numeric alarm delays by equipment class (e.g. "fridge 15 min, freezer 30 min, ambient 60 min"). Any such table in a client proposal must be presented as the provider's own risk-based convention, not as guidance.

---

## Q11. Glycol-buffered probes vs air sensors

### Takeaway
Buffering exists to make the sensor read something close to **product temperature** rather than air temperature, which suppresses door-opening and defrost transients. The clearest published requirement is CDC's (buffered probe mandatory for vaccine storage); **ISPE mentions the thermal-fluid container as an alternative to alarm delays**. I found no EU regulatory text mandating buffered vs air sensors — the choice is a documented risk decision.

### Cited Findings
- CDC device requirement: "Detachable probe that best reflects vaccine temperatures (e.g., a probe buffered with glycol, glass beads, sand, or Teflon®)" — [CDC, Temperature Monitoring Equipment](https://www.cdc.gov/vaccines/hcp/storage-handling/monitoring-equipment.html)
- CDC's definition of buffering: immersion "in a vial filled with liquid (e.g., glycol, ethanol, glycerin), loose media (e.g., sand, glass beads), or a solid block of material" — [CDC](https://www.cdc.gov/vaccines/hcp/storage-handling/monitoring-equipment.html)
- ISPE: to prevent nuisance alarms from transient door-opening events, consider "time delays into the alarm system" **or** placing the "sensor into a container of thermal fluid" — [ISPE white paper](https://ispe.org/pharmaceutical-engineering/white-papers/controlled-temperature-chamber-mapping)
- CDC probe siting for buffered probes: "in the center of the unit with the vaccines surrounding it"; not near walls, floor, vent, ceiling or door — [CDC](https://www.cdc.gov/vaccines/hcp/storage-handling/monitoring-equipment.html)
- Logging interval for buffered monitoring devices: "at least every 30 minutes" — [CDC](https://www.cdc.gov/vaccines/hcp/storage-handling/monitoring-equipment.html)

### Inferences
- The decision rule that follows from the sources: use **air sensors for mapping** (you want to see the transients and the true gradients — buffering would hide exactly the hot/cold spot behaviour you are paying to find), and consider **buffered probes for permanent monitoring of fridges and small chambers** where product temperature is the controlled attribute and where unbuffered air sensors generate nuisance alarms. For large ambient warehouses buffering is generally unnecessary because the thermal time constant of the room is already long.
- A buffered probe is functionally equivalent to a long alarm delay, so applying both simultaneously risks masking a real excursion; the monitoring design should choose one and document the reason.

### Gaps
- **No published guidance specifies a glycol bottle volume.** Common practice cited by vendors is 10–30 mL, but I found no citable figure in a regulatory or standards source. The Dickson and DataLoggerInc comparison articles found in search (dicksondata.com, dataloggerinc.com) discuss buffer effectiveness but were not fetched; they may contain measured time constants.
- No EU/EMA/MHRA/WHO text was found that mandates buffered probes or forbids air sensors. This should be stated plainly rather than implied.
- The NIST document "Guidelines for Storage and Temperature Monitoring of Refrigerated Vaccines" (nist.gov) appeared in search and is likely the best source of quantitative buffer time-constant data; not fetched in this pass.

---

## Q12. Power failure studies, holdover time, and independent backup loggers

### Takeaway
WHO requires a power-failure test as part of qualification, with the objective of establishing **holdover time**; two methods are sanctioned (run until out of range, or a predefined period such as two hours). For backup, the clearest published recommendation is CDC's: keep at least one spare logger for every unit.

### Cited Findings
- Objective: establish "The length of time during which the installation can maintain the specified temperature range following a power failure – this is known as the holdover time" — [WHO TRS 961 Annex 9 Supplement 7, §2.5.7](https://cdn.who.int/media/docs/default-source/medicines/norms-and-standards/guidelines/distribution/trs961-annex9-supp7.pdf?sfvrsn=3c9a94ed_2)
- Method 1 (run to excursion): "When the temperature inside the storage area goes out of range, then the storage area and equipment can be powered back on" — [WHO TRS 961 Annex 9 Supplement 7, §2.5.7](https://cdn.who.int/media/docs/default-source/medicines/norms-and-standards/guidelines/distribution/trs961-annex9-supp7.pdf?sfvrsn=3c9a94ed_2)
- Method 2 (fixed period): "the power is stopped for a predefined period – for example, two hours" — [WHO TRS 961 Annex 9 Supplement 7, §2.5.7](https://cdn.who.int/media/docs/default-source/medicines/norms-and-standards/guidelines/distribution/trs961-annex9-supp7.pdf?sfvrsn=3c9a94ed_2)
- Backup device: facilities should "Have at least one backup DDL or TMD in case a primary device breaks, malfunctions, or requires calibration." — [CDC, Temperature Monitoring Equipment](https://www.cdc.gov/vaccines/hcp/storage-handling/monitoring-equipment.html)
- ULT holdover expectation (vendor): "Typical target is 4–8 hours for ULT freezers to allow emergency response time" — [Eupry](https://eupry.com/iq-oq-pq/freezers/)
- Defrost cycles must be captured within the mapping period — [ISPE white paper](https://ispe.org/pharmaceutical-engineering/white-papers/controlled-temperature-chamber-mapping); [WHO TRS 961 Annex 9 Supplement 8](https://cdn.who.int/media/docs/default-source/medicines/norms-and-standards/guidelines/distribution/trs961-annex9-supp8.pdf)

### Inferences
- Holdover time is the number that determines the **alarm escalation SLA**: if a cold room holds 2–8 °C for 3 hours without power, the out-of-hours callout procedure must guarantee a response in well under 3 hours. A LoRaWAN system with battery-powered sensors and a UPS-backed gateway has a genuine advantage here over mains-powered wired systems, and the mapping/qualification report is the document that quantifies it.
- An independent battery-powered logger in each critical space is the practical answer to the failure mode that a networked system cannot cover: gateway or network outage. CDC's "one backup per unit" is the only citable anchor I found; for GDP purposes the argument is data-integrity continuity rather than a stated requirement.

### Gaps
- I found **no EU/WHO text requiring independent backup data loggers** in critical spaces. The practice is widespread and CDC recommends it, but it should be presented as risk-based good practice, not a requirement.
- WHO does not state a minimum acceptable holdover time; it only requires that it be measured and documented.

---

## Cross-cutting: regulatory chain of reasoning for the EU (quotable spine)

### Cited Findings
- EU GDP §3.2.1 (the single most important sentence for selling mapping in the EU): "An initial temperature mapping exercise should be carried out on the storage area before use, under representative conditions." — [EU Guidelines on Good Distribution Practice of medicinal products for human use (2013/C 343/01)](https://eur-lex.europa.eu/LexUriServ/LexUriServ.do?uri=OJ:C:2013:343:0001:0014:EN:PDF)
- EU GDP §3.2.1 (the sentence that links mapping to sensor sales): "Temperature monitoring equipment should be located according to the results of the mapping exercise, ensuring that monitoring devices are positioned in the areas that experience the extremes of fluctuations." — [EU GDP 2013/C 343/01](https://eur-lex.europa.eu/LexUriServ/LexUriServ.do?uri=OJ:C:2013:343:0001:0014:EN:PDF)
- EU GDP §3.2.1 (the re-mapping trigger): "The mapping exercise should be repeated according to the results of a risk assessment exercise or whenever significant modifications are made to the facility or the temperature controlling equipment." — [EU GDP 2013/C 343/01](https://eur-lex.europa.eu/LexUriServ/LexUriServ.do?uri=OJ:C:2013:343:0001:0014:EN:PDF)
- EU GDP §3.2.1 (small premises carve-out): "For small premises of a few square meters which are at room temperature, an assessment of potential risks (e.g. heaters) should be conducted and temperature monitors placed accordingly." — [EU GDP 2013/C 343/01](https://eur-lex.europa.eu/LexUriServ/LexUriServ.do?uri=OJ:C:2013:343:0001:0014:EN:PDF)
- EU GDP §3.3 (equipment): "All equipment impacting on storage and distribution of medicinal products should be designed, located and maintained to a standard which suits its intended purpose." — [EU GDP 2013/C 343/01, via Basicmedical Key transcription](https://basicmedicalkey.com/guidelines-on-good-distribution-practice-of-medicinal-products-for-human-use-2013c-34301/)
- EU GDP §3.2.1 (environment checks): "Suitable equipment and procedures should be in place to check the environment where medicinal products are stored." — [EU GDP 2013/C 343/01, via Basicmedical Key transcription](https://basicmedicalkey.com/guidelines-on-good-distribution-practice-of-medicinal-products-for-human-use-2013c-34301/)

### Inferences
- The commercial logic writes itself from EU GDP §3.2.1 alone: mapping is required before use; sensor positions must follow from mapping; mapping must be repeated on change. A sole trader who sells sensors without the mapping is selling an installation the customer cannot justify to an inspector; selling the mapping first makes the sensor count and positions an audited output rather than a quotation line.

### Gaps
- The EU GDP text above was obtained via a WebFetch summary of the EUR-Lex PDF and a partial third-party transcription; section numbering beyond 3.2.1 and 3.3 could not be fully verified in this pass (the fetch noted that "3.3.1" in the official text is *Computerised systems*, not temperature monitoring, and that §3.2.2 does not exist). **Verify exact section numbering against the EUR-Lex original before quoting in a client document.**
- The WHO supplement numbered 13 that I fetched turned out to cover **shipping container qualification**, not fixed-storage monitoring; it specifies EDLM accuracy "±0.5 °C, over a temperature range approximately between −20 °C and +50 °C" (§2.4 f) and "maximum temperature-recording interval … no greater than 30 minutes (5 or 10 minutes is better)" (§2.4 g), which are transport figures. The fixed-storage monitoring supplement (sometimes cited as supplement 15, "Temperature and humidity monitoring systems for fixed storage areas") was not located at the URL pattern tried and remains an open gap — [WHO supplement 13 PDF](https://cdn.who.int/media/docs/default-source/medicines/norms-and-standards/guidelines/distribution/trs961-annex9-supp13.pdf)
- PDA Technical Report 39: the 2021 revision is titled "Guidance for Temperature-Controlled Medicinal Products — Maintaining the Quality of Temperature-Sensitive Medicinal Products through the **Transportation Environment**", i.e. it is primarily a transport document, not a storage-mapping document. Contents page: [PDA TR 39 (2021) TOC](https://store.pda.org/TableOfContents/2021TR3921_TOC.pdf). Full text is paywalled and could not be quoted.
