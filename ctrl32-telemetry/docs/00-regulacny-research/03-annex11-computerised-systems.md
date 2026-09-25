# EU GMP Annex 11 (Computerised Systems) — 2011 version in force and July 2025 draft revision, applied to a temperature monitoring product

> **Version labelling convention used throughout this file**
> - `[A11-2011]` = EudraLex Volume 4, Annex 11 "Computerised Systems", revision January 2011, **operational since 30 June 2011 — this is the version legally in force**.
> - `[A11-DRAFT-2025]` = draft revised Annex 11 published for public consultation on 7 July 2025, consultation closed 7 October 2025. **Not in force. Text may change before adoption.**
> - `[CH4-DRAFT-2025]` = draft revised EudraLex Vol. 4 Chapter 4 "Documentation", same consultation.
>
> **Verbatim caveat (important for the report writer):** the official PDFs were retrieved through a fetch layer that enforces a per-quote length cap. Quotes below that are reproduced as full sentences are marked plainly; quotes where the retrieval tool truncated mid-sentence are shown with an explicit `[…]` and flagged. Every `[…]` marks text I could **not** verify, not text I chose to omit. Before the user relies on any clause in a regulatory submission or a validation document, he should re-download the source PDFs (URLs given in each section) and re-verify wording character-for-character. Do not present truncated clauses as complete citations.

---

## Q1. Status check — has the final revised Annex 11 been published as of September 2026?

### Takeaway
**No. As of 21 September 2026 the revised Annex 11 has NOT been published.** The official EudraLex Volume 4 page still lists Annex 11 in its **January 2011 revision** as the current version, and there is **no Annex 22 entry at all** on the page. The 2011 text therefore remains the only legally applicable version, and the July 2025 draft has no regulatory force whatsoever.

