# PMS Implementation Prompt

## Role
You are a senior AI coding agent implementing a production-oriented MVP of a Property Management System (PMS).

You must build the system module by module, using the specification package in this folder as the source of truth.

## Read These Files First
Before writing code, read these files in this exact order:
1. `spec/PMS_01_Architecture_and_Global_Spec.md`
2. `spec/PMS_02_Module_Specifications.md`
3. `spec/PMS_03_Implementation_Checklists.md`
4. `spec/initial_prompt.md`
5. `spec/Prompt_answers.md`

## Objective
Implement a multi-user web application for managing short-term rental properties with:
- Go backend
- Vue.js frontend
- SQLite database
- architecture prepared for future PostgreSQL migration

The system must support:
- authentication and authorization
- multi-property management
- occupancy sync from Booking.com ICS
- Nuki access code generation and cleanup
- cleaner log and salary analytics
- finance ledger with recurring expenses
- invoice PDF generation
- multilingual customer check-in messages
- dashboard summaries

## Product Rules
- This is a multi-user web app.
- Login is email/password only in v1.
- Roles are `super_admin`, `owner`, `property_manager`, and `read_only`.
- Permissions are property-scoped and module-granular.
- All API endpoints must be authenticated unless explicitly public.
- Backend/API audit logging is required in v1.
- SQLite is the initial database, but the codebase must be structured to migrate to PostgreSQL later.
- One property equals one rentable unit.
- Occupancy/stay is the core cross-module record.
- Invoices are manual and non-VAT in v1.
- Customer messages are generic and not guest-personalized.
- Google Calendar direct sync is out of scope for v1; JSON export for n8n is required.

## Mandatory Implementation Principles
- Follow the spec package exactly unless implementation reality reveals a conflict; if so, document the conflict and choose the safer design.
- Prefer simple, maintainable architecture over clever abstractions.
- Use explicit database migrations from the start.
- Keep external integrations behind interfaces/services.
- Make all sync and scheduled jobs idempotent.
- Use property timezone consistently for date/month calculations.
- Never rely only on frontend authorization.
- Never log secrets, access tokens, Nuki credentials, or raw access codes.
- Snapshot mutable billing data into invoices at generation time.

## Recommended Build Order

### Phase 1: Foundations
Implement first:
1. backend and frontend project scaffolding
2. database migrations and base schema
3. authentication
4. roles and permissions
5. property management
6. audit logging

### Phase 2: Occupancy Core
Implement next:
1. ICS source configuration
2. hourly/manual sync
3. raw and normalized occupancy storage
4. occupancy calendar and list UI
5. authenticated occupancy JSON export endpoint

### Phase 3: Operational Modules
Implement next:
1. Nuki integration and access code lifecycle
2. cleaning log derived from Nuki events
3. monthly salary calculation and adjustments

### Phase 4: Business Modules
Implement next:
1. finance ledger
2. recurring expenses
3. cleaner salary expense integration
4. invoices and PDF generation
5. customer message templates and clipboard generation

### Phase 5: Hardening
Implement last:
1. dashboard summaries
2. integration status/error visibility
3. retries and cleanup jobs
4. attachment handling
5. production-readiness cleanup

## Per-Module Expectations

### Global Platform
- Implement secure login.
- Implement role-based and property-scoped module permissions.
- Implement user/property administration.
- Implement audit logging for backend/API actions.

### Occupancy
- Store raw ICS events and normalized occupancies.
- Support hourly sync and manual sync.
- Show occupancy in calendar and list views.
- Provide authenticated JSON export for automation.

### Nuki
- Generate access codes from occupancies.
- Use property-configured check-in/check-out times.
- Avoid duplicates on re-sync.
- Clean up expired codes daily.
- Show active and historical codes.

### Cleaning
- Derive cleaning days from Nuki events.
- Count only the first entry per day.
- Apply fee history over time.
- Support monthly bonus/adjustment.
- Show monthly stats and arrival-time heatmap.

### Finance
- Support incoming/outgoing transactions.
- Support categories and property-income reporting.
- Generate recurring monthly expenses when a month is opened.
- Avoid duplicate generated entries.
- Link cleaner salary into finance.

### Invoices
- Create invoices manually.
- Use per-property per-year numbering.
- Support Slovak and English PDFs.
- Store PDF metadata in DB and file on disk.
- Support regeneration with version history.

### Messages
- Support property-specific editable templates.
- Support English, Slovak, German, Ukrainian, and Hungarian.
- Generate check-in messages from occupancy, property, and Nuki data.
- Provide one-click copy-to-clipboard in UI.

## Deliverables Expected From You
- working backend
- working frontend
- database migrations
- module-by-module implementation
- targeted automated tests for each module
- configuration examples where needed

## Definition of Done
A module is not done unless:
- backend endpoints are implemented
- database schema and migrations are implemented
- frontend screens are implemented where applicable
- permissions are enforced
- edge cases from the spec are handled
- targeted tests exist
- checklist items in `spec/PMS_03_Implementation_Checklists.md` can be checked off

## Important Constraints
- Do not invent requirements that conflict with the spec package.
- Do not skip historical tracking where the spec explicitly requires it.
- Do not merge operational calculations and accounting records into one ambiguous model.
- Do not assume ICS contains complete guest billing data.
- Do not implement Google Calendar direct sync in v1.

## Working Method
For each phase:
1. summarize what you are about to implement
2. implement the backend first where dependencies require it
3. implement UI for the module
4. add or update targeted tests
5. verify against the checklist
6. move to the next module only when the current module is coherent end-to-end

## Final Instruction
Use the specification package in `spec/` as the authoritative implementation guide. If a detail is ambiguous, choose the more conservative, auditable, and maintainable interpretation consistent with the overall product goals.
