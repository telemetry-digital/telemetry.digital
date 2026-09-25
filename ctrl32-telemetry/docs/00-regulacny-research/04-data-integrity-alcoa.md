# Data Integrity (ALCOA+), Audit Trails, Electronic Signatures and Record Retention for a Computerised Temperature Monitoring System (EU/SK)

> **METHOD AND RELIABILITY NOTE — READ FIRST.**
> Direct file download (`curl`) to every regulatory host was blocked by this session's egress policy (403 CONNECT denials for `picscheme.org`, `health.ec.europa.eu`, `eur-lex.europa.eu`, `assets.publishing.service.gov.uk`, `iris.who.int`, `ecfr.gov`, `raw.githubusercontent.com` — in fact *all* hosts). The only working channel was the page-fetch tool, which passes documents through a summarising model that (a) caps individual quote length and (b) in at least one verified instance **invented decimal clause numbers that do not exist in the source**.
> Every quote below is therefore tagged:
> - **[V]** = verified/high confidence — wording reproduced consistently across independent fetches and/or matches the well-known published text of the instrument.
> - **[U]** = unverified — the wording is plausible and came from a fetch of the primary document, but I could not corroborate the exact wording or the clause number. **Do not paste [U] text into a validation document or an inspection response without checking the PDF.**
> One concrete failure worth knowing: a fetch returned tidy "clauses 9.6.1 … 9.6.7" of PIC/S PI 041-1. A later structural check of the same document established that **PI 041-1 section 9.6 is not numbered that way at all** — sections 9.6 and 9.7 are Item/Expectation/Potential-risk **tables**, not decimal sub-clauses. Those fabricated numbers are excluded from these notes and flagged in the Gaps.

---

## 1. PIC/S PI 041-1 (1 July 2021): ALCOA and ALCOA+ definitions, audit trails, data criticality and risk

### Takeaway
PI 041-1 places the ALCOA+ definitions in **section 7 "General data integrity principles and enablers"** (the element definitions sit at **7.5**, introduced at 7.4), and treats computerised systems in **section 9**, where audit trails are **9.6**, data capture/entry **9.7**, review of data **9.8**, and storage/archival/disposal **9.9**. Section 9 is written as expectation tables, not as decimal clauses — an important point when citing it.

### Cited Findings

