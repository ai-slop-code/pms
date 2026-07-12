# PMS Spec Package

## Purpose
This folder contains the full specification package for the PMS project and is intended to be handed to an AI coding agent as the implementation source of truth.

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
Bulletproof Booking.com ICS reconciliation contract covering upstream event
identity, generated/manual split rows, source disappearance, duplicate active
occupancy prevention, repair of existing bad rows, and July 2026 acceptance
tests.

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
13. `initial_prompt.md`
14. `Prompt_answers.md`

## External API references
- **Nuki Smart Lock API** (OpenAPI / Swagger UI): https://api.nuki.io/
- **Google Calendar API**: https://developers.google.com/calendar/api

## Important v1 Scope Note
Direct Google Calendar integration is not part of v1. The intended v1 approach is:
- sync occupancies from ICS
- expose occupancies through the authenticated JSON endpoint
- use `n8n` externally if Google Calendar synchronization is needed

When native Google Calendar cleaning-event sync is selected for a later phase,
use `PMS_15_Google_Calendar_Cleaning_Events_Spec.md` as the implementation
source of truth.

## Suggested Usage
- Use `PMS_00_Implementation_Prompt.md` when starting implementation with an AI coding agent.
- Use `PMS_03_Implementation_Checklists.md` during review or acceptance testing.
- Use the architecture and module specification files when refining implementation details.
