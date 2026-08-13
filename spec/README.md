# PMS Spec Package

## Purpose
This folder contains the full specification package for the PMS project and is intended to be handed to an AI coding agent as the implementation source of truth.

## Current Authority

For availability and stay identity, the final authority is
`PMS_21_Legacy_Occupancy_Removal_Spec.md`, supported by ADR-007. The final
model uses raw booking blocks, named stays and named-stay nights, source links,
and property availability blocks. Earlier documents that describe
`occupancies`, occupancy IDs, public occupancy export, or synthetic finance
occupancies are historical implementation evidence where they conflict with
that final model.

This authority statement defines the target architecture only. It does not
claim Release A/B production windows, exception resolution, a restore drill,
or destructive cleanup have completed.

## Files

### `PMS_00_Implementation_Prompt.md`
Single handoff prompt for the implementation agent. This is the best starting point when you want an AI coding agent to begin building the project.

### `PMS_01_Architecture_and_Global_Spec.md`
Global architecture, shared rules, domain boundaries, assumptions, risks, and recommended implementation order.

### `PMS_02_Module_Specifications.md`
Detailed module-by-module functional specification, including suggested APIs, database entities, UI screens, and test focus.

### `PMS_03_Implementation_Checklists.md`
Verification checklist to confirm whether the implemented system actually matches the requested functionality.

### `PMS_04_Analytics_Data_Inventory.md`
Inventory of every analytical signal currently captured by the schema, grouped by module, with the metrics derivable from each. Intended as a handoff to a business analyst / property manager scoping a reporting dashboard.

### `initial_prompt.md`
Original business idea and high-level requirements.

### `Prompt_answers.md`
Clarifications and product decisions collected after the initial requirements review.

### `PMS_13_Coding_Conventions.md`
Canonical reference for coding style across the repo: commit message format
(`<TYPE-NN>: <summary>`), branch & release naming, backend (Go) and
frontend (Vue/TS) conventions, migration rules, and the definition of
"done". Read this before opening a PR.

### `PMS_14_Closed_Nights_BA_Spec.md`
Business-analyst view of the "manually mark a night as closed" feature
introduced in v1.1: problem framing, definitions, stakeholder Q&A, and
phased implementation order. Companion to PMS_12 §2.

### `PMS_15_Google_Calendar_Cleaning_Events_Spec.md`
Future native Google Calendar integration for checkout-driven cleaning
events, including same-day turnover event-title logic, idempotent event
reconciliation, Google credential handling, UI requirements, and tests.

### `PMS_16_Finance_Reset_Preserve_Cleaning_Salary_Spec.md`
Business and technical specification for a property-scoped finance reset
that deletes finance records while preserving cleaning lady salary derived
from flat-entry cleaning logs.

### `PMS_17_Stay_Outcome_Overrides_Spec.md`
Manual occupancy-level outcome labels for Booking.com stays such as no-show
and non-refundable cancellation, including cleaning suppression, analytics,
finance, Nuki, and guest messaging behavior.

### `PMS_18_Cleaning_Event_Exclusion_Spec.md`
Manual occupancy-level control for selected real guest stays where cleaning
is handled outside the cleaning lady's Google Calendar. Stays remain normal
occupied/financial stays, but PMS suppresses or removes the managed cleaning
event until the owner restores default behavior.

### `PMS_19_Booking_ICS_Reconciliation_Spec.md`
Historical predecessor to PMS 21 covering upstream identity, split-row repair,
and July 2026 acceptance examples. Its overloaded occupancy representation
model is superseded; retain it as design and incident evidence.

### `PMS_21_Raw_Booking_Blocks_Named_Stays_Migration_Plan.md`
Historical staged migration plan from the overloaded legacy occupancy model to
first-class raw Booking.com blocks, named stays, stay nights, source links, and
availability blocks. Its pre-production and compatibility status is
superseded by the final cleanup specification.

### `PMS_21_Legacy_Occupancy_Removal_Spec.md`
Post-cutover cleanup contract for removing legacy occupancy writes, reads,
routes, DTOs, UI, integration foreign keys, tables, flags, and transitional
tooling while preserving source, business, integration, and audit history.
This is the current PMS 21 authority.

### `docs/adr/ADR-007-final-occupancy-model-and-canonical-stay-ownership.md`
Final architecture decision superseding ADR-005's compatibility window and
ADR-006's permission for unmatched canonical finance bookings. The historical
ADRs remain unchanged.

## Recommended Reading Order
1. `PMS_00_Implementation_Prompt.md`
2. `PMS_01_Architecture_and_Global_Spec.md`
3. `PMS_02_Module_Specifications.md`
4. `PMS_03_Implementation_Checklists.md`
5. `PMS_04_Analytics_Data_Inventory.md`
6. `PMS_13_Coding_Conventions.md`
7. `PMS_14_Closed_Nights_BA_Spec.md`
8. `PMS_15_Google_Calendar_Cleaning_Events_Spec.md`
9. `PMS_16_Finance_Reset_Preserve_Cleaning_Salary_Spec.md`
10. `PMS_17_Stay_Outcome_Overrides_Spec.md`
11. `PMS_18_Cleaning_Event_Exclusion_Spec.md`
12. `PMS_19_Booking_ICS_Reconciliation_Spec.md`
13. `PMS_21_Raw_Booking_Blocks_Named_Stays_Migration_Plan.md`
14. `PMS_21_Legacy_Occupancy_Removal_Spec.md`
15. `initial_prompt.md`
16. `Prompt_answers.md`

## External API references
- **Nuki Smart Lock API** (OpenAPI / Swagger UI): https://api.nuki.io/
- **Google Calendar API**: https://developers.google.com/calendar/api

## Historical v1 Scope Note
The following describes the original v1 scope and is not the current target
after PMS 15 and PMS 21. Public occupancy export, token management, and n8n
export guidance have no place in the final model and are removed through the
gated PMS 21 cleanup sequence.

Direct Google Calendar integration was not part of v1. The intended v1 approach was:
- sync occupancies from ICS
- expose occupancies through the authenticated JSON endpoint
- use `n8n` externally if Google Calendar synchronization is needed

Current Google Calendar cleaning behavior is specified by
`PMS_15_Google_Calendar_Cleaning_Events_Spec.md` and the PMS 21 documents,
using named-stay or raw-block ownership rather than an occupancy ID.

## Suggested Usage
- Use `PMS_00_Implementation_Prompt.md` when starting implementation with an AI coding agent.
- Use `PMS_03_Implementation_Checklists.md` during review or acceptance testing.
- Use the architecture and module specification files when refining implementation details.