**Confirmed document structure (this is the anchor for all PI 041-1 citations)** — [PIC/S PI 041-1, 1 July 2021](https://picscheme.org/docview/4234):
- 5 DATA GOVERNANCE SYSTEM — 5.1 What is data governance?; 5.2 Data governance systems; 5.3 Risk management approach to data governance; **5.4 Data criticality**; **5.5 Data risk**; 5.6 Data governance system review **[V]**
- 6 ORGANISATIONAL INFLUENCES ON SUCCESSFUL DATA INTEGRITY MANAGEMENT (6.1–6.7) **[V]**
- **7 GENERAL DATA INTEGRITY PRINCIPLES AND ENABLERS** (no sub-headings in the contents list; numbered paragraphs 7.1 … 7.x) **[V]**
- 8 SPECIFIC DATA INTEGRITY CONSIDERATIONS FOR PAPER-BASED SYSTEMS — 8.1 … 8.7 Making corrections on records; 8.8 Verification of records (secondary checks); 8.9 Direct print-outs from electronic systems; **8.10 Document retention (Identifying record retention requirements and archiving records)**; 8.11 Disposal of original records or true copies **[V]**
- **9 SPECIFIC DATA INTEGRITY CONSIDERATIONS FOR COMPUTERISED SYSTEMS** — 9.1 Structure of the Pharmaceutical Quality System and control of computerised systems; 9.2 Qualification and validation of computerised systems; 9.3 Validation and Maintenance; **9.4 Data Transfer**; **9.5 System security for computerised systems**; **9.6 Audit trails for computerised systems**; **9.7 Data capture/entry for computerised systems**; **9.8 Review of data within computerised systems**; **9.9 Storage, archival and disposal of electronic data**; 9.10 Management of Hybrid Systems **[V]**
- 10 outsourced activities; 11 regulatory actions; 12 remediation; 13 Glossary; 14 Revision history **[V]**

**ALCOA+ element definitions — PI 041-1 §7.5** (§7.4 introduces the acronym: the ALCOA acronym summarises the key concepts, and "Complete, Consistent, Enduring and Available" extend this to ALCOA+) — [PIC/S PI 041-1](https://picscheme.org/docview/4234). These nine definitions were returned **identically across three independent fetches**, which is why they carry **[V]**:

| Element | Verbatim definition (PI 041-1 §7.5) | Conf. |
|---|---|---|
| **Attributable** | "It should be possible to identify the individual or computerised system that performed a recorded task and when the task was performed." | **[V]** |
| **Legible** | "All records should be legible – the information should be readable and unambiguous in order for it to be understandable and of use." | **[V]** |
| **Contemporaneous** | "The evidence of actions, events or decisions should be recorded as they take place." | **[V]** |
| **Original** | "The original record can be described as the first-capture of information, whether recorded on paper (static) or electronically." | **[V]** |
| **Accurate** | "Records need to be a truthful representation of facts to be accurate." | **[V]** |
| **Complete** | "All information that would be critical to recreating an event is important when trying to understand the event." | **[V]** |
| **Consistent** | "Information should be created, processed, and stored in a logical manner that has a defined consistency." | **[V]** |
| **Enduring** | "Records should be kept in a manner such that they exist for the entire period during which they might be needed." | **[V]** |
| **Available** | "Records should be available for review at any time during the required retention period." | **[V]** |

**Data criticality and data risk:**
- §5.4 Data criticality — the points to consider include "Which decision does the data influence?" and "What is the impact of the data to product quality or safety?" **[U]** — [PIC/S PI 041-1](https://picscheme.org/docview/4234)
- §5.5 Data risk — assessment should consider "the vulnerability of data to involuntary alteration, deletion, loss (either accidental or by security failure) or re-creation or deliberate falsification" **[U]** — [PIC/S PI 041-1](https://picscheme.org/docview/4234)
- §5.6.2 (data governance system review / self-inspection) should include "A risk-based sample of computerised system logs / audit trails to ensure that information of relevance to GMP/GDP activity is reported accurately." **[U, but returned twice independently with identical wording — treat as medium-high]** — [PIC/S PI 041-1](https://picscheme.org/docview/4234)

**Computerised system data, raw data and metadata (section 9.1 area):**
- "In dealing with raw data, the complete capture and retention of raw data would normally be required in order to reconstruct the manufacturing event or analysis." **[U]** — [PIC/S PI 041-1](https://picscheme.org/docview/4234)
- On metadata: some metadata is critical to the reconstruction of events and is "relevant metadata" that should be fully captured and managed; examples given include "user identification, times, critical process parameters". **[U]** — [PIC/S PI 041-1](https://picscheme.org/docview/4234)
- §9.4 Data Transfer: "Interfaces should be assessed and addressed during validation to ensure the correct and complete transfer of data." **[U]** — [PIC/S PI 041-1](https://picscheme.org/docview/4234)
- §9.5 System security: "There should also be restrictions to prevent users from amending audit trail functions and from changing any pre-defined directory paths where data files are to be stored." **[U]** — [PIC/S PI 041-1](https://picscheme.org/docview/4234)
- §8.10 (retention): "The retention period of each type of records should (at a minimum) meet those periods specified by GMP/GDP requirements." **[U]** — [PIC/S PI 041-1](https://picscheme.org/docview/4234)

### Inferences
- The ALCOA+ definitions in §7.5 are medium-agnostic and written in terms of *tasks* and *events*, not of humans. "Attributable" explicitly contemplates that the actor may be **"the individual or computerised system that performed a recorded task"** — this is the single most useful sentence for the sole trader's design, because it means a measurement attributed to *a sensor identity plus a decoder version*, rather than to a human user, is exactly what the guidance envisages.
- "Contemporaneous" is defined as recording evidence "as they take place" — note it is about *when the evidence is recorded*, not about when it reaches the server. See §7 below on backfill.
- Because section 9 is table-structured (Item / Expectation / Potential risk of not meeting expectations), the defensible way to cite it is **"PI 041-1 §9.6, item N"** or simply **"PI 041-1 §9.6"** — never "PI 041-1 clause 9.6.3".

### Gaps
- **I could not retrieve a trustworthy verbatim rendering of the §9.6 audit trail expectation table.** One fetch produced seven neatly numbered "clauses 9.6.1–9.6.7"; a subsequent structural check of the same PDF established the section is a table with Item numbers, so those quotes are discarded as probable fabrications. The user must open the PDF at §9.6 and transcribe the Item/Expectation cells himself.
- I could not confirm whether PI 041-1 contains introductory prose before the §9.6 table (one fetch asserted it does not; this conflicts with the widely reproduced PI 041-1 audit-trail definition, so the point is unresolved).
- PI 041-1 clause numbering for the §5.4/§5.5/§8.10/§9.x quotes above is **not independently confirmed**.

---

## 2. MHRA "GXP Data Integrity Guidance and Definitions" (March 2018)

### Takeaway
MHRA's guidance is the most *operationally specific* of the EU-sphere documents and is the best single source for the user's design questions: it explicitly requires audit trails to be **switched on and not switchable off by users**, requires **synchronised clocks including time zone**, requires **risk-based audit trail review as part of routine data review**, and distinguishes archive from backup. Section numbering (3 principles, 4 criticality/risk, 5 system design, 6.1–6.20 definitions) is consistent across fetches and matches the published document.

**Source:** MHRA, *'GXP' Data Integrity Guidance and Definitions*, Revision 1, March 2018 (page last updated 27 September 2021; PDF, 445 KB, 21 pages) — [gov.uk landing page](https://www.gov.uk/government/publications/guidance-on-gxp-data-integrity) / [PDF](https://assets.publishing.service.gov.uk/media/5aa2b9ede5274a3e391e37f3/MHRA_GxP_data_integrity_guide_March_edited_Final.pdf)

### Cited Findings

**Principles and design (sections 3–5):**
- §3 (principles): organisations must ensure data is "complete, consistent and accurate in all its forms, i.e. paper and electronic"; systems should be "designed, operated and, where appropriate, adapted to support a suitable working environment". **[U]**
- §3.10: "ALCOA being Attributable, Legible, Contemporaneous, Original, and Accurate", with the "+" attributes Complete, Consistent, Enduring and Available. **[V]**
- **§5 (designing systems and processes) — CLOCKS:** systems should provide "**access to appropriately controlled/synchronised clocks for recording timed events**" to ensure reconstruction and traceability, "knowing and specifying the time zone where this data is used across multiple sites". **[V — returned twice, independently, with the same core phrase]**
- §5 (access): "User access rights that prevent (or audit trail, if prevention is not possible) unauthorised data amendments." **[U]**

**Definitions (section 6):**
- **§6.1 Data:** "Facts, figures and statistics collected together for reference or analysis." **[U]**
- **§6.2 Raw data:** "The original record (data) which can be described as the first-capture of information" — i.e. MHRA equates raw data with the original record / first capture. **[V]**
- **§6.4 Data Integrity:** "The degree to which data are complete, consistent, accurate, trustworthy, reliable…" **[U]**
- **§6.6 Data lifecycle:** "All phases in the life of the data from generation through processing, use, retention, archive/retrieval and destruction." **[V]**
- **§6.8 Data transfer / migration:** "The process of transferring data between different data storage types, formats, or computerised systems." **[V]**
- **§6.9 Data processing:** "A sequence of operations performed on data to extract, present or obtain information in a defined format." Plus: "There should be adequate traceability of any user-defined parameters used within data processing activities to the raw data, including attribution to who performed the activity." And: "If data processing has been repeated with progressive modification of processing parameters this should be visible to ensure that the processing parameters are not being manipulated to achieve a more desirable result." **[V]**
- **§6.10 Excluding data:** "Data may only be excluded where it can be demonstrated through valid scientific justification that the data are not representative of the quantity measured, sampled or acquired." And: "All data (even if excluded) should be retained with the original data set, and be available for review in a format that allows the validity of the decision to exclude the data to be confirmed." **[V]**
- **§6.11 Original record / true copy:**
  - Original record: "The first or source capture of data or information e.g. original paper record of manual observation or electronic raw data file from a computerised system." **[V]**
  - True copy: "A copy (irrespective of the type of media used) of the original record that has been verified (i.e. by a dated signature or by generation through a validated process) to have the same information, including data that describe the context, content, and structure, as the original." **[V]**
  - "True copies of original records may be retained in place of the original record (e.g. scan of a paper record), if a documented system is in place to verify and record the integrity of the copy." **[V]**
- **§6.12 Computerised system transactions:** "A single operation or sequence of operations performed as a single logical 'unit of work'." **[U]**
- **§6.13 Audit trail — the key clause:**
  - "The audit trail is a form of metadata containing information associated with actions that relate to the creation, modification or deletion of GXP records." **[V]**
  - "**Audit trails (identified by risk assessment as required) should be switched on. Users should not be able to amend or switch off the audit trail.**" **[V]**
  - "Where a system administrator amends, or switches off the audit trail a record of that action should be retained." **[V]**
  - "**Routine data review should include a documented audit trail review where this is determined by a risk assessment.**" **[V]**
  - "Reviewers should have sufficient knowledge and system access to review relevant audit trails, raw data and metadata." **[V]**
- **§6.15 Data review and approval:**
  - "There should be a procedure that describes the process for review and approval of data. Data review should also include a risk-based review of relevant metadata, including relevant audit trails records." **[V]**
  - "Data review should be documented and the record should include a positive statement regarding whether issues were found or not, the date that review was performed and the signature of the reviewer." **[V]**
  - Procedures "should enable data corrections or clarifications to provide visibility of the original record, and traceability of the correction, using ALCOA principles." **[V]**
- **§6.17 Data retention:** "Data retention may be for archiving (protected data for long-term storage) or backup (data for the purposes of disaster recovery)." **[V]**
  - **§6.17.1 Archive:** "A designated secure area or facility (e.g. cabinet, room, building or computerised system) for the long term, retention of data and metadata for the purposes of verification of the process or activity." **[U]**
  - **§6.17.2 Backup:** "A copy of current (editable) data, metadata and system configuration settings maintained for recovery including disaster recovery." **[U]**; "Backup and recovery processes should be validated and periodically tested. Each back up should be verified to ensure that it has functioned correctly." **[V]**; "**Backups for recovery purposes do not replace the need for the long term, retention of data and metadata in its final form.**" **[V]**

All MHRA quotes above — [MHRA GXP Data Integrity Guidance and Definitions, March 2018](https://assets.publishing.service.gov.uk/media/5aa2b9ede5274a3e391e37f3/MHRA_GxP_data_integrity_guide_March_edited_Final.pdf)

### Inferences
- MHRA §6.13 answers three of the user's questions in one clause: audit trails **must be on**, **users must not be able to amend or disable them**, and **administrator disabling must itself be recorded**. An append-only PostgreSQL design where the application role has no `UPDATE`/`DELETE` privilege on the measurement and audit tables is the strongest possible implementation of that sentence.
- MHRA §6.17.2's "backups … do not replace the need for the long term retention" is a direct argument against a design where the only long-term copy of raw measurements is a nightly `pg_dump`.
- MHRA §6.9's traceability-of-processing-parameters requirement maps almost exactly onto **versioned decoders**: the decoder version is a "user-defined parameter used within data processing activities", and §6.9 requires traceability from it back to raw data and attribution of who performed the activity. Keeping the raw payload plus the decoder version ID satisfies this better than storing only the decoded value.
- §6.10 ("all data, even if excluded, should be retained with the original data set") is the guidance basis for never hard-deleting a measurement that is later judged spurious — flag it, don't delete it.

### Gaps
- MHRA is a UK (post-Brexit) regulator. It has no legal force in Slovakia. It is nevertheless routinely cited by EU inspectors and by ISPE/GAMP as the reference interpretation; the report should present it as **persuasive, not binding** in the EU.
- I could not confirm the exact numbering of the §3.x principle sub-clauses (one fetch gave "3.1–3.2, 3.10", another only "3.10").

---

## 3. EudraLex Volume 4 Annex 11 (current, 2011) — clause 9 audit trails and neighbouring clauses

### Takeaway
The legally binding EU text is **much weaker** than MHRA or PIC/S: Annex 11 clause 9 makes the audit trail subject to a **risk assessment** ("Consideration should be given, based on a risk assessment…"), requires a **reason for change or deletion** to be documented, and requires audit trails to be **available, convertible to an intelligible form, and regularly reviewed**. Clause 12.4 separately mandates who/when recording for data entry, change, confirmation and deletion.

**Source:** [EudraLex Vol. 4, Annex 11 Computerised Systems (effective 30 June 2011)](https://health.ec.europa.eu/system/files/2016-11/annex11_01-2011_en_0.pdf)

### Cited Findings (all **[V]** — these match the published Annex 11 text)
- **Clause 4.8 (validation — data migration):** "If data are transferred to another data format or system, validation should include checks that data are not altered in value and/or meaning during this migration process."
- **Clause 5 (Data):** "Computerised systems exchanging data electronically with other systems should include appropriate built-in checks for the correct and secure entry and processing of data, in order to minimize the risks."
- **Clause 6 (Accuracy checks):** "For critical data entered manually, there should be an additional check on the accuracy of the data. This check may be done by a second operator or by validated electronic means."
- **Clause 7.1 (Data storage):** "Data should be secured by both physical and electronic means against damage. Stored data should be checked for accessibility, readability and accuracy."
- **Clause 7.2 (Backup):** "**Regular back-ups of all relevant data should be done. Integrity and accuracy of backup data and the ability to restore the data should be checked during validation and monitored periodically.**"
- **Clause 8 (Printouts):** "It should be possible to obtain clear printed copies of electronically stored data."
- **Clause 9 (Audit Trails):** "**Consideration should be given, based on a risk assessment, to building into the system the creation of a record of all GMP-relevant changes and deletions (a system generated 'audit trail').**" — the clause continues (standard published text, **[V] by knowledge, not re-fetched**) with the requirement that for change or deletion of GMP-relevant data **the reason should be documented**, and that **audit trails need to be available and convertible to a generally intelligible form and regularly reviewed**. *The fetch truncated after the first sentence; the user should confirm sentences 2–3 in the PDF.*
- **Clause 10 (Change and configuration management):** "Any changes to a computerised system including system configurations should only be made in a controlled manner in accordance with a defined procedure."
- **Clause 12.1 (Security):** "Physical and/or logical controls should be in place to restrict access to computerised system to authorised persons."
- **Clause 12.2:** "The extent of security controls depends on the criticality of the computerised system."
- **Clause 12.3:** "Creation, change, and cancellation of access authorisations should be recorded."
- **Clause 12.4:** "**Management systems for data and for documents should be designed to record the identity of operators entering, changing, confirming or deleting data including date and time.**"
- **Clause 13 (Incident management):** "All incidents, not only system failures and data errors, should be reported and assessed."
- **Clause 14 (Electronic signature):** "Electronic records may be signed electronically. Electronic signatures are expected to: a. have the same impact as hand-written signatures within the boundaries of the company, b. be permanently linked to their respective record, c. include the time and date that they were applied."
- **Clause 15 (Batch release):** "When a computerised system is used for recording certification and batch release, the system should allow only Qualified Persons to certify the release of the batches and it should clearly identify and record the person releasing or certifying the batches."
- **Clause 16 (Business continuity):** "For the availability of computerised systems supporting critical processes, provisions should be made to ensure continuity of support for those processes in the event of a system breakdown (e.g. a manual or alternative system)."
- **Clause 17 (Archiving):** "**Data may be archived. This data should be checked for accessibility, readability and integrity.** If relevant changes are to be made to the system (e.g. computer equipment or programs), then the ability to retrieve the data should be ensured and tested." *(final sentence per published text, not re-fetched)*

### Inferences
- **Backup/restore (user's question):** Annex 11 clause 7.2 is unambiguous — the **ability to restore** must be **checked during validation** *and* **monitored periodically**. A restore test that is performed once at go-live and never repeated does not meet clause 7.2. The test and its result must be documented (it is part of validation evidence).
- **Migration/retirement:** clause 4.8 (no alteration of value **or meaning**) plus clause 17 (archived data checked for accessibility, readability and integrity, retrieval tested after equipment/program changes) are the two clauses to cite in a retirement/migration plan. For versioned decoders, "meaning" is the operative word: migrating decoded values without the decoder version would alter meaning.
- **Audit trail:** the phrase "based on a risk assessment" in clause 9 is often misread as optional. For a system whose sole output is GxP-critical temperature data, the risk assessment has only one defensible outcome, and clause 12.4 independently requires identity + date and time for entering/changing/confirming/deleting data.

### Gaps
- The full second and third sentences of clause 9 were truncated by the fetch tool; I have reproduced them from knowledge and flagged them. Verify in the PDF before quoting.
- I could not retrieve the EMA GMP **Q&A on data integrity** (the dedicated Q&A page) — searches returned only third-party commentary and the FDA Q&A. **Reported as a gap; nothing invented.**

---

## 4. What must an audit trail capture; reviewability; independence; can it be switched off; "targeted" review

### Takeaway
Across the instruments the consensus content is **who / what (old value → new value) / when / why**, the audit trail must be **automatic, enabled, non-editable, and reviewable in an intelligible form**, review must be **risk-based, documented and part of routine data review**, and only an administrator action (itself recorded) may touch it. The 2025 draft Annex 11 hardens all of this from "consideration should be given" into a flat requirement.

### Cited Findings
- **Annex 11 (2011) cl. 12.4:** record "the identity of operators entering, changing, confirming or deleting data including date and time." **[V]** — [Annex 11](https://health.ec.europa.eu/system/files/2016-11/annex11_01-2011_en_0.pdf)
- **Annex 11 (2011) cl. 9:** a system-generated record of "all GMP-relevant changes and deletions"; reason for change/deletion documented; audit trails "available and convertible to a generally intelligible form and regularly reviewed". **[V for sentence 1; sentences 2–3 flagged above]**
- **MHRA §6.13:** "Audit trails (identified by risk assessment as required) should be switched on. Users should not be able to amend or switch off the audit trail." / "Where a system administrator amends, or switches off the audit trail a record of that action should be retained." / "Routine data review should include a documented audit trail review where this is determined by a risk assessment." **[V]** — [MHRA](https://assets.publishing.service.gov.uk/media/5aa2b9ede5274a3e391e37f3/MHRA_GxP_data_integrity_guide_March_edited_Final.pdf)
- **WHO TRS 996 Annex 5, Glossary:** an audit trail is "a form of metadata that contains information associated with actions that relate to the creation, modification or deletion of GXP records"; it "facilitates the reconstruction of the history of such events … including the '**who, what, when and why**' of the action." **[V]** — [WHO TRS 996 Annex 5](https://rx-360.org/wp-content/uploads/2018/08/WHO_TRS_996_annex05.pdf)
- **WHO TRS 996 Annex 5, Appendix 1:** written procedures must define "the frequency, roles and responsibilities and approach to review of meaningful metadata, such as audit trails." **[U]**; §11.11: "The approach to reviewing specific record content, such as critical data fields and metadata … should meet all applicable regulatory requirements and be risk-based." **[U]** — [WHO TRS 996 Annex 5](https://rx-360.org/wp-content/uploads/2018/08/WHO_TRS_996_annex05.pdf)
- **WHO TRS 996 Annex 5, Appendix 1 (independence):** access to enhanced permissions should be "strictly controlled" and "restricted only to persons independent of those responsible for the content of electronic records." **[U]** — [WHO](https://rx-360.org/wp-content/uploads/2018/08/WHO_TRS_996_annex05.pdf)
- **Draft Annex 11 (2025 public consultation), section 12 "Audit Trails"** — [European Commission consultation document](https://health.ec.europa.eu/document/download/40231f18-e564-4043-94de-c031f813d38b_en?filename=mp_vol4_chap4_annex11_consultation_guideline_en.pdf):
  - **12.1:** "Systems which are used to control processes, capture, hold or report data … should have an audit trail functionality which automatically logs all manual user interactions." **[U]**
  - **12.2:** "The audit trail should unambiguously capture the user who made a change … what was changed … and the date and time when the change was made." **[U]**
  - **12.3:** "**Audit trail functionality should be enabled and locked at all times, and it should not be possible for any user to edit audit trail data.**" **[U]**
  - **12.5:** "Audit trail reviews should be conducted according to a documented procedure for the specific system, or type of systems." **[U]**
  - **12.8:** "Audit trail reviews should be conducted in a timely manner according to the risk of the process reviewed … prior to batch release." **[U]**
- **FDA DI Q&A (Dec 2018), Q1.c:** "An 'audit trail' means a secure, computer-generated, time-stamped electronic record that allows for reconstruction of the course of events relating to the creation, modification, or deletion of an electronic record." **[V]** — [FDA, Data Integrity and Compliance With Drug CGMP: Questions and Answers](https://www.fda.gov/media/119267/download)
- **FDA DI Q&A Q7/Q8:** personnel performing record review should examine audit trails capturing changes to data as they assess the complete record; the quality unit reviews and approves production and control records including audit trails. Review frequency should follow the CGMP requirement where one is specified (e.g. before batch release) and otherwise be set by risk assessment considering data criticality and impact on product quality. **[U]** — [FDA](https://www.fda.gov/media/119267/download)
- **21 CFR 11.10(e)** (US comparator) requires "use of secure, computer-generated, time-stamped audit trails to independently record the date and time of operator entries and actions that create, modify, or delete electronic records"; record changes "shall not obscure previously recorded information" and audit trail documentation "shall be retained for a period at least as long as that required for the subject electronic records and shall be available for agency review and copying." **[V by knowledge — see Gaps: I could not fetch eCFR]**

**"Targeted audit trail review":** the term does not appear as a defined term in any of the primary documents I could reach. The nearest concepts are (a) MHRA §6.13 "documented audit trail review where this is determined by a risk assessment", (b) PI 041-1 §5.6.2's "risk-based sample of computerised system logs / audit trails", (c) draft Annex 11 12.5/12.8's documented, risk-and-timing-based review, and (d) WHO §11.11's risk-based approach to "critical data fields and metadata". **[V that the concept is risk-based sampling; [U] that any document uses the phrase "targeted audit trail review".]**

### Inferences
- For an append-only measurement store, the honest design position is: **measurements are never changed, so the measurement audit trail is degenerate** — the audit-trail obligation bites instead on (i) configuration changes (alarm limits, calibration offsets, sensor↔location assignments, decoder versions), (ii) user/access changes, (iii) excursion acknowledgements and their justifications, and (iv) any administrative action. That is where "who/what/when/why" must be captured, and that is what "routine data review" should cover. Saying this explicitly in the validation documentation is much stronger than claiming "we have an audit trail".
- "Why" (reason for change) is required by Annex 11 cl. 9 only for **change or deletion of GMP-relevant data**. In an append-only system there is no change or deletion of measurements, so the *mandatory* reason-for-change field belongs on configuration and acknowledgement actions — which is a defensible and inspectable design decision.
- Independence of the audit trail from the operator is supported by WHO Appendix 1 and by PI 041-1 §9.5's restriction on users amending audit trail functions; in a single-person business ("sole trader"), true segregation of duties is impossible, so the compensating control is **technical**: a database role without DELETE/UPDATE on audit tables, plus documented acknowledgement that the administrator is also the user, plus periodic review evidence. Inspectors expect that compensating-control argument to be written down.

### Gaps
- No primary source I could reach uses the exact phrase "targeted audit trail review"; I did not find a regulator-defined meaning for it.
- I could not fetch 21 CFR Part 11 text (eCFR blocked by fetch-permission policy). The §11.10(e) wording above is from knowledge and must be verified.

---

## 5. Electronic signatures: Annex 11 cl. 14, the July 2025 draft, and 21 CFR Part 11 subpart C

### Takeaway
Current Annex 11 cl. 14 sets three loose expectations (same impact as handwritten within the company, permanently linked to the record, include time and date). The **2025 draft moves section 13 much closer to 21 CFR Part 11**: full re-authentication at the moment of signing, automatic logging of date/time **and time zone**, a prompt for the **meaning** of the signature, and a permanent link with controls ensuring a signed record cannot be modified.

### Cited Findings
- **Annex 11 (2011) clause 14:** "Electronic records may be signed electronically. Electronic signatures are expected to: a. have the same impact as hand-written signatures within the boundaries of the company, b. be permanently linked to their respective record, c. include the time and date that they were applied." **[V]** — [Annex 11](https://health.ec.europa.eu/system/files/2016-11/annex11_01-2011_en_0.pdf)
- **Draft Annex 11 (2025), section 13 "Electronic Signatures"** — [EC consultation document](https://health.ec.europa.eu/document/download/40231f18-e564-4043-94de-c031f813d38b_en?filename=mp_vol4_chap4_annex11_consultation_guideline_en.pdf):
  - **13.3:** "**When executing an electronic signature, a system should enforce users to perform a full re-authentication providing at least the same level of security as during system login.**" **[U]**
  - **13.4:** "Systems should automatically log the date and time and, where applicable, the **time zone** when an electronic signature was applied." **[U]**
  - **13.5:** "It should be clear when a user is executing an electronic signature and where applicable, systems should prompt the user for the **meaning** of the signature." **[U]**
  - **13.8:** "**Electronic signatures should be permanently linked to their respective records. Controls should be in place to ensure that a signed record cannot be modified.**" **[U]**
  - Secondary commentary on the same draft describes the effect as: first signing in a session = full re-authentication, subsequent signings = password/biometric; smart-card-only signing not acceptable; if a record is altered after signing the signature is voided/flagged; the system must ask the user for the purpose of the signature (approve, review, …); time zone must be captured. **[U — this is an author's paraphrase, not draft text]** — [Investigations of a Dog, "Draft Annex 11, Section 13", 29 July 2025](https://investigationsquality.com/2025/07/29/draft-annex-11-section-13-what-the-proposed-electronic-signature-rules-mean/)
  - **Draft §2.4 (data integrity framing):** "As defined by the ALCOA+ principles, data integrity covers many topics including but not limited to requirements defined in the sections Handling of Data, Identity and Access Management, Audit Trails, Electronic Signatures, and Security." **[U]**
  - **Draft glossary, ALCOA+:** "An acronym for 'attributable, legible, contemporaneous, original and accurate', which puts additional emphasis on the attributes of being complete, consistent, enduring and available." **[U]**
- **21 CFR Part 11 subpart C (comparator, from knowledge — see Gaps):** §11.100(a) each electronic signature shall be unique to one individual and not reused by, or reassigned to, anyone else; §11.100(b) identity verification before assigning a signature; §11.100(c) certification to FDA that electronic signatures are the legally binding equivalent of handwritten signatures. §11.200(a)(1) non-biometric signatures shall employ at least **two distinct identification components such as an identification code and password**; (a)(1)(i) for a series of signings during a single continuous period of controlled system access, the **first signing uses all components and subsequent signings use at least one component that is executable only by, and designed to be used only by, the individual**; (a)(1)(ii) signings **not** in a single continuous session use all components; (a)(2) they shall be used only by their genuine owners; (a)(3) administration/use designed such that attempted use by anyone other than the genuine owner requires collaboration of two or more individuals. §11.200(b) biometric signatures shall be designed to ensure they cannot be used by anyone other than their genuine owners. §11.70 signature/record linking: electronic and handwritten signatures executed to electronic records shall be **linked to their respective electronic records to ensure that the signatures cannot be excised, copied, or otherwise transferred** to falsify an electronic record by ordinary means. §11.300 controls for identification codes/passwords (uniqueness, periodic revision/ageing, loss management, transaction safeguards to detect unauthorised use, initial and periodic testing of devices). **[U — all of subpart C reported from knowledge; the eCFR fetch was blocked]**
- **FDA DI Q&A Q11:** electronic signatures with appropriate controls may replace handwritten signatures provided they securely link the signature to the associated record and identify the specific signer. **[U]** — [FDA](https://www.fda.gov/media/119267/download)

### Inferences
- The **direction of travel** is convergence: draft Annex 11 §13.3's "full re-authentication providing at least the same level of security as during system login" is functionally the EU answer to 21 CFR 11.200(a)(1)(ii), and §13.8's "a signed record cannot be modified" is the EU answer to §11.70's "cannot be excised, copied, or otherwise transferred".
- For the Go/PostgreSQL system: if the software will ever carry a *signed* record (e.g. a signed excursion assessment or a signed monthly review), the defensible implementation today is (a) re-prompt for the password at the point of signing, (b) store username, full timestamp **with time zone**, and the signature *meaning* as a free-text or enumerated reason, and (c) bind the signature to a cryptographic hash of the exact record content so that any later modification is detectable. That design already satisfies both the current cl. 14 and the draft §13.3/13.4/13.5/13.8.
- Conversely, if the system does **not** implement electronic signatures at all and instead produces reports that are signed on paper (a hybrid), Annex 11 cl. 14 simply does not apply — but PI 041-1 §9.10 "Management of Hybrid Systems" then does. For a sole trader this is often the cheaper defensible route and should be presented as a genuine option.

### Gaps
- **I could not obtain the official draft Annex 11 numbering with certainty.** One fetch of the EC consultation document returned sections 10 (migration), 12 (audit trails), 13 (electronic signatures), 16 (backup), 17 (archiving); a third-party article independently refers to "Section 13" for electronic signatures, which corroborates the e-signature numbering but not the others. The consultation document I reached is titled in a way suggesting it may be the "reasons for changes"/consultation guideline rather than the draft annex itself.
- **Consultation dates for the draft Annex 11 were not confirmed.** The article is dated 29 July 2025; the draft's publication date and comment deadline, and whether a final revised Annex 11 has since been adopted (relevant as of September 2026), were **not** established. This is material — the report should not state the draft's current legal status without checking.
- 21 CFR Part 11 text could not be fetched; all subpart C detail above is unverified.

---

## 6. Record retention: GDP distribution records, GMP records, and Slovak national law

### Takeaway
The operative EU number for a **wholesale distributor** is **"the period stated in national legislation but at least five years"** (GDP Guidelines Chapter 4.2). For **GMP batch documentation** it is **one year after batch expiry or five years after QP certification, whichever is longer** (GMP Chapter 4). Temperature monitoring records are *supporting distribution/manufacturing records*, and follow the regime of the operation they evidence.

### Cited Findings
**GDP (distribution):**
- **Guidelines of 5 November 2013 on Good Distribution Practice of medicinal products for human use (2013/C 343/01), Chapter 4.1 Principle:** "Good documentation constitutes an essential part of the quality system. Written documentation should prevent errors from spoken communication and permits the tracking of relevant operations during the distribution of medicinal products." **[V]** — [EU GDP Guidelines 2013/C 343/01](https://health.ec.europa.eu/system/files/2016-11/2013_c343_01_en_0.pdf)
- **Chapter 4.2 General:** documentation comprises "all written procedures, instructions, contracts, records and data, in paper or in electronic form"; and — **the retention sentence** — "**Documents should be retained for the period stated in national legislation but at least five years.**" **[V — returned identically from two independent fetches of two different hosts]** — [EU GDP Guidelines](https://health.ec.europa.eu/system/files/2016-11/2013_c343_01_en_0.pdf); [same text via EUR-Lex OJ C 343/1](https://eur-lex.europa.eu/LexUriServ/LexUriServ.do?uri=OJ:C:2013:343:0001:0014:EN:PDF)
- Chapter 4 of the GDP Guidelines contains **only 4.1 (Principle) and 4.2 (General)** — there is no 4.3. **[U but stated explicitly by the fetch]**
- **Chapter 3.2.1 (Temperature and environment control):** "Suitable equipment and procedures should be in place to check the environment where medicinal products are stored"; environmental factors include "temperature, light, humidity and cleanliness of the premises"; "An initial temperature mapping exercise should be carried out on the storage area before use, under representative conditions"; "Temperature monitoring equipment should be located according to the results of the mapping exercise", positioned in the areas that experience the extremes of fluctuations; mapping should be repeated according to risk assessment or whenever significant modifications are made. **[U/V mixed — the mapping sentences were returned consistently twice]** — [EU GDP Guidelines](https://health.ec.europa.eu/system/files/2016-11/2013_c343_01_en_0.pdf)
- **Chapter 3.3 (Equipment):** equipment must be "designed, located and maintained to a standard which suits its intended purpose"; equipment used to control or monitor the environment should be "calibrated at defined intervals based on a risk and reliability assessment"; "**Appropriate alarm systems should be in place to provide alerts when there are excursions from pre-defined storage conditions.**" Records of repairs, maintenance and calibration must be retained. **[U/V mixed — alarm sentence returned twice]**
- **Chapter 9.2 (Transportation):** "The required storage conditions for medicinal products should be maintained during transportation within the defined limits as described by manufacturers"; risk assessment of delivery routes determines where temperature controls are required; temperature monitoring equipment in vehicles/containers should be maintained and calibrated **at least once a year**. **[U]**
- **Chapter 1.1 Principle:** "Wholesale distributors must maintain a quality system setting out responsibilities, processes and risk management principles in relation to their activities." **[U]**

**GMP (manufacturing):**
- **EudraLex Vol. 4 Chapter 4 (Documentation) — batch documentation retention:** batch records must be retained for "**one year after expiry of the batch to which it relates or at least five years after certification of the batch by the Qualified Person, whichever is the longer**"; for investigational medicinal products, batch documentation must be kept "for at least five years after the completion or formal discontinuation of the last clinical trial in which the batch was used". **[V — this is the well-known cl. 4.11 wording and was returned as such]** — [EU GMP Chapter 4](https://health.ec.europa.eu/system/files/2016-11/chapter4_01-2011_en_0.pdf)
- **GMP Chapter 4 on raw data:** "For electronic records regulated users should define which data are to be used as raw data. At least, all data on which quality decisions are based should be defined as raw data." **[V — this is the published Chapter 4 wording]**
- **GMP Chapter 4 on electronic documents:** "Appropriate controls for electronic documents such as templates, forms, and master documents should be implemented. Appropriate controls should be in place to ensure the integrity of the record throughout the retention period." **[U]**
- **Records definition:** records "provide evidence of various actions taken to demonstrate compliance with instructions", include "a history of each batch of product, including its distribution", and encompass "the raw data which is used to generate other records". **[U]**

**Directive 2001/83/EC:**
- Article 80 is the legal basis for wholesale distributor record-keeping; point (e) requires records of every transaction in medicinal products received/dispatched/brokered (date, name of the medicinal product, quantity received/supplied/brokered, name and address of supplier or consignee, and batch number at least for products bearing the safety features), and a subsequent point requires those records to be kept **available to the competent authorities for inspection purposes for a period of five years**. **[U — reported from knowledge; see Gaps]** — [Directive 2001/83/EC on EUR-Lex](https://eur-lex.europa.eu/eli/dir/2001/83/oj/eng)

**Slovakia:**
- The national act is **zákon č. 362/2011 Z. z. o liekoch a zdravotníckych pomôckach**, with wholesale-distributor obligations in **§ 18**. **[V that this is the correct act and that §18 covers wholesale obligations]** — [SLOV-LEX 362/2011 Z. z.](https://www.slov-lex.sk/pravne-predpisy/SK/ZZ/2011/362/); consolidated PDF hosted by ŠÚKL: [ZZ_2011_362](https://www.sukl.sk/buxus/docs/Inspekcia/Legislativa/ZZ_2011_362_20180101.pdf)

### Inferences
- **Where "five years" comes from, precisely:** it is *not* primarily a GDP-guideline invention. It flows from **Directive 2001/83/EC Article 80** (records available to competent authorities for five years), and the **GDP Guidelines Chapter 4.2** then restates it as a floor — "the period stated in national legislation **but at least five years**". The phrasing matters for a Slovak customer: if § 18 of 362/2011 Z. z. (or a ŠÚKL requirement) states a longer period, the longer period wins; five years is the EU minimum, not a ceiling.
- **Which record gets which period (design-relevant mapping):**
  - Temperature monitoring records at a *wholesale distributor* (warehouse, vehicle) → GDP Chapter 4.2 regime → **≥ 5 years**, subject to Slovak national law.
  - Temperature records that form part of *GMP batch documentation* (e.g. storage of a bulk or finished batch at a manufacturing site) → GMP Chapter 4 regime → **1 year after batch expiry or 5 years after QP certification, whichever is longer** — which for a long-dated product can substantially exceed five years.
  - Calibration certificates, equipment maintenance and qualification records → GDP Chapter 3.3 requires them to be retained; period follows the Chapter 4.2 floor.
  - Audit trail data → must be retained **at least as long as the underlying record** (explicit in 21 CFR 11.10(e); implicit in PI 041-1 §8.10 and Annex 11 cl. 17). Practical consequence: the audit trail cannot be pruned on a shorter schedule than the measurements.
- **Design consequence:** the system needs a *per-record-class* retention policy, not one global number, and because the GMP case is "expiry + 1 year", the retention clock for some records cannot be computed from the measurement timestamp alone — it depends on external batch data. The safest engineering position for a monitoring product is **retain everything, delete nothing automatically**, and let deletion be a deliberate, logged, human-authorised operation.

### Gaps
- **Verbatim text of Directive 2001/83/EC Article 80(e) and the five-year point could not be retrieved.** Six different attempts (EUR-Lex ELI, EUR-Lex CELEX HTML, EUR-Lex consolidated PDF, EMA-hosted PDF, legislation.gov.uk) all failed — either the fetch returned a different document, or the article fell outside the retrieved excerpt, or the fetch was refused. The Article 80 content above is from knowledge and **must be verified against EUR-Lex before use**.
- **Slovak § 18 of 362/2011 Z. z. could not be quoted.** SLOV-LEX returned only page metadata, the ŠÚKL PDF excerpt ended at § 15, and zakony.judikaty.info is disallowed by robots.txt. **I have no verified Slovak retention period.** The report must not assert a Slovak number; it should say the EU floor is five years and that § 18 must be checked directly on SLOV-LEX.
- I could not verify whether GDP Chapter 4.2 contains the additional sentence on personal data deletion/anonymisation that appears in some renderings.

---

## 7. Late-arriving / backfilled data, "contemporaneous" for automatic capture, and retrospective deviations

### Takeaway
**No guidance I could find addresses buffered/backfilled sensor data directly.** This is a genuine silence, and the report should say so plainly. The nearest applicable principles are WHO TRS 996's rule on **late entries** (record both the date of the activity and the date of the entry — never back-date), FDA's Q12 ("document, or save, the data **at the time of performance**"), and PI 041-1's definition of Contemporaneous as evidence "recorded as they take place".

### Cited Findings
- **PI 041-1 §7.5, Contemporaneous:** "The evidence of actions, events or decisions should be recorded as they take place." **[V]** — [PI 041-1](https://picscheme.org/docview/4234)
- **WHO TRS 996 Annex 5 §9.4:** "Contemporaneous data are data recorded at the time they are generated or observed." **[U]** — [WHO TRS 996 Annex 5](https://rx-360.org/wp-content/uploads/2018/08/WHO_TRS_996_annex05.pdf)
- **WHO TRS 996 Annex 5, Appendix 1 (special risk management considerations) — THE key sentence for backfill:** training should emphasise that "**it is unacceptable to backdate or forward date a record. Instead the date recorded should be the actual date of the data entry. Late entries should be indicated as such with both the date of the activity and the date of the entry being recorded.**" **[U — but this is the most directly on-point sentence found anywhere in the corpus]** — [WHO](https://rx-360.org/wp-content/uploads/2018/08/WHO_TRS_996_annex05.pdf)
- **WHO TRS 996 Annex 5, Appendix 1:** "it is unacceptable to record data first in unofficial documentation (e.g. on a scrap of paper) and later transfer the data to official documentation." **[U]**
- **WHO TRS 996 Annex 5 §4.12 (automatic capture):** controls should include "ensuring automated data capture or printers are attached and connected to equipment, such as balances, to ensure independent and timely recording of the data." **[U]**
- **WHO Appendix 1 (Contemporaneous, electronic):** "Where possible, data and time recording of manual activities (e.g. weighing) should be done automatically." **[U]**
- **FDA DI Q&A Q12:** "**You must document, or save, the data at the time of performance to create a record in compliance with CGMP requirements.**" Data may not be recorded on temporary papers and later transcribed to permanent records. **[V]** — [FDA](https://www.fda.gov/media/119267/download)
- **FDA DI Q&A Q1.b (metadata):** metadata is "contextual information required to understand data" and includes date/time stamps, user identification, instrument identification and audit trails necessary to reconstruct the activity. **[U]** — [FDA](https://www.fda.gov/media/119267/download)

### Inferences (flagged as inference — the guidance does not say this)
- **The two-timestamp model is the defensible design, and WHO Appendix 1 is the authority to cite for it.** WHO's late-entry rule — "both the date of the activity and the date of the entry being recorded" — is written for paper, but it maps one-to-one onto a schema that stores **`measured_at` (the logger's onboard timestamp = the date of the activity)** and **`received_at` / `ingested_at` (the server receipt = the date of the entry)**, with the pair immutable and both visible on any report. Storing only one of the two is what the rule forbids.
- **"Contemporaneous" is satisfied at the point of capture, not at the point of ingestion.** PI 041-1 §7.5 and WHO §9.4 both define contemporaneity by reference to *when the data is generated or observed*. A logger with onboard memory that timestamps at the sensor and later uploads is therefore recording contemporaneously; the network outage delays *transmission*, not *recording*. This is a strong, principled argument — but it depends entirely on the **logger's own clock being trustworthy and controlled**, which is why clock synchronisation (section 8 below) is the load-bearing control for this whole argument. The user should document this as an explicit risk-assessment conclusion.
- **Backfill must never overwrite or silently interleave.** Append-only insertion of late records, with the `received_at` preserved, means the audit picture reconstructs correctly; a design that re-ordered or de-duplicated by overwriting would breach the "original record"/"first capture" principle (PI 041-1 §7.5 Original; MHRA §6.2, §6.11).
- **Retrospectively discovered excursion = a deviation from the moment it is discovered.** There is no guidance text on this specific scenario, but the applicable chain is: GDP Ch. 3.3 requires alarms on excursions; GDP Ch. 4 requires the record; the quality system (GDP Ch. 1) requires deviations to be investigated. The practical position: the *excursion* is dated to `measured_at`, the *deviation record* is dated to the discovery, and the gap between the two is itself a data point the investigation must explain (why was the alarm not raised in real time?). A monitoring system should therefore make "excursion detected on backfilled data" a distinct, visible event type rather than folding it into normal alarms.
- **Gap-detection is an unstated but implied requirement.** Nothing in the guidance says "detect missing data", but ALCOA+ **Complete** ("All information that would be critical to recreating an event", PI 041-1 §7.5) means a period with no data must be *visible as a gap* rather than invisible. A system that silently shows an unbroken line across a four-hour outage is not Complete. Reporting gaps explicitly — and then closing them when backfill arrives — is the direct implementation of "Complete".

### Gaps
- **No regulator document I could reach discusses data loggers with onboard memory, buffered upload, store-and-forward, or backfill after a network outage.** I searched the PIC/S, MHRA, WHO, Annex 11, draft Annex 11 and FDA corpus. This is a real silence, not a search failure — but note I could not fully read PI 041-1 §9.7 "Data capture/entry for computerised systems", which is the single most likely place for such a statement to appear. **That section should be read directly before concluding the guidance is silent.**
- ISPE GAMP "Records and Data Integrity" Guide (and its "Data Integrity by Design" companion) was on the suggested source list but is **paywalled/not publicly retrievable**; I obtained nothing from it. It is the most likely source of practical guidance on buffered instrument data and should be flagged as worth purchasing.
- No guidance found on how to treat a retrospectively discovered excursion specifically.

---

## 8. Time: system clocks, time zones, synchronisation, trusted time, DST

### Takeaway
Time control is explicitly required, and in unusually concrete terms: MHRA requires **controlled/synchronised clocks with a specified time zone**; WHO requires **restricting the ability to change any clock used for recording timed events**, **secure time/date stamps that cannot be altered by personnel**, **maintenance programmes that keep stamps synchronised across GXP operations**, and **time zone controls**. The 2025 draft Annex 11 adds time-zone logging for signatures. **Daylight saving time is nowhere addressed.**

### Cited Findings
- **MHRA §5:** systems should provide "**access to appropriately controlled/synchronised clocks for recording timed events**", including "knowing and specifying the time zone where this data is used across multiple sites". **[V — returned twice independently]** — [MHRA](https://assets.publishing.service.gov.uk/media/5aa2b9ede5274a3e391e37f3/MHRA_GxP_data_integrity_guide_March_edited_Final.pdf)
- **WHO TRS 996 Annex 5 §4.12:** controls should include "**restricting the ability to change any clock used for recording timed events, for example, system clocks in electronic systems**". **[U]** — [WHO](https://rx-360.org/wp-content/uploads/2018/08/WHO_TRS_996_annex05.pdf)
- **WHO TRS 996 Annex 5, Appendix 1 (Contemporaneous, electronic records):** electronic records require "**secure system time/date stamps that cannot be altered by personnel**" and "**procedures and maintenance programmes that ensure time/date stamps are synchronized across the GXP operations**". **[U]** — [WHO](https://rx-360.org/wp-content/uploads/2018/08/WHO_TRS_996_annex05.pdf)
- **WHO TRS 996 Annex 5 §4.13:** organisations should address "controls that allow for the determination of the timing of one activity relative to another (e.g. **time zone controls**)". **[U]** — [WHO](https://rx-360.org/wp-content/uploads/2018/08/WHO_TRS_996_annex05.pdf)
- **Draft Annex 11 (2025) §13.4:** "Systems should automatically log the date and time and, where applicable, the time zone when an electronic signature was applied." **[U]** — [EC consultation document](https://health.ec.europa.eu/document/download/40231f18-e564-4043-94de-c031f813d38b_en?filename=mp_vol4_chap4_annex11_consultation_guideline_en.pdf)
- **Annex 11 (2011) cl. 12.4:** identity of operators "including date and time" must be recorded. **[V]** — [Annex 11](https://health.ec.europa.eu/system/files/2016-11/annex11_01-2011_en_0.pdf)
- **21 CFR 11.10(e)** requires "secure, computer-generated, **time-stamped** audit trails". **[U — from knowledge, eCFR unfetchable]**
- **PI 041-1:** one fetch stated "No explicit clause found addressing clock synchronisation or time zone management" in PI 041-1. **[U — an absence claim from a summarising model is weak evidence; treat as unconfirmed]** — [PI 041-1](https://picscheme.org/docview/4234)

### Inferences
- The four WHO/MHRA requirements translate directly into concrete, defensible engineering decisions the user can document:
  1. **"Restricting the ability to change any clock"** → the application must never accept a client-supplied wall-clock for the authoritative `received_at`; the server clock is disciplined by NTP, the NTP configuration is part of the qualified infrastructure, and no application user has the OS privilege to set the clock.
  2. **"Secure time/date stamps that cannot be altered by personnel"** → `received_at` written by the database (`now()` / `clock_timestamp()` in an append-only insert), not by application code and never client-supplied.
  3. **"Synchronized across the GXP operations"** → logger clock drift is a monitored, periodically corrected parameter with a documented tolerance; drift correction is itself an audited configuration event.
  4. **"Time zone controls"** → store everything as `timestamptz` (UTC internally), render with explicit zone, and never store a naive local timestamp.
- **DST is the practical trap and the guidance does not cover it.** In Europe/Bratislava, 02:00–03:00 on the October changeover occurs twice in local time. A system storing local naive time makes two distinct hours indistinguishable — a direct breach of ALCOA+ **Consistent** ("stored in a logical manner that has a defined consistency", PI 041-1 §7.5) and arguably of **Attributable** (§7.5 requires identifying "when the task was performed"). Storing UTC with zone rendering removes the problem entirely. This is an inference from principles, not a quoted requirement, and should be presented as such — but it is the sort of finding an inspector recognises immediately.
- **"Trusted time source" is not a defined regulatory term** in any document I reached. The regulatory expectation is *controlled and synchronised*, not *cryptographically attested*. So NTP to a documented, reliable source plus periodic verification records meets the bar; RFC 3161 trusted timestamping is a bonus, not a requirement. Worth saying explicitly, because it saves the user from over-engineering.

### Gaps
- **No guidance found anywhere on daylight saving time for GxP records.** Genuine silence.
- No guidance found specifying an acceptable clock drift tolerance, an NTP stratum, or a re-synchronisation frequency.
- Whether PI 041-1 contains any clock/time-zone requirement is unresolved (one negative claim from a summarising model, not trustworthy).

---

## 9. Backup and restore; migration and archiving on retirement; deletion; raw vs calibrated value

### Takeaway
Backup/restore testing is a hard requirement (Annex 11 cl. 7.2, "checked during validation and monitored periodically"; draft Annex 11 §16.6 adds "tested and documented … after changes"). Archiving on retirement is governed by Annex 11 cl. 17 and cl. 4.8. On **raw vs calibrated value the guidance does not resolve the sensor case**, but GMP Chapter 4's "regulated users should define which data are to be used as raw data" hands the decision to the user — provided he documents it.

### Cited Findings

**Backup and restore:**
- **Annex 11 cl. 7.2:** "Regular back-ups of all relevant data should be done. **Integrity and accuracy of backup data and the ability to restore the data should be checked during validation and monitored periodically.**" **[V]** — [Annex 11](https://health.ec.europa.eu/system/files/2016-11/annex11_01-2011_en_0.pdf)
- **MHRA §6.17.2:** "Backup and recovery processes should be validated and periodically tested. Each back up should be verified to ensure that it has functioned correctly." / "Backups for recovery purposes do not replace the need for the long term, retention of data and metadata in its final form." **[V]** — [MHRA](https://assets.publishing.service.gov.uk/media/5aa2b9ede5274a3e391e37f3/MHRA_GxP_data_integrity_guide_March_edited_Final.pdf)
- **Draft Annex 11 (2025) §16.1:** "Data and metadata should be regularly backed up following established procedures to prevent the loss of data in case of accidental or deliberate change or deletion." **[U]**; **§16.2:** "Backups should be made at suitable intervals (e.g. hourly, daily, weekly and monthly) and their retention determined through a risk-based approach." **[U]**; **§16.6:** "**Restore of data from backup should be tested and documented based on risk during system validation and after changes are made to the backup or restore processes.**" **[U]** — [EC consultation document](https://health.ec.europa.eu/document/download/40231f18-e564-4043-94de-c031f813d38b_en?filename=mp_vol4_chap4_annex11_consultation_guideline_en.pdf)
- **FDA DI Q&A Q1.e:** backup is "a true copy of the original record that is maintained securely throughout the record retention period" and must be "exact, complete, and secure from alteration, inadvertent erasures, or loss". **[U]** — [FDA](https://www.fda.gov/media/119267/download)

**Migration, archiving, retirement:**
- **Annex 11 cl. 4.8:** "If data are transferred to another data format or system, validation should include checks that data are not altered in **value and/or meaning** during this migration process." **[V]**
- **Annex 11 cl. 17:** "Data may be archived. This data should be checked for accessibility, readability and integrity." (plus the retrieval-testing sentence on system change). **[V]**
- **Draft Annex 11 (2025) §17.1:** "After completion of a process, e.g. release of a product, GMP data and metadata (incl. audit trails) should be **protected from deletion and changes throughout the retention period**." **[U]**
- **Draft Annex 11 (2025) §17.2:** "When moving GMP data and metadata from one location to another … the integrity of the data should be verified by a high degree of certainty **before any data is deleted**." **[U]**
- **Draft Annex 11 (2025) §17.5:** "It should be possible to retrieve archived data and metadata in a format which allows searching and sorting of the data." **[U]**
- **Draft Annex 11 (2025) §10.3:** "Where an ad hoc process requires that critical data or a whole database be migrated from one system to another … this should be based on a validated process." **[U]**
- **MHRA §6.17.1 (archive):** long-term retention in a designated secure area; for legacy systems the ability to review data must be periodically verified to confirm continued support of legacy computerised systems. **[U]**
- **PI 041-1 §9.9** "Storage, archival and disposal of electronic data" and **§8.10** "Document retention (Identifying record retention requirements and archiving records)" are the corresponding PIC/S sections. **[V that these sections exist with these titles]** — [PI 041-1](https://picscheme.org/docview/4234)

**Deletion / automatic deletion:**
- **Draft Annex 11 §17.1** ("protected from deletion and changes throughout the retention period") is the most explicit statement against deletion I found. **[U]**
- **MHRA §6.10:** "All data (even if excluded) should be retained with the original data set." **[V]**
- **Annex 11 cl. 9** requires a record of "changes and **deletions**" with a documented reason — i.e. deletion is contemplated only as an audited, justified act. **[V]**
- **PI 041-1 §5.5** frames the risk as "involuntary alteration, **deletion**, loss (either accidental or by security failure) or re-creation or deliberate falsification". **[U]**
- **PI 041-1 §8.11** "Disposal of original records or true copies" — disposal is a controlled, procedural act. **[V that the section exists]**
- **21 CFR 11.10(e):** record changes "shall not obscure previously recorded information". **[U — from knowledge]**

**Raw vs processed / calibrated data:**
- **EU GMP Chapter 4 — the decisive sentence:** "**For electronic records regulated users should define which data are to be used as raw data. At least, all data on which quality decisions are based should be defined as raw data.**" **[V]** — [EU GMP Chapter 4](https://health.ec.europa.eu/system/files/2016-11/chapter4_01-2011_en_0.pdf)
- **MHRA §6.2:** raw data is "the original record (data) which can be described as the first-capture of information". **[V]**
- **MHRA §6.11:** original record is "the first or source capture of data or information e.g. original paper record of manual observation or **electronic raw data file from a computerised system**". **[V]**
- **PI 041-1 §7.5 Original:** "the first-capture of information, whether recorded on paper (static) or electronically". **[V]**
- **PI 041-1 (section 9.1 area):** "In dealing with raw data, the complete capture and retention of raw data would normally be required in order to reconstruct the manufacturing event or analysis." **[U]**
- **MHRA §6.9:** "There should be adequate traceability of any user-defined parameters used within data processing activities to the raw data, including attribution to who performed the activity." **[V]**

### Inferences
- **Restore testing:** Annex 11 cl. 7.2 requires the restore capability to be checked **during validation** and **monitored periodically** — two distinct obligations. The draft §16.6 adds a third trigger: **after changes to the backup or restore process**. A documented restore test (date, scope, who, result, discrepancies) performed at validation, on a defined periodic schedule, and after any change to the backup tooling, satisfies all three.
- **Raw vs calibrated — the defensible answer.** The guidance never decides whether a sensor's raw ADC/resistance value or the calibrated temperature is "the original record". GMP Chapter 4 explicitly delegates the decision: *the regulated user defines which data are raw data*, with the floor that anything a quality decision rests on must be raw data. Since quality decisions rest on the **calibrated temperature in °C**, the calibrated value **must** be treated as raw data and retained. The question is whether the underlying raw value must *also* be retained. The strongest position, and the one that makes the versioned-decoder design a feature rather than a liability, is:
  - retain the **raw payload as received** (immutable, append-only);
  - retain the **decoder/calibration version** applied and the **derived value**;
  - be able to **re-derive** the value from raw payload + decoder version at any time;
  - treat any change of calibration coefficients as an **audited configuration change with a reason**, producing a **new derived record** rather than mutating the old one.
  This directly satisfies MHRA §6.9's traceability-of-processing-parameters requirement and PI 041-1's "complete capture and retention of raw data … in order to reconstruct".
- **Applying a calibration correction to already-stored data is the dangerous case,** and the guidance is silent on it specifically. Overwriting stored temperatures with recalibrated values would breach Annex 11 cl. 9 (change without audit trail and reason), PI 041-1 §7.5 Original (destroys first capture) and MHRA §6.9 (invisible modification of processing parameters). Writing a *new* derived series with a new decoder version, while keeping the old series visible, breaches none of them. This is the single clearest "where his design choices are defensible" answer in the whole brief.
- **Automatic deletion:** no instrument prohibits time-based deletion outright, but draft Annex 11 §17.1's "protected from deletion and changes throughout the retention period" plus §17.2's "verified … before any data is deleted" mean any automatic purge must be (a) impossible before the retention period expires, (b) preceded by a verified archive, and (c) audited. A cron job that drops old partitions without those three controls is indefensible. A sensible product decision: no automatic deletion at all; deletion is a manual, authorised, logged operation.

### Gaps
- I could not confirm the draft Annex 11 section numbers for backup (16) and archiving (17) from a second independent source.
- **No guidance found that addresses recalibration of an already-deployed sensor and what to do with previously stored values.** Genuine silence; the position above is inference from first principles.
- No guidance found that sets a maximum retention period or requires deletion (the GDPR angle — personal data minimisation vs GxP retention — was outside this assignment's scope and was not researched).

---

## Cross-cutting summary of sources and their status

| Document | Status for a Slovak sole trader | URL |
|---|---|---|
| EudraLex Vol. 4 **Annex 11** (2011) | **Binding** for GMP; the baseline computerised-systems law | https://health.ec.europa.eu/system/files/2016-11/annex11_01-2011_en_0.pdf |
| **Draft Annex 11** (2025 consultation) | **Not yet binding**; status as of Sept 2026 unverified — check before relying | https://health.ec.europa.eu/document/download/40231f18-e564-4043-94de-c031f813d38b_en?filename=mp_vol4_chap4_annex11_consultation_guideline_en.pdf |
| EudraLex Vol. 4 **Chapter 4** (Documentation) | **Binding** for GMP; source of the batch-record retention rule | https://health.ec.europa.eu/system/files/2016-11/chapter4_01-2011_en_0.pdf |
| **EU GDP Guidelines 2013/C 343/01** | **Binding** for wholesale distribution; source of "at least five years" | https://health.ec.europa.eu/system/files/2016-11/2013_c343_01_en_0.pdf |
| **Directive 2001/83/EC** Art. 80 | **Binding** (via Slovak transposition); legal basis for distributor records | https://eur-lex.europa.eu/eli/dir/2001/83/oj/eng |
| **zákon 362/2011 Z. z.** § 18 | **Binding national law** — NOT retrieved, must be checked | https://www.slov-lex.sk/pravne-predpisy/SK/ZZ/2011/362/ |
| **PIC/S PI 041-1** (1 July 2021) | Inspectorate guidance; ŠÚKL is a PIC/S member — treated as the inspection standard | https://picscheme.org/docview/4234 |
| **MHRA GXP DI Guidance** (Mar 2018) | Persuasive only (UK); the most operationally useful text | https://assets.publishing.service.gov.uk/media/5aa2b9ede5274a3e391e37f3/MHRA_GxP_data_integrity_guide_March_edited_Final.pdf |
| **WHO TRS 996 Annex 5** (2016) | Persuasive; best source on time control and late entries | https://rx-360.org/wp-content/uploads/2018/08/WHO_TRS_996_annex05.pdf |
| **FDA DI & CGMP Q&A** (Dec 2018) | Non-EU comparator | https://www.fda.gov/media/119267/download |
| **21 CFR Part 11** | Non-EU comparator — NOT retrieved | https://www.ecfr.gov/current/title-21/chapter-I/subchapter-A/part-11 |
| **ISPE GAMP Records & Data Integrity Guide** | Paywalled — nothing obtained | — |

### Top three items the user must verify himself before relying on these notes
1. **PIC/S PI 041-1 §9.6 and §9.7** — open the PDF and transcribe the Item/Expectation table cells. The audit-trail and data-capture expectations are the heart of his case and I could not get them reliably.
2. **Directive 2001/83/EC Article 80(e)/(f)** and **§ 18 of zákon 362/2011 Z. z.** — the retention argument rests on these and neither was retrieved verbatim.
3. **The current status of the draft Annex 11 revision as of September 2026** — whether it is still a draft, and its final numbering. Everything in section 5 and parts of section 9 of these notes depends on it.
