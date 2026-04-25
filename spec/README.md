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

## Recommended Reading Order
1. `PMS_00_Implementation_Prompt.md`
2. `PMS_01_Architecture_and_Global_Spec.md`
3. `PMS_02_Module_Specifications.md`
4. `PMS_03_Implementation_Checklists.md`
5. `PMS_04_Analytics_Data_Inventory.md`
6. `initial_prompt.md`
7. `Prompt_answers.md`

## External API references
- **Nuki Smart Lock API** (OpenAPI / Swagger UI): https://api.nuki.io/

## Important v1 Scope Note
Direct Google Calendar integration is not part of v1. The intended v1 approach is:
- sync occupancies from ICS
- expose occupancies through the authenticated JSON endpoint
- use `n8n` externally if Google Calendar synchronization is needed

## Suggested Usage
- Use `PMS_00_Implementation_Prompt.md` when starting implementation with an AI coding agent.
- Use `PMS_03_Implementation_Checklists.md` during review or acceptance testing.
- Use the architecture and module specification files when refining implementation details.