### Cited Findings
- The official European Commission EudraLex Volume 4 page lists **"Annex 11 | Computerised Systems (revision January 2011)"** as the current version; **no 2026 revised version is listed** — [EudraLex Volume 4, health.ec.europa.eu](https://health.ec.europa.eu/medicinal-products/eudralex/eudralex-volume-4_en) (page checked 21 Sept 2026)
- The same official page lists **"Chapter 4 - Documentation (January 2011)"** — the revised Chapter 4 is also **not** published — [EudraLex Volume 4](https://health.ec.europa.eu/medicinal-products/eudralex/eudralex-volume-4_en)
- **No Annex 22 entry appears** on the official EudraLex Volume 4 page; the annex listings go from Annex 21 straight to Part IV (ATMPs) — [EudraLex Volume 4](https://health.ec.europa.eu/medicinal-products/eudralex/eudralex-volume-4_en)
- For contrast, the most recently updated document on the same page is **Annex 19 (Reference and Retention Samples), "applicable as of 24 September 2026"** — showing the page is actively maintained and current, which strengthens the conclusion that Annex 11's absence of a new version is real and not a stale page — [EudraLex Volume 4](https://health.ec.europa.eu/medicinal-products/eudralex/eudralex-volume-4_en)
- The consultation itself: **opening date 7 July 2025, closing date 7 October 2025 23:59 CEST, status "Closed"**, with the notice *"The response period for this consultation has ended. Thank you for your input."* The consultation page gives **no publication date for the final versions and no operational date** — [Stakeholders' Consultation on EudraLex Volume 4, health.ec.europa.eu](https://health.ec.europa.eu/consultations/stakeholders-consultation-eudralex-volume-4-good-manufacturing-practice-guidelines-chapter-4-annex_en)
- The original concept-paper timeline was: draft release December 2024; comment deadline March 2025; **EMA adoption March 2026; European Commission publication June 2026; PIC/S adoption September 2026** — [qmsdoc.com, Revision of PIC/S GMP Annex 11](https://qmsdoc.com/2026/01/15/revision-of-pic-s-gmp-annex-11-computerised-systems/)
- Actual progress has slipped against that plan: draft released 7 July 2025, consultation closed 7 October 2025, **"Mid-2026: Expected final publication"**, **"2027: Anticipated implementation date following a transition period"**, with the note that *"the revision process experienced some delays from the original timeline"* — [qmsdoc.com](https://qmsdoc.com/2026/01/15/revision-of-pic-s-gmp-annex-11-computerised-systems/)
- A transition period of **approximately 12–18 months after publication** is anticipated before mandatory compliance — [qmsdoc.com](https://qmsdoc.com/2026/01/15/revision-of-pic-s-gmp-annex-11-computerised-systems/)
- Industry commentary written before/around the deadline expected **"formal adoption expected in 2026"** for all three documents (Chapter 4, Annex 11, Annex 22), and advised acting during *"EMA's typical 6-12 month grace period after publication"* — [Epista](https://www.epista.com/knowledge/preparing-for-emas-2026-good-manufacturing-practice-gmp-revisions-chapter-4-annex-11-annex-22)
- The revision is a **joint EMA–PIC/S effort**: *"a joint EMA-PIC/S drafting group to maintain global harmonization of standards"* — meaning the PIC/S PE 009 Annex 11 will track the EU text — [qmsdoc.com](https://qmsdoc.com/2026/01/15/revision-of-pic-s-gmp-annex-11-computerised-systems/)

### Inferences
- The **June 2026 target for Commission publication has been missed**. As of 21 September 2026 the document is roughly 3+ months past its own revised target and ~6 months past the original EMA-adoption milestone. The PIC/S September 2026 adoption milestone is also evidently not met, since PIC/S adoption follows EU publication.
- Practical consequence for telemetry.digital: **build and validate against the 2011 Annex 11 as the binding standard, but design against the 2025 draft.** The draft is the clearest available statement of what EU GMP inspectors already expect in practice (much of it restates existing PIC/S PI 041-1 data-integrity guidance), and a system built to the draft will comply with the 2011 text by construction. The reverse is not true — a system built only to the 2011 minimum will need rework.
- Because a 12–18 month transition is anticipated **after** publication, and publication had not occurred by late Sept 2026, the realistic mandatory-compliance date is **2028 at the earliest**. There is time, but the commercial argument (pharma warehouse customers will start asking supplier-qualification questions framed in draft-Annex-11 language well before it is binding) means draft-readiness is a sales asset now.
- Caution for the user: several vendor blogs are titled "Annex 11: The 2026 Revision" or "Annex 11:2026" as though a 2026 version exists. **It does not.** These are marketing pieces about the draft. He should not cite them as evidence of a published standard.

### Gaps
- I could not find any official EMA, European Commission or PIC/S announcement dated 2026 that gives a **revised** publication date after the missed mid-2026 target. There appears to be no public statement explaining the delay. The absence of the document from the official page is the strongest evidence available; I found no source announcing a new target date.
- I could not access picscheme.org directly (the outbound proxy rejected the host), so I could not confirm the current PIC/S PE 009 revision number or whether PIC/S has issued its own status note on Annex 11.
- The number of pages in the draft (the "5 to ~19 pages" claim in the assignment) was not confirmed by any source I reached; ECA's analysis does not state a page count. Treat the page-count figure as unverified.

---

## Q2. VERBATIM clauses of the CURRENT Annex 11 (2011) and what each means for a temperature monitoring system

### Takeaway
The 2011 Annex 11 is a short, principles-based text of 17 numbered clauses plus a Principle and Glossary. For a temperature monitoring system the load-bearing clauses are **4 (validation), 5 (data exchange checks), 6 (accuracy checks), 7 (data storage/backup), 9 (audit trail), 11 (periodic evaluation), 12 (security/access), 16 (business continuity) and 17 (archiving)** — plus **clause 3, which is the clause the user's customers will apply to him as the supplier.**

**Source of all quotes in this section:** `[A11-2011]` — [EudraLex Volume 4, Annex 11 Computerised Systems, revision January 2011 (PDF)](https://health.ec.europa.eu/system/files/2016-11/annex11_01-2011_en_0.pdf), linked from [EudraLex Volume 4](https://health.ec.europa.eu/medicinal-products/eudralex/eudralex-volume-4_en). Operational since 30 June 2011.

### Cited Findings — clause by clause

#### Principle `[A11-2011]` — quoted in full
> "This annex applies to all forms of computerised systems used as part of a GMP regulated activities. A computerised system is a set of software and hardware components which together fulfill certain functionalities. The application should be validated; IT infrastructure should be qualified. Where a computerised system replaces a manual operation, there should be no resultant decrease in product quality, process control or quality assurance. There should be no increase in the overall risk of the process."

**Meaning for telemetry.digital:** the split of **"application validated / IT infrastructure qualified"** is the fundamental architecture of the whole annex. The Go application + PostgreSQL schema + alarm logic = the *application*, which is **validated**. The servers, OS, network, hosting/cloud, sensors and gateways = *IT infrastructure*, which is **qualified** (IQ/OQ). These are two separate documentation streams. If the product replaces paper chart recorders or manual twice-daily thermometer readings in a warehouse, he must be able to argue there is **no increase in overall process risk** — the obvious risk he introduces is a silent failure mode (sensor stops reporting and nobody notices), which is precisely why draft clause 8 on alarms and the 2011 clause 16 on business continuity matter.

#### 1. Risk Management `[A11-2011]`
> "Risk management should be applied throughout the lifecycle of the computerised system taking into account patient safety, data integrity and product quality."

*(Note: the 2011 clause 1 continues with a second sentence about decisions on validation extent and data integrity controls being based on a justified and documented risk assessment — I could not retrieve that sentence verbatim within the fetch constraint. Flagged as `[…]`.)*

**Meaning:** every design decision — sampling interval, alarm thresholds, redundancy, retention — needs a documented rationale tied to **patient safety, data integrity, product quality** (the three-part test recurs throughout the annex). For a warehouse monitoring system the patient-safety link is direct: undetected excursion → degraded medicine → patient harm. A one-page risk assessment (e.g. GAMP 5 style, or an FMEA on "sensor fails / gateway offline / DB corrupt / alarm not delivered") is the single most useful document he can write first, because clauses 4, 9, 12 and 16 all say their extent is determined by it.

#### 2. Personnel `[A11-2011]`
> "All personnel should have appropriate qualifications, level of access and defined responsibilities to carry out their assigned duties."

*(The 2011 clause 2 also covers close cooperation between key personnel such as Process Owner, System Owner, Qualified Persons and IT — not retrieved verbatim. Flagged.)*

**Meaning for a one-man company:** this is a real problem for a sole trader and he should address it head-on. Roles of **System Owner, Process Owner, QA** must be *named* in his customers' documentation, and he cannot hold all of them at the customer site. In his own supplier QMS he must show his own competence (CV, training record, evidence of GAMP/GMP knowledge) — this is what a customer audit will ask for first. Segregation of duties is a 2011 expectation and becomes an explicit, hard requirement in the draft (see Q3, draft 11.10).

#### 3. Suppliers and Service Providers `[A11-2011]` — **the clauses that apply to the user himself**
> **3.1** "Formal agreements must exist between the manufacturer and any third parties, and these agreements should include clear statements of the responsibilities of the third party."
>
> **3.2** "The competence and reliability of a supplier are key factors when selecting a product or service provider." *(clause continues — the remainder concerns the need for audit and its justification being based on risk; not retrieved verbatim. Flagged `[…]`.)*
>
> **3.3** "Documentation supplied with commercial off-the-shelf products should be reviewed by regulated users to check that user requirements are fulfilled."
>
> **3.4** "Quality system and audit information relating to suppliers or developers of software and implemented systems should be made available to inspectors on request."

**Meaning — this is the user's key section, see Q6 below for full treatment.**

#### 4. Validation `[A11-2011]`
> **4.1** "Manufacturers should be able to justify their standards, protocols, acceptance criteria, procedures and records based on their risk assessment."
>
> **4.2** "Validation documentation should include change control records (if applicable) and reports on any deviations observed."
>
> **4.3** "An up to date listing of all relevant systems and their GMP functionality (inventory) should be available."
>
> **4.4** "User Requirements Specifications should describe the required functions of the computerised system and be based on documented risk assessment and GMP impact."
>
> **4.5** "The regulated user should take all reasonable steps, to ensure that the system has been developed in accordance with an appropriate quality management system. The supplier should be assessed appropriately." *(quoted in full)*
>
> **4.6** "For the validation of bespoke or customised computerised systems there should be a process in place that ensures the formal assessment and reporting of quality and performance measures."
>
> **4.7** "Evidence of appropriate test methods and test scenarios should be demonstrated. Particularly, system (process) parameter limits, data limits and error handling should be considered."
>
> **4.8** "If data are transferred to another data format or system, validation should include checks that data are not altered in value and/or meaning during this migration process." *(quoted in full)*

**Meaning for telemetry.digital:**
- **4.3** — his customers need a system inventory entry; he should supply a ready-made "system description / GMP functionality" one-pager they can drop in.
- **4.4** — the **URS belongs to the customer**, not to him. But a sole trader selling to small Slovak warehouses will find that customers cannot write one. Best commercial move: ship a **template URS** the customer reviews, edits and approves in their own name. (The draft makes this explicit — draft 9.9 says the regulated user "is fully accountable and should carefully review and authorise the use of the documentation", which is aimed exactly at the practice of vendors supplying validation packs.)
- **4.5** — **"The supplier should be assessed appropriately."** This is the hook for the supplier audit. He is a bespoke/custom developer, so GAMP category 5 logic applies and scrutiny will be highest.
- **4.6** — because telemetry.digital is **bespoke software written by him**, this clause bites hardest: he needs a documented development process with formal assessment and reporting of quality and performance measures. In Go terms: version control with meaningful history, code review (a problem for a sole trader — consider documented self-review checklists or an external reviewer), unit/integration test suites with recorded results, a defined release procedure, and a traceability matrix from URS → design → test.
- **4.7** — test **limits and error handling** explicitly: what happens at the sensor's range boundary, at an implausible value (-300 °C), on a gateway disconnect, on a DB write failure, on a clock jump/DST change, on duplicate timestamps. Error-handling tests are the ones most often missing and most often cited by inspectors.
- **4.8** — applies whenever he migrates a customer from an old logger, changes PostgreSQL major version with a dump/restore, or changes the measurement schema. Requires documented checks that values **and meaning** (units, timezone, sensor identity) survive.

#### 5. Data `[A11-2011]` — quoted in full
> "Computerised systems exchanging data electronically with other systems should include appropriate built-in checks for the correct and secure entry and processing of data, in order to minimize the risks."

**Meaning:** this is **the interface clause and it is central to a telemetry product**, because the whole product is an exchange of data between systems: sensor → gateway → ingest API → PostgreSQL → dashboard/report/export. "Built-in checks" means technical controls in the software, not a procedure: checksums or CRC on the sensor/gateway link, authenticated and TLS-encrypted ingest, sequence numbers / gap detection so a missing interval is *detected and visible* rather than silently absent, rejection or quarantining of malformed payloads, idempotency keys against duplicate insertion, and range/plausibility validation on ingest. If he exports to a customer's WMS/ERP or emails CSV/PDF reports, those transfers need checks too. **A silent data gap is the classic Annex 11 clause 5 failure in temperature monitoring** — the system must prove that absence of data is visible.

#### 6. Accuracy Checks `[A11-2011]` — quoted in full
> "For critical data entered manually, there should be an additional check on the accuracy of the data. This check may be done by a second operator or by validated electronic means. The criticality and the potential consequences of erroneous or incorrectly entered data to a system should be covered by risk management."

**Meaning:** in a mostly-automated telemetry system, the manual entries are the risk surface: **alarm limit setpoints, sensor calibration offsets/coefficients, sensor-to-location mapping, excursion justification comments, calibration certificate values, mapping-study parameters.** Each of these is critical data entered manually. Either build a **four-eyes/second-operator confirmation workflow** in the app for setpoint and calibration changes, or implement "validated electronic means" (e.g. re-entry confirmation plus range validation) — and document in the risk assessment why the chosen method is adequate. Changing an alarm limit from +8 °C to +15 °C is the single highest-consequence manual entry in the whole product and deserves the strongest control (second-person approval + audit trail + alarm-log annotation).

#### 7. Data Storage `[A11-2011]` — quoted in full
> **7.1** "Data should be secured by both physical and electronic means against damage. Stored data should be checked for accessibility, readability and accuracy. Access to data should be ensured throughout the retention period."
>
> **7.2** "Regular back-ups of all relevant data should be done. Integrity and accuracy of backup data and the ability to restore the data should be checked during validation and monitored periodically."

**Meaning:** **7.2 is not satisfied by "we run pg_dump nightly".** Three distinct obligations: (a) backups are *regular* per a documented schedule; (b) **integrity, accuracy and restorability are checked during validation** — i.e. a documented restore test is part of the validation pack, with evidence; and (c) they are **monitored periodically** thereafter — i.e. a recurring, recorded restore test (annual at minimum, and after any change to the backup tooling). **7.1's "throughout the retention period"** is the forward-looking trap: GMP retention for warehouse temperature records runs for years (see Q5 on Chapter 4 retention), so he must be able to demonstrate that data written in 2026 is still accessible, readable and accurate in 2031+ — which constrains format choices, PostgreSQL upgrade paths, and what happens to a customer's data if he ceases trading (see draft 7.5 item viii, exit strategy).

#### 8. Printouts `[A11-2011]` — quoted in full
> **8.1** "It should be possible to obtain clear printed copies of electronically stored data."
>
> **8.2** "For records supporting batch release it should be possible to generate printouts indicating if any of the data has been changed since the original entry."

**Meaning:** he needs a **report/PDF export** that is clear and complete (temperature graph + tabular data + excursion list + sensor identity + period + generation timestamp + who generated it). **8.2** is the important one: if a warehouse temperature record supports QP batch release or GDP certification, the printout must **show whether data has been changed since original entry** — practically, the report must surface amended/annotated records and carry a "modified" indicator, or the report must embed/reference the relevant audit trail entries. A PDF that silently shows only the current values fails 8.2.

#### 9. Audit Trails `[A11-2011]` — quoted in full
> "Consideration should be given, based on a risk assessment, to building into the system the creation of a record of all GMP-relevant changes and deletions (a system generated 'audit trail'). For change or deletion of GMP-relevant data the reason should be documented. Audit trails need to be available and convertible to a generally intelligible form and regularly reviewed."

**Meaning — four separate obligations, and note the weak 2011 wording:** the 2011 text says only *"consideration should be given, based on a risk assessment"* — it is risk-based, not absolute. **In practice this softness is dead**: for GMP-relevant temperature data any risk assessment concludes an audit trail is required, and the draft removes the optionality entirely (draft 12.1 says systems "should have an audit trail functionality"). Concretely:
1. **System-generated** — written by the application, not by the user; an append-only table in PostgreSQL, ideally with DB-level revocation of UPDATE/DELETE on it for the application role.
2. **Changes AND deletions** — deletions must leave a trace.
3. **Reason documented** — the UI must *require* a reason string on any change/deletion of GMP data (including on an excursion annotation or a setpoint change). A nullable reason column is a compliance defect.
4. **Available, convertible to a generally intelligible form, regularly reviewed** — human-readable export (not raw JSON blobs, not opaque IDs: resolve user IDs to names and field names to labels), and **the system must support review** — searchable/filterable, because "regularly reviewed" is impossible without it.

*What to log:* who, what (old value → new value), when (with timezone), why. Raw measurements ingested automatically are generally not "changes" requiring audit-trail entries, but **every** manual interaction with them is: annotations, excursion acknowledgements/justifications, alarm setpoint changes, sensor calibration changes, user/role changes, report generation, configuration changes.

#### 10. Change and Configuration Management `[A11-2011]`
> "Any changes to a computerised system including system configurations should only be made in a controlled manner in accordance with a defined procedure."

**Meaning:** covers both **his** releases and **the customer's** configuration changes. He needs a documented change-control SOP: impact assessment → regression/validation testing proportionate to impact → approval → release notes → customer notification. Note the phrase **"including system configurations"** — changing an alarm limit or adding a sensor in the customer's instance is a configuration change and needs to run through the customer's change control, not just be done ad hoc.

#### 11. Periodic evaluation `[A11-2011]` — quoted in full
> "Computerised systems should be periodically evaluated to confirm that they remain in a valid state and are compliant with GMP. Such evaluations should include, where appropriate, the current range of functionality, deviation records, incidents, problems, upgrade history, performance, reliability, security and validation status reports."

**Meaning:** the customer must run a periodic review (typically annual, risk-based). He should **sell/supply the inputs**: a per-customer annual report bundling upgrade history, incident log, uptime/performance stats, security status (patches applied, vulnerabilities), open deviations, and current validation status. This is a cheap recurring-revenue service and it is exactly what the draft's 12-subsection clause 14 expands into.

#### 12. Security `[A11-2011]` — quoted in full
> **12.1** "Physical and/or logical controls should be in place to restrict access to computerised system to authorised persons. Suitable methods of preventing unauthorised entry to the system may include the use of keys, pass cards, personal codes with passwords, biometrics, restricted access to computer equipment and data storage areas."
>
> **12.2** "The extent of security controls depends on the criticality of the computerised system."
>
> **12.3** "Creation, change, and cancellation of access authorisations should be recorded."
>
> **12.4** "Management systems for data and for documents should be designed to record the identity of operators entering, changing, confirming or deleting data including date and time."

**Meaning:**
- **12.1** — role-based access control; authenticated access to the web UI and to the ingest API; and *physical* protection of the server/DB (relevant if hosted on his own hardware; if cloud, this transfers to the cloud provider and becomes a supplier-management issue under clause 3).
- **12.3** — **user administration itself must be logged**: account creation, role changes, deactivation. This is a separate log obligation from clause 9's data audit trail, and it is commonly missed. A `users` table with a mutable `role` column and no history fails 12.3.
- **12.4 is the clause the assignment singles out on "who can change data" —** note what it actually says: it requires the system to **record the identity** of operators entering, changing, confirming or deleting data, **including date and time**. Combined with 12.1 (restrict access to authorised persons) the practical rule is: **no shared accounts, no anonymous writes, no application-level "system" user performing changes on behalf of a human.** Every mutation is attributable to a named individual with a timestamp. For PostgreSQL this means the app must carry the end-user identity down to the audit record — a single DB connection user is fine, but the *application* must record the human. Also note "confirming" — a second-operator accuracy check under clause 6 is itself an event that must record who confirmed and when.

#### 13. Incident Management `[A11-2011]` — quoted in full
> "All incidents, not only system failures and data errors, should be reported and assessed. The root cause of a critical incident should be identified and should form the basis of corrective and preventive actions."

**Meaning:** he needs an incident log and a CAPA process as the supplier, and his contract/SLA must define **how and how fast he notifies the customer** of an incident — because the customer's own GMP incident management depends on being told. Note the breadth: **"not only system failures and data errors"** — a security incident, a missed alarm notification, a gateway outage, or a near-miss all count. For a monitoring product the defining incident class is **"the system failed to detect or failed to notify an excursion"**, and root-cause analysis on that must be documented.

#### 14. Electronic Signature `[A11-2011]` — quoted in full
> "Electronic records may be signed electronically. Electronic signatures are expected to:
> a. have the same impact as hand-written signatures within the boundaries of the company,
> b. be permanently linked to their respective record,
> c. include the time and date that they were applied."

**Meaning:** **only relevant if his software captures signatures** — e.g. a QA reviewer signing off a monthly temperature report or an excursion investigation. If he implements it: bind the signature cryptographically or referentially to the exact record version (so a later edit breaks or invalidates it), store timestamp with timezone, and display the signer's full name and the meaning of the signature. If he does *not* implement e-signatures, the customer will print and wet-ink sign — a **hybrid** approach, which the 2011 text tolerates but the draft Chapter 4 explicitly discourages (`[CH4-DRAFT-2025]` 4.70: *"If records exist electronically such records should be signed electronically. The use of a hybrid system should be avoided."*). **Strategic note: building e-signature into the product is a differentiator and future-proofs against the draft.**

#### 15. Batch release `[A11-2011]` — quoted in full
> "When a computerised system is used for recording certification and batch release, the system should allow only Qualified Persons to certify the release of the batches and it should clearly identify and record the person releasing or certifying the batches."

**Meaning:** **almost certainly NOT applicable to telemetry.digital.** A warehouse temperature monitoring system records storage conditions; it does not certify or release batches. He should state this explicitly in his validation documentation as a scoping exclusion ("clause 15 not applicable — the system performs no batch certification or release function"). Stating non-applicability with a justification is good practice and saves audit time. *Indirect relevance:* his data may *support* a QP's release decision, which is what triggers clause 8.2's "changed since original entry" printout requirement and the draft's 12.10 (audit trail reviews available to the QP).

#### 16. Business Continuity `[A11-2011]` — quoted in full
> "For the availability of computerised systems supporting critical processes, provisions should be made to ensure continuity of support for those processes in the event of a system breakdown (e.g. a manual or alternative system). The time required to bring the alternative arrangements into use should be based on risk and appropriate for a particular system and the business process it supports. These arrangements should be adequately documented and tested."

**Meaning:** temperature monitoring of a pharmaceutical warehouse **is** a critical process — if monitoring stops, the warehouse is running blind and stored stock's condition becomes unverifiable. Required: a documented fallback (calibrated standalone data loggers, or manual readings at defined intervals with a paper form), a **risk-justified time** to bring it into use, and — the part most often missing — **evidence the fallback has been tested**. Note also a design implication: sensors/gateways should **buffer locally** so a server or network outage does not create a permanent data gap; buffered data backfilled later must be timestamped with the *original* measurement time and clearly identifiable as backfilled.

#### 17. Archiving `[A11-2011]` — quoted in full
> "Data may be archived. This data should be checked for accessibility, readability and integrity. If relevant changes are to be made to the system (e.g. computer equipment or programs), then the ability to retrieve the data should be ensured and tested."

**Meaning:** the 2011 archiving clause is famously thin (three sentences). Key operational trigger: **any relevant system change obliges a tested retrieval check.** So a PostgreSQL major-version upgrade, a server migration, or a schema change each require documented evidence that archived data is still retrievable. The draft expands this into 5 subsections (see Q3) including checksum verification and a requirement that archived data remain searchable/sortable.

### Inferences
- The 2011 annex never once mentions passwords complexity, MFA, encryption, firewalls, patching, anti-virus, penetration testing or segregation of duties. **Everything security-technical in the draft is genuinely new as explicit text**, though inspectors have expected much of it under the general umbrella of 12.1/12.2 and PIC/S PI 041-1.
- Clauses that are *optional/risk-based* in 2011 and become *mandatory* in the draft are the biggest design risk for a product being built now: **audit trail (9 → 12.1)**, and the whole of security (12 → 15). Build them as non-optional.
- Mapping to product architecture: clause 5 ↔ the ingest pipeline; clause 6 ↔ setpoint/calibration entry forms; clause 9 + 12.3/12.4 ↔ two distinct append-only log tables (data audit trail, and access/admin log); clause 7.2 ↔ backup+restore evidence; clause 16 ↔ local buffering + documented manual fallback; clause 8.2 ↔ the PDF report generator.

### Gaps
- Verbatim text of clauses **1 (2nd sentence), 2 (2nd sentence), 3.2 (remainder), 4.1–4.4, 4.6, 4.7, 10** could not be retrieved in full sentences within the fetch tool's per-quote limit; the first sentence of each is quoted above and is accurate, but the clauses are longer than shown. **The user must re-download the 2011 PDF and verify these before citing them.**
- The 2011 Glossary (definitions of e.g. "Application", "Bespoke/Customized computerised system", "Commercial of the shelf software", "IT Infrastructure", "Life cycle", "Process owner", "System owner", "Third Party") was not retrieved verbatim.

---

## Q3. What the JULY 2025 DRAFT REVISION adds — structure and new requirements

### Takeaway
The draft is a near-total rewrite: **17 numbered chapters plus a Glossary**, restructured and dramatically expanded, with entirely new chapters on **System Requirements (6), Supplier and Service Management (7), Alarms (8), Identity and Access Management (11), Periodic Reviews (14), Security (15, ~20 subsections) and Backup (16)**. For a temperature monitoring product the two most consequential new chapters are **8 (Alarms)** — which did not exist at all in 2011 and speaks directly to this product category — and **15 (Security)**.

**Source of all quotes in this section:** `[A11-DRAFT-2025]` — [Draft revised Annex 11: Computerised Systems, consultation version, 7 July 2025 (PDF)](https://health.ec.europa.eu/document/download/40231f18-e564-4043-94de-c031f813d38b_en?filename=mp_vol4_chap4_annex11_consultation_guideline_en.pdf), from the [consultation page](https://health.ec.europa.eu/consultations/stakeholders-consultation-eudralex-volume-4-good-manufacturing-practice-guidelines-chapter-4-annex_en). **NOT IN FORCE.**

### Cited Findings

#### Reasons for changes `[A11-DRAFT-2025]`
> "The GMP/GDP Inspectors Working Group and the PIC/S Committee jointly recommended that the current version of Annex 11 on Computerised Systems, be revised to reflect changes in regulatory and manufacturing environments."

The revision aims to *"clarify requirements and expectations from regulatory authorities, and remove ambiguity and inconsistencies."*

On drivers:
> "With an ever-evolving IT landscape, increased use of cloud services, and introduction of new technologies in computerised systems used in GMP activities, there is a growing need for updated guidance."

#### Full chapter structure `[A11-DRAFT-2025]`
1. Scope
2. Principles
3. Pharmaceutical Quality System
4. Risk Management
5. Personnel and Training
6. System Requirements
7. Supplier and Service Management
8. Alarms
9. Qualification and Validation
10. Handling of Data
11. Identity and Access Management
12. Audit Trails
13. Electronic Signatures
14. Periodic Reviews
15. Security
16. Backup
17. Archiving
+ Glossary

Subsection counts confirmed by ECA's analysis: **Chapter 12 (Audit Trails) "ten neatly structured subsections"; Chapter 14 (Periodic Reviews) "twelve subsections"; Chapter 7 (External IT Services / Supplier and Service Management) "nine subsections"** — [ECA Academy, Annex 11 Draft - First Analysis](https://www.gmp-compliance.org/gmp-news/annex-11-draft-first-analysis). My own retrieval of chapter 15 returned **20 numbered subsections (15.1–15.20)**, matching the "~20 subsections" in the assignment.

#### Chapter 2 — Principles `[A11-DRAFT-2025]`
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

**Note 2.3 — this is a valuable clause for a small supplier:** it explicitly permits alternative practices if *proven and documented* to give equal or higher control. A sole trader who cannot do four-eyes code review in the conventional way can use 2.3 to justify a documented alternative (e.g. automated test gates + external periodic code review).

#### Chapter 1 Scope `[A11-DRAFT-2025]`
> "This annex applies to all types of computerised systems used in the manufacturing of medicinal products and active substances."

#### Chapter 3 — Pharmaceutical Quality System `[A11-DRAFT-2025]` (entirely new chapter)
> **3.1** "A regulated user should implement a pharmaceutical quality system (PQS), which covers all computerised systems used in GMP activities and personnel involved with these."

Sub-requirements quoted:
> *Deviations:* "All deviations occurring during validation or operation of computerised systems are recorded and any significant deviations investigated with the objective of determining the root cause."
>
> *Changes:* "Any change to a computerised system including but not limited to its configuration, its hardware and software components, and its platform and operating system, are made in a controlled manner."
>
> *Internal audits:* "Internal audits are planned, conducted, reported and followed up on to detect procedural deviations and ensure product quality, patient safety and data integrity."
>
> *Management reviews:* "Regular management reviews cover relevant performance indicators for the computerised system and the process it is used in (quality metrics)."
>
> *Senior management oversight:* "Senior management effectively oversee the state of control throughout the system lifecycle, allocate appropriate resources, and implement a culture that promotes data integrity."

#### Chapter 4 — Risk Management `[A11-DRAFT-2025]`
> **4.1** *Lifecycle:* "Quality Risk Management (QRM) should be applied throughout the lifecycle of a computerised system considering any possible impact on product quality, patient safety or data integrity."
>
> **4.2** *Identification and analysis:* "Risks associated with the use of computerised systems in GMP activities should be identified and analysed according to an established procedure."
>
> **4.3** *Appropriate validation:* "The validation strategy and effort should be determined based on the intended use of the system and potential risks to product quality, patient safety and data integrity."
>
> **4.4** *Mitigation:* "Where applicable, risks associated with the use of computerised systems in GMP activities should be mitigated and brought down to an acceptable level, if possible, by modifying processes or system design."
>
> **4.5** *Data integrity:* "Quality risk management principles should be used to assess the criticality of data to product quality, patient safety and data integrity."

#### Chapter 5 — Personnel and Training `[A11-DRAFT-2025]`
> **5.1** *Cooperation:* "When conducting the activities required in this document, there should be, where applicable, close cooperation between all relevant parties."
>
> **5.2** *Training:* "All parties involved with computerised systems used in GMP activities should have adequate system specific training, and appropriate qualifications and experience."

#### Chapter 6 — System Requirements `[A11-DRAFT-2025]` (new; ECA: *"Almost one page is reserved for requirements specifications…which are often neglected in practice"*)
> **6.1** *GMP functionality:* "A regulated user should establish and approve a set of system requirements (e.g. a User Requirements Specification, URS), which accurately describe the functionality the regulated user has automated and is relying on when performing GMP activities."
>
> **6.2** *Extent and detail:* "The extent and detail of defined requirements should be commensurate with the risk, complexity and novelty of a system, and the description should be sufficient to support subsequent risk analysis, specification, design, purchase, configuration, qualification and validation."
>
> **6.3** *Ownership:* "The regulated user should carefully review and approve the document and consider whether the system fulfils GMP requirements and company processes as is, or whether it should be configured or customised."
>
> **6.4** *Update:* "Requirements should be updated and maintained throughout the lifecycle of a system to ensure that they continue to give a complete and accurate description of system functionality."
>
> **6.5** *Traceability:* "Documented traceability between individual requirements, underlaying design specifications and corresponding qualification and validation test cases should be established and maintained."
>
> **6.6** *Configuration:* "It should be clear what functionality, if any, is modified or added by configuration of a system. Options allowing configuration of system functionality should be described in the requirements specification."

**Meaning:** **6.5 makes a requirements traceability matrix effectively mandatory.** For telemetry.digital: maintain a living URS → design spec → test case matrix, kept updated per 6.4. This is the single most valuable artefact he can supply to customers.

#### Chapter 7 — Supplier and Service Management `[A11-DRAFT-2025]` — **critical for the user; see Q6 for full treatment**
> **7.1** "[…] the regulated user remains fully responsible for these activities" *(clause truncated by fetch tool — flagged)*
>
> **7.2** Audits required "according to risk and system criticality" to assess vendor procedures *(partially paraphrased by fetch tool — flagged)*
>
> **7.3** "[…] effective oversight […] according to defined service level agreements (SLA) and key performance indicators (KPI)" *(truncated — flagged)*
>
> **7.4** "[…] documentation for activities […] should be accessible and can be explained from their facility" *(truncated — flagged)*
>
> **7.5** — contract/agreement must address **nine elements**:
> - **i.** "Describes the activities and documentation to be provided"
> - **ii.** "Establishes the company procedures and regulatory requirements to be met"
> - **iii.** "Agrees on regular, ad hoc and incident reporting and oversight"
> - **iv.** "Agrees on conditions for supplier audits"
> - **v.** "Agrees on support during regulatory inspections, if so requested"
> - **vi.** "Agrees on resolution of issues brought up during normal operation"
> - **vii.** "Defines requirements and processes for communication of quality and security [issues/events — text truncated]"
> - **viii.** "Defines an exit strategy by which the regulated user may retain control [of data — text truncated]"
> - **ix.** "Agrees on the process for release of new system versions"

**This confirms the "nine mandatory contract elements" in the assignment.** Note items vii and viii were truncated by the fetch tool at the end — the substance is clear but exact wording needs verification.

#### Chapter 8 — Alarms `[A11-DRAFT-2025]` — **entirely new; the most product-specific chapter for telemetry.digital**
> **8.1** "Alarms should be implemented […] where the user must take a specific action" *(truncated — flagged)*
>
> **8.2** "Setting, changing or deactivation should only be available to users with appropriate access [privileges]"
>
> **8.3** "Alarms should set off visible and/or audible signals when set alarm limits [are] exceeded"
>
> **8.4** "Critical alarms […] should only be acknowledged by users with appropriate access privileges"
>
> **8.5** "All alarms and acknowledgements should be automatically added to an alarm log"
>
> **8.6** "Alarm logs should be searchable and sortable in the originating system"
>
> **8.7** "Alarm logs should be subject to appropriate periodic reviews based on approved procedures"

ECA notes chapter 8 introduces a *"non-erasable/deactivatable record (log) with a corresponding annotation, similar to an audit trail"* — [ECA Academy](https://www.gmp-compliance.org/gmp-news/annex-11-draft-first-analysis).

**Meaning — this chapter is essentially written for temperature monitoring systems.** Design consequences, each mapping to a subsection:
- **8.2** → alarm limits are privileged configuration; only an authorised role may set/change/**deactivate** them; every such action needs audit trail + reason (cross-refs clause 6 accuracy check and 12.4).
- **8.3** → visible and/or audible signalling. Email/SMS/push alone may not be enough; a visible dashboard alarm state is expected.
- **8.4** → **acknowledgement is privileged and personal** — no shared "ack" button, no acknowledging from an unauthenticated link in an email. This is a common design shortcut that would fail 8.4.
- **8.5** → an **alarm log** that is a *separate artefact from the audit trail*: every alarm raised, and every acknowledgement, auto-recorded. Append-only.
- **8.6** → the alarm log must be **searchable and sortable in the originating system** — i.e. inside telemetry.digital itself, not only in an exported CSV. This is a direct UI requirement.
- **8.7** → periodic review of alarm logs — another input he can package for customers.
- **Implied:** alarm *deactivation* is expressly contemplated and must be logged — so "muted"/"suppressed" sensors need a first-class, logged, time-bounded state, not a hidden config flag.

#### Chapter 9 — Qualification and Validation `[A11-DRAFT-2025]`
> **9.1** *Principles:* "Qualification and validation activities for computerised systems should follow the general principles outlined in GMP Annex 15. The activities should address both standard and configured system functionality, as well as any functionality realised through customisation."
>
> **9.2** *Quality risk management:* "Computerised systems should be qualified and validated in accordance with the principles of quality risk management. Decisions on the scope and extent of qualification and validation of specific functionality and entire systems should be based on a justified and documented risk assessment."
>
> **9.3** *Installation and configuration:* "Prior to commencing any test activity, it should be verified that a computerised system and its components have been correctly installed and configured according to specifications."
>
> **9.4** *Evidence:* "System qualification and validation should provide evidence in the form of executed test scripts, and where relevant, screen dumps, that requirements, and where applicable, derived functional specifications, are met by the system."
>
> **9.5** *Traceability:* "Test cases should be traceable to individual requirements or specifications, e.g. by means of a requirements traceability matrix."
>
> **9.6** *Focus:* "Increased focus should be on testing a system's handling of key functional requirements, on functionality intended to ensure that activities are conducted according to GMP, and on functionality designed to ensure data integrity."
>
> **9.7** *Plan and approval:* "Qualification and validation activities should be conducted according to approved plans, protocols and test scripts. Test scripts should be described in sufficient detail to ensure a correct and repeatable conduct of test steps and prerequisites."
>
> **9.8** *Completion prior to use:* "Qualification and validation activities should be successfully completed and reported prior to approval and taking a system into use."
>
> **9.9** *Authorisation:* "The regulated user is fully accountable and should carefully review and authorise the use of the documentation. They should carefully consider whether it covers the implemented version and supports GMP, and company processes as is."

**Note 9.1's explicit cross-reference to Annex 15** (Qualification and Validation) — new and binding on methodology. **9.4's "screen dumps"** means screenshot evidence is expected in test execution records. **9.9** is aimed at vendor-supplied validation packages: the customer must review and authorise them and **cannot simply outsource accountability to him** — worth saying plainly in his sales material so expectations are right.

#### Chapter 10 — Handling of Data `[A11-DRAFT-2025]` (replaces 2011 clauses 5 & 6)
> **10.1** *Input verification:* "Where critical data is entered manually, systems should, were applicable, have functionality to verify the plausibility of the inputs (e.g. within expected ranges), and alert the user when the input is not plausible." *(note: "were" appears in the draft — a typo for "where")*
>
> **10.2** *Data transfer:* "Where a routine work process requires that critical data be transferred from one system to another (e.g. from a laboratory instrument to a LIMS system), this should, where possible, be based on validated interfaces rather than on manual transcriptions."
>
> **10.3** *Data migration:* "Where an ad hoc process requires that critical data or a whole database be migrated from one system to another (e.g. when moving data from a retired to a new system), this should be based on a validated process."
>
> **10.4** *Encryption:* "Where applicable, critical data should be encrypted on a system."

**Big change vs 2011:** the 2011 clause 6 required a **second-operator or validated-electronic check** on manually entered critical data; draft **10.1 shifts the emphasis to built-in plausibility/range validation with a user alert** — a software function, not a procedure. **10.4 introduces encryption**, absent from 2011 entirely: he should implement encryption at rest (PostgreSQL disk/volume encryption or column-level for critical fields) and in transit (TLS everywhere, including sensor→gateway→ingest).

#### Chapter 11 — Identity and Access Management `[A11-DRAFT-2025]` — new chapter
> **11.1** "All users should have unique and personal accounts. The use of shared accounts […] constitute[s] a violation" *(truncated — flagged; substance: shared accounts violate data-integrity/attributability principles)*
>
> **11.2** "User accesses and roles should be granted, modified and revoked […] in a timely manner"
>
> **11.3** "The method of authentication should identify users with a high degree of certainty"
>
> **11.4** "Passwords […] should be changed at the first login, preferably required by the system"
>
> **11.5** "Passwords should be secure and enforced by systems […] contain a combination of uppercase, lowercase [, numbers and special characters — truncated]"
>
> **11.6** "Remote authentication on critical systems […] should include multifactor authentication (MFA)"
>
> **11.7** "Accounts should be automatically locked after a pre-defined number of successive failed [login] attempts"
>
> **11.8** "Systems should include an automatic inactivity logout […] The user should not be able to change [the timeout setting — truncated]"
>
> **11.9** "Systems should include an access log […] which, for each login, automatically logs the username [, date, time — truncated]"
>
> **11.10** *Guiding principles:*
> - "Segregation of duties, i.e. that users […] do not have administrative privileges"
> - "Least privilege principle, i.e. that users do not have higher access privileges than necessary"
>
> **11.11** "User accounts should be subject to recurrent reviews where managers confirm continued access [need — truncated]"

ECA notes chapter 11 also *"restricts smart card authentication and emphasizes segregation of duties"* — [ECA Academy](https://www.gmp-compliance.org/gmp-news/annex-11-draft-first-analysis).

**Meaning — a concrete feature checklist for telemetry.digital:** unique personal accounts (11.1); user lifecycle management with timely revocation (11.2); forced password change at first login (11.4); enforced password complexity **by the system** (11.5); **MFA for remote access to critical systems (11.6)** — for a cloud/SaaS product accessed over the internet, *all* access is remote, so MFA is effectively mandatory; account lockout after N failed attempts (11.7); **inactivity auto-logout the user cannot change (11.8)** — must be an admin-only setting; an **access log** separate from both the audit trail and the alarm log (11.9); **role design enforcing segregation of duties — a user who enters/approves GMP data must not also be a system administrator (11.10)**, which for a sole trader is a real tension he must address with documented compensating controls; and periodic access review support (11.11).

#### Chapter 12 — Audit Trails `[A11-DRAFT-2025]` — 10 subsections
> **12.1** "Systems […] should have an audit trail functionality which automatically logs all manual user interactions"
>
> **12.2** "The audit trail should unambiguously capture the user […] what was changed […] when the change was made [and why — truncated]"
>
> **12.3** "Audit trail functionality should be enabled and locked at all times […] should not be possible for any user to edit [or disable it — truncated]"
>
> **12.4** "Systems should accommodate effective and efficient reviews of audit trail data […] sort and search"
>
> **12.5** "Audit trail reviews should be conducted according to a documented procedure […] Appropriate action [should be] taken"
>
> **12.6** "Audit trail reviews should be conducted by personnel not directly involved in the activities reviewed"
>
> **12.7** "Reviews should be targeted, based on risk […] focus on detecting any deliberate or indeliberate changes"
>
> **12.8** "Audit trail reviews should be conducted in a timely manner […] prior to batch release, unless justified"
>
> **12.9** "It should be possible to obtain a complete electronic copy […] Flat and locked files are not acceptable"
>
> **12.10** "Audit trail reviews with direct impact on the release of a product should be available to the QP"

**Meaning — the biggest single change from 2011.** The 2011 clause 9 said "consideration should be given, based on a risk assessment"; **12.1 makes audit trail functionality a flat requirement** covering **"all manual user interactions"**. Specific design consequences:
- **12.3 — "enabled and locked at all times […] not possible for any user to edit"**: the audit trail must be **technically immutable, including against administrators**. In PostgreSQL: revoke UPDATE/DELETE on the audit table from the app role, use triggers or an append-only pattern, ideally hash-chain the rows so tampering is detectable. There must be **no configuration switch to turn the audit trail off** — not even a hidden one.
- **12.4 + 12.9 — the audit trail must be reviewable in-system (sort/search) AND exportable as a "complete electronic copy"**, explicitly **"Flat and locked files are not acceptable"** — meaning a locked PDF export does **not** satisfy the requirement. He must provide a **structured, machine-processable export (CSV/XML/JSON) that remains sortable and searchable** by the recipient. This is a direct, testable product requirement and a common failure point.
- **12.6 — "conducted by personnel not directly involved in the activities reviewed"** — this is the **"targeted independent review"** referred to in the assignment. It is an organisational requirement on the *customer*, but his software must **support** it: a reviewer role with read access to audit trails but no ability to alter the underlying data, and a way to record that a review happened, by whom, when, and with what outcome. **Building an "audit trail review" workflow into the product is a strong differentiator** — most competitors leave the customer to do this in Excel.
- **12.7 — targeted, risk-based reviews** rather than reading everything: so filtering by risk-relevant event types (setpoint changes, deletions, excursion annotations, permission changes) must be first-class.
- **12.10** — QP access to release-relevant audit trail reviews.

#### Chapter 13 — Electronic Signatures `[A11-DRAFT-2025]` — quoted in full sentences
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

**Meaning:** hugely expanded from the 2011 three-bullet clause 14. **13.2 links to eIDAS trusted services for open systems** — relevant if telemetry.digital is a multi-tenant SaaS where he (not the customer) controls access; that arguably makes it an **open system** in the annex's sense, pulling in eIDAS-qualified signature requirements. **13.3 requires full re-authentication at the moment of signing** (not just an active session). **13.6** specifies exactly what the signature manifestation must display. **13.8** requires that a later change makes the record **clearly appear as unsigned** — a strong technical requirement best met by hashing the signed record version.

#### Chapter 14 — Periodic Reviews `[A11-DRAFT-2025]` — 12 subsections
> **14.1** "After a system [is taken into use], periodic reviews should be conducted […] verify whether the system remains 'fit for intended use'"
>
> **14.2** *Scope includes:* items i–iii on changes to hardware, software, documentation, configuration; items iv–xii on follow-up on supporting processes including **audit trail reviews, security incidents, compliance** *(individual item wording not retrieved — flagged)*
>
> **14.3** "The frequency of reviews should be established and justified based on the risk the system poses"

#### Chapter 15 — Security `[A11-DRAFT-2025]` — 20 subsections, the largest chapter and almost entirely new
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

*(All 15.x quotes were truncated by the fetch tool at the ellipses — substance is reliable, exact wording needs verification.)*

ECA summarises chapter 15 as covering *"firewalls, disaster recovery - RTO/RPO, patches, virus protection"* and mandatory *"penetration tests for critical systems"* — [ECA Academy](https://www.gmp-compliance.org/gmp-news/annex-11-draft-first-analysis).

**Meaning for telemetry.digital — the most demanding new chapter:**
- **15.6 / 15.7** — **RTO (Recovery Time Objective) and RPO must be defined, and the disaster recovery plan must be tested.** For a cloud-hosted monitoring service this means documented DR runbooks and evidence of a DR test, not just "we have backups".
- **15.8 / 15.9** — network segmentation and firewall rules, with **periodic firewall rule review**.
- **15.10–15.14** — a real patching discipline: timely OS/platform patching, **immediate deployment for critical vulnerabilities (15.13)**, planned revalidation after OS updates (15.11), and isolation of anything running on end-of-life platforms (15.12). For a Go/PostgreSQL stack: pin and track dependency CVEs (govulncheck), track PostgreSQL EOL dates, and document the patch process.
- **15.19** — **regular penetration testing for critical internet-facing systems.** A SaaS temperature monitoring platform is internet-facing and critical. This is a recurring external cost he should budget for and can market as a trust signal.
- **15.20** — encrypted protocols for remote connections (TLS; SSH for admin).
- Note **15.1's "information security management system"** — this is effectively an ISO 27001-shaped expectation. **Commercial implication: ISO 27001 certification, or a documented ISMS mapped to it, would let him answer chapter 15 and chapter 7 supplier questions in one move.**

#### Chapter 16 — Backup `[A11-DRAFT-2025]` — quoted in full sentences (new dedicated chapter)
> **16.1** *Regular backup:* "Data and metadata should be regularly backed up following established procedures to prevent the loss of data in case of accidental or deliberate change or deletion, loss as the result of a malfunction or corruption, e.g. as the result of a cyber-attack."
>
> **16.2** *Frequency and retention:* "The frequency, retention period and storage of backups is critically important to the effectiveness of the process to mitigate the loss of data. Backups should be made at suitable intervals (e.g. hourly, daily, weekly and monthly) and their retention determined through a risk-based approach (e.g. correspondingly a week, a month, a quarter, and years)."
>
> **16.3** *Physical separation:* "Backups should be physically separated from the server or computer holding the original data and stored at a safe distance from this, to prevent that both would be impacted by the same incident."
>
> **16.4** *Logical separation:* "Backups should not be stored at the same logical network as the original data to avoid simultaneous destruction or alteration."
>
> **16.5** *Scope:* "Depending on the criticality and urgency for recovery after an incident, applications and system configurations may also need to be backed up."
>
> **16.6** *Restore test:* "Restore of data from backup should be tested and documented based on risk during system validation and after changes are made to the backup or restore processes and tools."

**Meaning:** far more prescriptive than 2011's clause 7.2. **16.2 gives an explicit worked example of a grandfather-father-son schedule** (hourly/daily/weekly/monthly backups retained a week/month/quarter/years) — a defensible default to implement. **16.3 + 16.4 together are an anti-ransomware requirement**: backups must be both **physically** and **logically** separated — a backup on the same cloud account/VPC as the production database does **not** satisfy 16.4. Use a separate account/provider/region with separate credentials, ideally immutable/object-lock storage. **16.5** extends backup scope to **application code and system configuration**, not just data.

#### Chapter 17 — Archiving `[A11-DRAFT-2025]` — quoted in full sentences (expanded from 3 sentences to 5 subsections)
> **17.1** *Read only:* "After completion of a process, e.g. release of a product, GMP data and metadata (incl. audit trails) should be protected from deletion and changes throughout the retention period."
>
> **17.2** *Verification:* "When moving GMP data and metadata from one location to another in a system, or to a dedicated archival system, the integrity of the data should be verified by a high degree of certainty before any data is deleted, e.g. by means of a checksum."
>
> **17.3** *Backup:* "If data is archived on a server (disk), it should be regularly backed up following the same procedures as for live data (see 16 Backup)."
>
> **17.4** *Durability:* "If data is archived long-term on volatile storage media with limited durability (e.g. CD), this should follow a validated process."
>
> **17.5** *Retrieval:* "It should be possible to retrieve archived data and metadata in a format which allows searching and sorting of the data, or alternatively, to allow export of the data to a tool where this is possible."

ECA: chapter 17 addresses data archiving comprehensively **for the first time in GMP regulations** — [ECA Academy](https://www.gmp-compliance.org/gmp-news/annex-11-draft-first-analysis).

**Meaning:** **17.1 explicitly includes audit trails in what must be archived and protected** — the audit trail is retained as long as the data it describes. **17.2 requires checksum-style integrity verification before any deletion during a move** — directly applicable to data-tiering (e.g. moving old partitions to cold storage) and to customer migrations. **17.5 requires archived data to remain searchable and sortable** — so archiving to a pile of static PDFs or flat files fails; archive to a queryable store or provide an export path into one.

#### Glossary `[A11-DRAFT-2025]` — quoted in full
> **ALCOA+:** "An acronym for 'attributable, legible, contemporaneous, original and accurate', which puts additional emphasis on the attributes of being complete, consistent, enduring and available."
>
> **Audit trail:** "In computerised systems, an audit trail is a secure, computer generated, time-stamped electronic record that allows reconstruction of the events relating to the creation, modification, or deletion of an electronic record."
>
> **Backup:** "Provisions made for the recovery of data files or software, for the restart of processing, or for the use of alternative computer equipment after a system failure or disaster."
>
> **Electronic record:** "Any combination of text, graphics, data, audio, pictorial, or other information representation in digital form that is created, modified, maintained, archived, retrieved, or distributed by a computer system."
>
> **Validation:** "Action of proving that a process leads to the expected results. Validation of a computerised system requires ensuring and demonstrating the fitness for its purpose."

#### AI / Annex 22 and cloud services in the draft
- **The draft Annex 11 document itself does not mention Annex 22 or artificial intelligence** in the body text I retrieved. AI is handled in the **separate new Annex 22 (Artificial Intelligence)**, published in the same consultation package — [draft Annex 22 PDF](https://health.ec.europa.eu/document/download/5f38a92d-bb8e-4264-8898-ea076e926db6_en?filename=mp_vol4_chap4_annex22_consultation_guideline_en.pdf)
- **Cloud services are referenced once**, in the introduction/reasons for changes: *"With an ever-evolving IT landscape, increased use of cloud services, and introduction of new technologies in computerised systems used in GMP activities, there is a growing need for updated guidance."* — [A11-DRAFT-2025]. Cloud is otherwise handled **indirectly but pervasively**, through chapter 7 (supplier and service management, SLAs/KPIs, exit strategy) and chapter 15 (data centres, DR/RTO, network segmentation, remote access).
- **Data integrity has no single dedicated chapter** in the draft; it is threaded through as a core principle (2.4, 3.1 senior management culture, 4.5, 9.6, and the whole of chapters 11/12/16/17).

### Inferences
- **The draft does not contain a chapter equivalent to 2011's clause 15 (Batch release) or 13 (Incident Management) as standalone chapters** — incident/deviation handling has been absorbed into chapter 3 (Pharmaceutical Quality System) and chapter 14 (periodic review follow-up). The report writer should not assume a 1:1 clause mapping between the two versions; the renumbering is total.
- **Clause-number mapping 2011 → draft** (approximate, for the user's traceability matrix): 1→4; 2→5; 3→7; 4→6+9; 5→10.2; 6→10.1; 7.1→15/17; 7.2→16; 8→(dispersed; 12.9); 9→12; 10→3 (changes); 11→14; 12→11+15; 13→3 (deviations); 14→13; 15→(no direct equivalent); 16→15.7; 17→17. **New with no 2011 ancestor: chapter 8 (Alarms) and most of chapter 15 (Security).**
- **For telemetry.digital specifically, the draft is in large part a product-requirements document.** Chapter 8 (alarms), chapter 11 (identity/access), chapter 12 (audit trails), chapter 16 (backup) and chapter 17 (archiving) can be read almost directly as a feature backlog. Building to them now is cheaper than retrofitting, and the immutability requirement (12.3) in particular is very hard to retrofit into an existing schema.

### Gaps
- **Full verbatim text of chapters 7, 8, 11, 12, 14 and 15 could not be obtained** — the fetch tool enforced a 125-character-per-quote cap and truncated. The substance is reliable but the exact wording of these clauses (including the precise phrasing of the nine contract elements in 7.5, and the twelve items in 14.2) must be verified from the source PDF before citation in a validation document.
- The **individual items i–xii of clause 14.2** (periodic review scope) were not retrieved.
- I could not verify the claim that the draft grew "from 5 to ~19 pages"; no source I reached states a page count.
- The content of **draft Annex 22 (Artificial Intelligence)** was not researched — it is a separate document and outside the retrieved scope. Its URL is given above if the report writer needs it.

---

## Q4. What Annex 11 requires of the SUPPLIER of the software — the user IS the supplier

### Takeaway
Under the 2011 text in force, clause **3.1–3.4** plus **4.5** place the *legal* obligation on the regulated user (the pharma warehouse), but the practical burden lands on the supplier: he must have a **formal written agreement**, a demonstrable **quality management system**, be **auditable**, supply **documentation** that lets the customer verify their user requirements, and accept that his **quality system and audit information must be made available to inspectors**. The draft's chapter 7 turns this into **nine mandatory contract elements** plus SLA/KPI oversight and an exit strategy.

### Cited Findings

#### `[A11-2011]` clause 3 — the supplier clauses, verbatim
> **3.1** "Formal agreements must exist between the manufacturer and any third parties, and these agreements should include clear statements of the responsibilities of the third party."
>
> **3.2** "The competence and reliability of a supplier are key factors when selecting a product or service provider." *(remainder of clause, concerning the need for and justification of an audit, not retrieved — flagged)*
>
> **3.3** "Documentation supplied with commercial off-the-shelf products should be reviewed by regulated users to check that user requirements are fulfilled."
>
> **3.4** "Quality system and audit information relating to suppliers or developers of software and implemented systems should be made available to inspectors on request."

#### `[A11-2011]` clause 4.5 — the assessment hook
> "The regulated user should take all reasonable steps, to ensure that the system has been developed in accordance with an appropriate quality management system. The supplier should be assessed appropriately."

#### `[A11-2011]` clause 4.6 — bespoke software
> "For the validation of bespoke or customised computerised systems there should be a process in place that ensures the formal assessment and reporting of quality and performance measures."

#### `[A11-DRAFT-2025]` chapter 7 — the nine contract elements *(see Q3 for the quoted text of 7.1–7.5 and the truncation caveat)*
The contract/agreement must, per **7.5**: (i) describe activities and documentation to be provided; (ii) establish company procedures and regulatory requirements to be met; (iii) agree regular, ad hoc and incident reporting and oversight; (iv) agree conditions for supplier audits; (v) agree support during regulatory inspections if requested; (vi) agree resolution of issues during normal operation; (vii) define requirements and processes for communication of quality and security matters; (viii) **define an exit strategy by which the regulated user may retain control** (of their data/system); (ix) agree the process for release of new system versions.

Plus: **7.1** the regulated user remains fully responsible; **7.2** audits according to risk and system criticality; **7.3** effective oversight via defined **SLAs and KPIs**; **7.4** documentation accessible and explainable from the regulated user's own facility.

Also **draft 2.6**: *"When using outsourced activities, the regulated user remains fully responsible for adherence to the requirements included in this document."*

And **draft 9.9**: *"The regulated user is fully accountable and should carefully review and authorise the use of the documentation. They should carefully consider whether it covers the implemented version and supports GMP, and company processes as is."*

Independent confirmation that vendor qualification is an Annex 11 distinctive: *"Annex 11 explicitly requires supplier qualification in its general requirements (Clause 3)"*, whereas *"FDA 21 CFR Part 11 does not explicitly address risk management or vendor assessment"* — [SimplerQMS, 21 CFR Part 11 vs EU Annex 11](https://simplerqms.com/21-cfr-part-11-vs-eu-annex-11/)

Vendors serving both regimes are advised to deliver validation documentation per **ISPE GAMP 5**, role-based access controls, comprehensive audit trails, controlled printout capability, standardised workflows, and **complete supplier qualification documentation demonstrating technical competence** — [SimplerQMS](https://simplerqms.com/21-cfr-part-11-vs-eu-annex-11/)

### Inferences — concrete supplier checklist for a Slovak sole trader
What a pharma warehouse customer (or a SUKL/EMA inspector via the customer) will ask him for, derived directly from the clauses above:

1. **A written Quality/Technical Agreement with every customer** (2011 clause 3.1; draft 7.5). Must state responsibilities of each party explicitly. **This is the single highest-priority document** — a sole trader selling without one fails clause 3.1 on day one, and the customer fails their own audit. He should draft a standard bilingual (SK/EN) agreement covering all nine draft 7.5 elements now, so it survives the revision.
2. **A documented QMS covering software development** (2011 clause 4.5/4.6; draft 3.1). Even a lightweight one: SOPs for requirements management, design, coding standards, code review, testing, release/change control, incident/CAPA handling, backup, and security. ISO 9001 or ISO 27001 certification is not required by Annex 11 but makes clause 3.2/4.5 assessment trivial and is a strong sales asset against larger competitors.
3. **Acceptance of supplier audits** (2011 clause 3.2; draft 7.2, 7.5 item iv). He must be willing and contractually obliged to host a customer audit — remote or on-site. Preparing a **standard supplier audit dossier / vendor qualification pack** in advance (company info, QMS overview, SDLC description, security posture, DR/backup approach, references, sample validation documentation) turns each audit from days into hours and is a repeatable sales asset.
4. **Documentation package for the customer to review** (2011 clause 3.3; draft 6.x, 9.x). Since telemetry.digital is bespoke rather than COTS, this should include: system description, functional specification, template URS, requirements traceability matrix (draft 6.5), IQ/OQ protocols and templates, test evidence including screenshots (draft 9.4), release notes, admin/user manuals, and a **data integrity / Annex 11 compliance statement** mapping each clause to how the product meets it. **That clause-by-clause compliance matrix is the single most sales-effective document he can produce.**
5. **Quality system and audit info available to inspectors on request** (2011 clause 3.4). He must keep his own QMS records in order and be prepared for them to reach a regulator indirectly. Support during regulatory inspections is a contract element in the draft (7.5 item v).
6. **SLA with KPIs** (draft 7.3). Uptime, alarm-delivery latency, incident response and notification times, support response times, patch deployment times. He should define these now — draft 7.3 makes SLA/KPI-based oversight explicit, and it is also how he differentiates.
7. **Exit strategy** (draft 7.5 item viii). He must contractually guarantee the customer can retain control of their data if he stops trading, the contract ends, or he is unavailable. **For a sole trader this is an existential customer objection — bus-factor one.** Mitigations that also satisfy 7.5(viii): documented full data export in an open, searchable format (cross-ref 12.9 and 17.5); source-code escrow; a documented handover/continuity plan; the customer's right to a full database dump on demand. Addressing this proactively neutralises the main commercial objection to buying from a one-man company.
8. **Version release process** (draft 7.5 item ix). A documented, customer-agreed release procedure: advance notice, release notes, regression test evidence, impact assessment, and customer approval before production deployment. Customers cannot maintain a validated state (draft 2.1) if he pushes changes unannounced — **continuous deployment straight to production is incompatible with GMP** and he must design his release process accordingly.
9. **Incident notification obligations** (2011 clause 13; draft 7.5 item iii and vii). Defined channels and timelines for reporting incidents, including **security** incidents (draft 7.5 vii), to the customer.

**Overall strategic inference:** for a sole trader the regulatory burden is real but tractable, and it is a **moat**, not just a cost. Most small competitors in the Slovak/CEE market will not have a quality agreement template, an audit dossier, a traceability matrix, an exit strategy or a clause-by-clause compliance matrix. Producing these — sized to a one-person business, leaning on draft clause 2.3's explicit allowance for documented alternative practices — is what makes the product sellable into GMP warehouses at all.

### Gaps
- The remainder of 2011 clause 3.2 (which addresses when an audit is needed and how that need is justified) was not retrieved verbatim.
- Exact verbatim wording of draft 7.1–7.4 and the precise closing words of items vii and viii of 7.5 were truncated by the fetch tool.
- I found no EU guidance specifically addressing how a **sole trader / micro-enterprise** supplier should satisfy segregation-of-duties expectations (draft 11.10) — this appears to be an unaddressed gap in the guidance and will have to be handled by documented compensating controls justified under draft clause 2.3.

---

## Q5. Revised Chapter 4 (Documentation) draft — electronic records and retention

### Takeaway
The draft Chapter 4 is heavily expanded (sections 4.1–4.85 plus glossary) and is markedly more digital-first than the 2011 version: it puts electronic and paper documentation on an equal footing, **discourages hybrid systems**, requires regulated users to **define which electronic data are raw data**, and keeps the classic retention rule (batch documentation: 1 year after batch expiry or 5 years after QP certification, whichever is longer).

**Source:** `[CH4-DRAFT-2025]` — [Draft revised Chapter 4 Documentation (PDF)](https://health.ec.europa.eu/document/download/fa336a6e-753b-46fc-b53b-9178bacd8878_en?filename=mp_vol4_chap4_consultation_guideline_en.pdf). **NOT IN FORCE.**

### Cited Findings

#### Draft Chapter 4 structure `[CH4-DRAFT-2025]`
1. Principle (4.1–4.9) · 2. Data Governance Systems (4.10–4.18) · 3. Risk Management (4.19–4.22) · 4. General Requirements for Documentation (4.23–4.26) · 5. Master Documents (4.27–4.48) · 6. Generation and Control of Documentation (4.49–4.55) · 7. Good Documentation Practice (4.56–4.63) · 8. Signatures in GMP Relevant Documentation (4.64–4.75) · 9. Retention of Documents (4.76–4.79) · 10. Data Integrity in Documentation (4.80–4.81) · 11. Hybrid Systems (4.82–4.85) · 12. Glossary

#### Electronic records `[CH4-DRAFT-2025]`
> **4.7** "Documentation may exist in a variety of forms, including paper-based, electronic, or other means (e.g. photography, imagery, video and audio recordings)."
>
> **4.8** "Whether documents are created, stored, and managed electronically, paper based, by other means or through a hybrid system, they must meet the same GMP requirements for legibility, accuracy, integrity, and completeness throughout the whole lifecycle."
>
> **4.26** "Data which is recorded or processed electronically should not be converted to or stored in a paper form unless it meets the requirements set out in section 13 'hybrid systems' or the conversion is validated or verified for accuracy."
>
> **4.70** "If records exist electronically such records should be signed electronically. The use of a hybrid system should be avoided."
>
> **4.36** "Where a validated process is continuously monitored and controlled, then automatically generated reports may be limited to compliance summaries and exception/out-of-specification (OOS) data reports."

#### Raw data and metadata `[CH4-DRAFT-2025]`
> **4.27** "Records include the raw data which is used to generate other records. For electronic records regulated users should define which data are to be used as raw data. At least, all data on which quality decisions are based should be defined as raw data."
>
> **4.12** "For derived data the traceability which allows reconstruction of all data processing activities should be maintained."
>
> *Glossary — Raw data:* "Raw data is defined as the original record (data) which can be described as the first capture of stored information, whether recorded on paper or electronically."
>
> *Glossary — Complete electronic record:* "A complete record of data generated electronically includes relevant metadata."

#### Hybrid systems `[CH4-DRAFT-2025]`
> **4.82** "They should be clearly defined and identified, and each contributing element of the system validated and controlled according to risk management principles."
>
> **4.83** "A detailed description of the entire system should be available. The description should outline all major components, their functions, and interactions with each other as well as control for data management and data integrity." … "Procedures and records should be available to manage and appropriately control the interface between manual and computerised systems."
>
> **4.85** "Procedures should be in place to manage the review of data generated by hybrid systems which clearly outline the process for the evaluation, approval and archiving of electronic and paper-based data."

#### True copies and durability `[CH4-DRAFT-2025]`
> *Glossary — True copy:* "An exact copy of original documentation that preserves the same content, meaning and attributes of the original. The term 'true copy' is synonymous with 'certified' or 'verified copy'."
>
> **4.58** "Recorded media should be durable throughout the retention period. If this is not feasible, then true copies should be generated. For this case a documented system should be in place to verify and record the integrity of the copy."

#### Retention `[CH4-DRAFT-2025]`
> **4.77** "Batch documentation which must be kept for one year after expiry of the batch to which it relates or at least five years after certification of the batch by the Qualified Person, whichever is the longer."
>
> **4.77** (IMPs) "For investigational medicinal products, the batch documentation must be kept for at least five years after the completion or formal discontinuation of the last clinical trial in which the batch was used."
>
> **4.78** "For other types of documentation, the retention period will depend on the business activity which the documentation supports. Critical documentation, including raw data (for example relating to validation or stability), which supports information in the Marketing Authorisation should be retained whilst the authorization remains in force."
>
> **4.78** "It may be considered acceptable to retire certain documentation (e.g. raw data supporting validation reports or stability reports) where the data has been superseded by a full set of new data."
>
> **4.79** "A documented process for the disposal of records should be in place to ensure that the correct original records or true copies are disposed only after the defined retention period."

### Inferences — what this means for telemetry.digital
- **4.27 is directly actionable:** the customer must **define which telemetry data are raw data**. For a temperature monitoring system the raw data is the **first captured measurement as received from the sensor** — not the averaged/downsampled series shown on a dashboard and not the PDF report. **Design consequence: never overwrite or discard the original measurement.** If he downsamples or aggregates for performance, the original series must be retained and retrievable, and **4.12** requires traceability allowing reconstruction of all processing (so the aggregation logic and its parameters must be documented and reproducible). **Aggressive time-series downsampling that destroys originals would be a data integrity failure.**
- **Retention drives storage architecture.** Warehouse temperature records support the storage conditions of batches; under 4.77 logic those records need to live **at least 5 years after QP certification, or 1 year past batch expiry if longer** — realistically **5–10+ years** for a warehouse holding long-dated products. Combined with 2011 clause 7.1 ("access to data should be ensured throughout the retention period") and draft 17.5 (archived data must remain searchable/sortable), he needs a costed, tested long-term retention plan — **and a contractual answer to "what happens to 8 years of my data if you stop trading"** (draft Annex 11 7.5 item viii).
- **4.70 + 4.26 push strongly against hybrid.** A workflow where the system produces a PDF that a QA person prints and wet-ink signs is exactly the hybrid pattern the draft says "should be avoided". **Building in-app electronic signature (per draft Annex 11 chapter 13) is therefore both a compliance and a competitive argument.**
- **4.36 is a small but genuine gift to monitoring vendors:** where a validated process is continuously monitored and controlled, **automatically generated reports may be limited to compliance summaries and exception/OOS reports**. This legitimises exception-based reporting — the customer does not need to print 8,760 hourly readings per sensor per year; a summary plus excursion report suffices. **This is a strong feature/marketing point** (and it must still be backed by full retained raw data).

### Gaps
- I did not retrieve the full verbatim text of Chapter 4 sections 4.10–4.18 (Data Governance Systems), 4.64–4.75 (Signatures), or 4.80–4.81 (Data Integrity in Documentation), which are likely relevant.
- Note an internal inconsistency in the draft as retrieved: **4.26 cross-references "section 13 'hybrid systems'"** but hybrid systems appear as section 11 (4.82–4.85) in the structure. This may be a numbering error in the consultation draft, or an artefact of retrieval — worth flagging, and worth the user checking the source PDF.
- The GDP-specific angle (Guidelines on Good Distribution Practice 2013/C 343/01, which is what actually governs a *pharmaceutical warehouse* as opposed to a manufacturing site) was outside this assignment's scope but is arguably **more directly applicable to telemetry.digital's customers than GMP Annex 11** — see the note under "Cross-cutting" below.

---

## Q6. Relationship between Annex 11 and 21 CFR Part 11 (for customers who also export to the US)

### Takeaway
They are complementary rather than equivalent: **21 CFR Part 11 is a binding US regulation narrowly about electronic records and electronic signatures; Annex 11 is an EU GMP guideline that governs the whole computerised system lifecycle.** Annex 11 is broader on validation, risk management, periodic review and supplier qualification; Part 11 is more prescriptive on the technical mechanics of signatures. A system built to the *stricter of the two on each topic* satisfies both, and the 2025 draft narrows the gap considerably.

### Cited Findings
- **Legal status:** Part 11 is *"a binding U.S. regulation within Title 21 of the Code of Federal Regulations"*; Annex 11 is *"a legally binding European guideline annex that forms part of EU-GMP requirements, enforced by national competent authorities across member states"* — [SimplerQMS](https://simplerqms.com/21-cfr-part-11-vs-eu-annex-11/)
- **Scope:** Part 11 *"applies broadly across all FDA-regulated industries (pharma, biotech, medical devices), focusing specifically on electronic records and signatures"*; Annex 11 *"applies narrowly to medicinal products but takes a 'more holistic approach, governing computerized systems in GMP environments'"* — [SimplerQMS](https://simplerqms.com/21-cfr-part-11-vs-eu-annex-11/)
- **Validation:** *"Annex 11 demands comprehensive system lifecycle validation, from User Requirements Specifications through design, testing, operation, and retirement"* and *"Annex 11 mandates periodic reviews to confirm that validated systems remain in compliance"*, whereas Part 11 addresses validation only in the context of electronic record management — [SimplerQMS](https://simplerqms.com/21-cfr-part-11-vs-eu-annex-11/)
- **Audit trails:** *"Part 11 requires audit trails for all electronic records subject to FDA predicate rules. Annex 11 requires them only 'where a risk assessment determines them necessary for computerized systems with GMP-relevant data.' The updated Annex 11 narrows this gap by expanding audit trail scope."* — [SimplerQMS](https://simplerqms.com/21-cfr-part-11-vs-eu-annex-11/)
- **Electronic signatures:** both require signatures to be unique, verifiable and permanently linked to records; *"Part 11 provides 'more prescriptive and technical requirements,' while Annex 11 outlines principles without mandating specific implementation methods"* — [SimplerQMS](https://simplerqms.com/21-cfr-part-11-vs-eu-annex-11/)
- **Supplier/vendor — a major difference:** *"Annex 11 explicitly requires supplier qualification in its general requirements (Clause 3). FDA 21 CFR Part 11 does not explicitly address risk management or vendor assessment."* Those expectations sit in broader GMP predicate rules in the US — [SimplerQMS](https://simplerqms.com/21-cfr-part-11-vs-eu-annex-11/)
- **Recommended vendor strategy:** *"Vendors should adopt the stricter Annex 11 requirements as baseline, since 'the draft revision of EU Annex 11 is more aligned with FDA 21 CFR Part 11,' narrowing the compliance gap over time."* — [SimplerQMS](https://simplerqms.com/21-cfr-part-11-vs-eu-annex-11/)

### Inferences
- **Practical dual-compliance design rules for telemetry.digital**, taking the stricter requirement on each axis:
  - **Audit trail: always on, never risk-optional** (Part 11 §11.10(e) requires it unconditionally for records under predicate rules; draft Annex 11 12.1 agrees). Do not build a configurable "audit trail enabled" toggle.
  - **E-signatures: follow Part 11's prescriptiveness** — signature manifestation showing printed name, date/time and meaning of signing (Part 11 §11.50); signature/record linking (§11.70); two distinct identification components; re-authentication at signing (matching draft Annex 11 13.3).
  - **Copies: Part 11 §11.10(b)** requires the ability to generate accurate and complete copies in both human-readable **and electronic** form suitable for inspection — which aligns with and reinforces draft Annex 11 12.9's *"Flat and locked files are not acceptable"*.
  - **Validation, supplier qualification, periodic review, business continuity: follow Annex 11** — Part 11 is largely silent on these, so Annex 11 sets the higher bar.
- **A single well-designed product satisfies both.** The union of requirements is essentially: unique authenticated accounts + RBAC + immutable audit trail + structured export + compliant e-signature + validated + supplier-qualified + periodically reviewed. There is no architectural conflict between the two regimes.
- Useful commercial framing for his Slovak customers who export to the US: "Annex 11 + Part 11 compliant by design" is a credible claim if he builds to the draft, because the draft was explicitly moved closer to Part 11.

### Gaps
- I relied on a single secondary source (SimplerQMS, a QMS vendor) for the comparison rather than reading 21 CFR Part 11 itself. The characterisations are consistent with my background knowledge of Part 11 and with the Annex 11 primary text quoted above, but **exact verbatim 21 CFR Part 11 clause text (§11.10, §11.50, §11.70, §11.100–11.300) was not retrieved** and should be taken from [ecfr.gov](https://www.ecfr.gov/current/title-21/chapter-I/subchapter-A/part-11) if the report needs verbatim US citations.
- The source is a vendor marketing page; its legal characterisation of Annex 11 as "legally binding" is a simplification (Annex 11 is guidance to Directive 2003/94/EC and its successor, enforced through GMP inspection rather than being directly binding law). Flag this nuance rather than repeating the vendor's phrasing uncritically.

---

## Cross-cutting note for the report writer (scope caveat, not a finding)

Two scoping points the user should not miss, both of which I flag as **inference from the primary texts rather than sourced findings**:

1. **Annex 11's own scope is manufacturing.** `[A11-2011]` Principle: *"This annex applies to all forms of computerised systems used as part of a GMP regulated activities."* `[A11-DRAFT-2025]` 1: *"This annex applies to all types of computerised systems used in the manufacturing of medicinal products and active substances."* A pure **distribution/wholesale warehouse** is governed by **GDP** (Guidelines of 5 November 2013 on Good Distribution Practice, 2013/C 343/01), whose section 3.2.1 covers temperature mapping and monitoring of storage areas and section 3.3 covers computerised systems — and which largely mirrors Annex 11's expectations. **If telemetry.digital's customers are warehouses/wholesalers rather than manufacturing sites, GDP is the primary framework and Annex 11 applies by analogy and via inspector expectation.** If they are manufacturers' own warehouses, Annex 11 applies directly. He likely needs to address **both**, and GDP chapter 3 / Annex 15 temperature mapping was outside this assignment's scope.
2. **Temperature mapping and sensor calibration are not Annex 11 topics** but will be asked about in the same breath by every customer: calibrated sensors with traceable certificates, documented mapping studies of the warehouse, and defined alarm limits derived from the mapping. Annex 11 governs the *software*; these govern the *measurement*. Worth a separate research thread.

---

## Source list

| Ref | Document | URL |
|---|---|---|
| `[A11-2011]` | EU GMP Annex 11 Computerised Systems, rev. January 2011 (in force 30 June 2011) | https://health.ec.europa.eu/system/files/2016-11/annex11_01-2011_en_0.pdf |
| — | EudraLex Volume 4 official index (status check, 21 Sept 2026) | https://health.ec.europa.eu/medicinal-products/eudralex/eudralex-volume-4_en |
| `[A11-DRAFT-2025]` | Draft revised Annex 11, consultation 7 July 2025 | https://health.ec.europa.eu/document/download/40231f18-e564-4043-94de-c031f813d38b_en?filename=mp_vol4_chap4_annex11_consultation_guideline_en.pdf |
| `[CH4-DRAFT-2025]` | Draft revised Chapter 4 Documentation | https://health.ec.europa.eu/document/download/fa336a6e-753b-46fc-b53b-9178bacd8878_en?filename=mp_vol4_chap4_consultation_guideline_en.pdf |
| — | Draft new Annex 22 Artificial Intelligence (not analysed) | https://health.ec.europa.eu/document/download/5f38a92d-bb8e-4264-8898-ea076e926db6_en?filename=mp_vol4_chap4_annex22_consultation_guideline_en.pdf |
| — | Consultation page (dates, status) | https://health.ec.europa.eu/consultations/stakeholders-consultation-eudralex-volume-4-good-manufacturing-practice-guidelines-chapter-4-annex_en |
| — | ECA Academy, "Annex 11 Draft – First Analysis" | https://www.gmp-compliance.org/gmp-news/annex-11-draft-first-analysis |
| — | qmsdoc.com, "Revision of PIC/S GMP Annex 11" (timeline) | https://qmsdoc.com/2026/01/15/revision-of-pic-s-gmp-annex-11-computerised-systems/ |
| — | Epista, "Preparing for EMA's 2026 GMP revisions" | https://www.epista.com/knowledge/preparing-for-emas-2026-good-manufacturing-practice-gmp-revisions-chapter-4-annex-11-annex-22 |
| — | SimplerQMS, "21 CFR Part 11 vs EU Annex 11" | https://simplerqms.com/21-cfr-part-11-vs-eu-annex-11/ |
