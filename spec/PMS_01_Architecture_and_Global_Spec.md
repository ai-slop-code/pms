# PMS Specification Package

## Document Purpose
This document defines the global architecture, cross-cutting rules, data boundaries, and implementation order for a Property Management System (PMS) web application. It is written for a single AI coding agent that will implement the system module by module.

This specification is based on:
- `initial_prompt.md`
- `Prompt_answers.md`

## Product Goal
Build a multi-user web application for managing short-term rental properties. The system must support:
- property-based multi-tenancy
- role-based access control
- occupancy synchronization from ICS sources
- Nuki access management
- cleaner activity and salary analytics
- finance tracking
- manual invoice generation with PDF output
- customer message template generation

The first production target is a VPS-hosted web app using:
- Backend: Go
- Frontend: Vue.js
- Database: SQLite

The design must keep future migration to PostgreSQL feasible.

## Analyst Notes: Gaps, Risks, and Challenged Decisions
The following points are important for a correct implementation and should be treated as explicit product decisions.

### 1. SQLite is valid for v1, but the code must be migration-friendly
SQLite is acceptable for the initial deployment, but the backend must avoid SQLite-specific assumptions in repository code, migrations, and transaction handling. Schema design should use types and constraints that can move to PostgreSQL later with minimal rewrite.

### 2. ICS data is not enough for all downstream workflows
ICS feeds usually provide stay dates and summary text, but often do not provide complete guest identity or billing information. Therefore:
- occupancy sync can drive calendars, Nuki codes, and generic messages
- invoice generation cannot rely on ICS alone
- invoice creation must require manual customer detail entry

### 3. Property-level credentials are sensitive
Each property may contain Nuki credentials, ICS URL, contact details, Wi-Fi, and billing data. Sensitive values must not be stored or logged in plaintext where avoidable. At minimum:
- secrets must be excluded from API responses unless explicitly needed
- secrets must be masked in UI after initial save where practical
- logs must never print tokens, credentials, or access codes

### 4. Recurring finance entries generated "when opening a month" must be idempotent
This is a non-trivial rule. If recurring entries are generated lazily when a month is opened, the implementation must ensure:
- no duplicate auto-generated entries for the same recurring rule and month
- future amount changes affect only future months
- already generated records remain historically correct

### 5. Cleaner salary and finance integration need two layers
The cleaner module calculates operational salary continuously from Nuki events and fee history. The finance module tracks accounting entries. These are not the same thing. To avoid inconsistency:
- cleaner salary should exist as a calculated monthly draft
- finance should create or update one linked monthly expense entry per property/month
- the user must be able to apply a manual override, for example a bonus

### 6. Direct Google Calendar integration is intentionally excluded from v1
Native Google Calendar sync requires OAuth or service-account based integration, token storage, sync reconciliation, and failure handling. This adds extra implementation and operational complexity that is not needed for the first release. The v1 approach is:
- build a robust JSON occupancy endpoint first
- let n8n push data to Google Calendar if needed
- defer direct Google Calendar integration until a later phase

### 7. Messages are generic and not guest-personalized
Because guest identity is not guaranteed from ICS, customer messages must be designed as generic arrival instructions with inserted stay dates, property details, Nuki code, and check-in/check-out rules.

## Scope Definition

### In Scope for v1
- authentication and role-based access control
- multi-property support
- per-property permissions by module
- backend/API logging
- property management
- occupancy sync from configurable ICS URL
- occupancy calendar and list views
- JSON occupancy endpoint secured via token
- Nuki access code generation and cleanup
- cleaner log and monthly salary analytics
- finance ledger with recurring expenses
- manual invoice generation and PDF storage
- property-specific multilingual check-in message templates
- dashboard summaries

### Explicitly Out of Scope for v1
- public booking engine
- payment processing
- VAT invoice logic
- advanced accounting exports
- multi-unit property hierarchy
- multiple cleaners per property as a fully supported workflow
- email or WhatsApp delivery of generated messages
- native Google Calendar sync

## Target Users and Roles

### Roles
- `super_admin`: full access to all users, properties, modules, and settings
- `owner`: owns one or more properties and can fully manage their assigned properties
- `property_manager`: limited operational access to assigned properties and assigned modules
- `read_only`: read-only access to assigned properties and modules

### Permission Model
Permissions are property-scoped and module-granular.

Minimum permission dimensions:
- property access
- module access for each property
- action level: read, write, admin

Suggested modules for permissioning:
- property settings
- occupancy
- Nuki access
- cleaning log
- finance
- invoices
- messages
- users and permissions

## Recommended System Architecture

### Backend
Go monolith with modular internal packages. Recommended layers:
- HTTP handlers / controllers
- service layer for business logic
- repository layer for persistence
- scheduler / background jobs
- integration clients for ICS and Nuki
- PDF generation service
- auth and authorization middleware
- audit logging middleware

### Frontend
Vue.js single-page application with:
- authenticated shell layout
- property switcher
- module-based navigation
- reusable tables/forms/dialogs
- calendar page for occupancy
- dashboard widgets

### Persistence
SQLite database with migration files from day one.

Store PDFs and transaction attachments on disk, with metadata in the database.

Recommended file storage layout:
- `/data/invoices/<property_id>/<year>/<invoice_number>.pdf`
- `/data/attachments/<property_id>/<transaction_id>/<filename>`

Paths should be configurable so storage can later move to object storage if needed.

## Cross-Cutting Functional Requirements

### Authentication
- email/password login only in v1
- all API endpoints require authentication except login and health endpoints
- password storage must use secure hashing
- session strategy may be cookie-based or token-based, but must support browser use cleanly
- logout endpoint required

### Authorization
- every request that touches property data must validate property-level permissions
- module permissions must be checked server-side, not only in frontend
- super admin bypasses property restrictions

### Logging and Audit
- log backend/API actions in v1
- log request metadata, actor, action, target entity, result, and timestamp
- do not log passwords, tokens, raw Nuki credentials, or generated access codes

### Time and Locale
- each property must have its own timezone
- all monthly reporting must use the property timezone
- default locale and preferred invoice language must be property-configurable
- system currency is EUR only in v1

### Background Jobs
Required scheduled jobs:
- hourly ICS sync per property
- daily Nuki code cleanup
- daily cleaner/Nuki event reconciliation

Jobs must be safe to rerun.

### Error Handling
- integration failures must be stored as visible sync/job errors
- errors must be surfaced in UI with retry options where appropriate
- partial failures must not corrupt existing data

### Idempotency Rules
The following operations must be idempotent:
- ICS import for unchanged events
- Nuki code creation for already-processed occupancies
- recurring expense generation for a month already initialized
- monthly cleaner finance expense synchronization

## Core Domain Model
The following data model is the recommended baseline.

### Identity and Access
- `users`
- `roles`
- `properties`
- `property_user_permissions`
- `api_audit_logs`
- `auth_sessions` or `refresh_tokens`

### Property Configuration
- `property_profiles`
- `property_localizations`
- `property_secrets`

### Occupancy
- `occupancy_sources`
- `occupancy_raw_events`
- `occupancies`
- `occupancy_sync_runs`
- `occupancy_api_tokens`

### Nuki
- `nuki_access_codes`
- `nuki_sync_runs`
- `nuki_event_logs`

### Cleaning
- `cleaner_profiles`
- `cleaner_fee_history`
- `cleaning_daily_logs`
- `cleaning_monthly_summaries`
- `cleaning_salary_adjustments`

### Finance
- `finance_categories`
- `finance_transactions`
- `finance_recurring_rules`
- `finance_month_states`

### Invoicing
- `invoice_sequences`
- `invoices`
- `invoice_files`

### Messages
- `message_templates`
- `message_template_versions` optional

## Schema Direction by Entity

### `properties`
Must include:
- id
- name
- timezone
- default_language
- default_currency = EUR
- owner_user_id
- address fields
- active flag
- created_at
- updated_at

### `property_profiles`
Must include:
- property_id
- legal_owner_name
- billing_name
- billing_address
- city
- postal_code
- country
- ICO
- DIC
- VAT_ID
- contact_phone
- Wi-Fi details
- parking instructions
- default check-in time
- default check-out time

### `property_secrets`
Must include:
- property_id
- booking_ics_url
- nuki_api_key or token
- nuki_auth_id

### `occupancies`
Must include:
- id
- property_id
- source_type
- source_event_uid
- start_at
- end_at
- status
- raw_summary
- guest_display_name optional
- imported_at
- last_synced_at
- hash or fingerprint for change detection

### `cleaner_fee_history`
Must include:
- id
- property_id
- cleaning_fee_amount
- washing_fee_amount
- effective_from
- created_by

### `cleaning_daily_logs`
Must include:
- id
- property_id
- day_date
- first_entry_at
- nuki_event_reference
- counted_for_salary boolean
- created_at

Enforce unique `(property_id, day_date)` so only one counted cleaning per day exists.

### `cleaning_salary_adjustments`
Must include:
- id
- property_id
- month
- year
- adjustment_amount
- reason
- created_by

### `finance_transactions`
Must include:
- id
- property_id
- transaction_date
- direction
- amount
- category_id
- note
- source_type
- source_reference_id
- is_auto_generated
- attachment_path optional
- created_at
- updated_at

### `finance_recurring_rules`
Must include:
- id
- property_id
- title
- category_id
- amount
- direction
- frequency = monthly
- start_month
- end_month optional
- effective_from
- effective_to optional
- active flag

### `invoices`
Must include:
- id
- property_id
- invoice_number
- sequence_year
- sequence_value
- language
- issue_date
- taxable_supply_date
- due_date
- stay_start_date
- stay_end_date
- supplier_snapshot_json
- customer_snapshot_json
- amount_total
- currency
- payment_status = paid
- payment_note
- version
- created_by

### `nuki_access_codes`
Must include:
- id
- property_id
- occupancy_id
- code_label
- access_code_masked
- external_nuki_id
- valid_from
- valid_until
- status
- created_at
- revoked_at optional

### `message_templates`
Must include:
- id
- property_id
- language_code
- template_type
- title
- body
- active
- updated_at

## Derived Business Rules

### Occupancy-Derived Rules
- one occupancy/stay is the primary cross-module record
- Nuki code generation references one occupancy
- check-in message generation references one occupancy
- invoice may optionally be linked to one occupancy

### Cleaner Salary Rules
- only the first valid Nuki entry per day counts
- later entries on the same day are fully ignored for cleaner metrics
- monthly base salary = counted cleaning days x (cleaning fee + washing fee)
- monthly final salary = monthly base salary + manual adjustments
- fee history applies immediately from its effective timestamp/date

### Finance Rules
- cleaner salary margin = cleaner monthly expense / total monthly property income
- total monthly property income must exclude categories not considered rental income
- recurring expenses are created only for future months and only when the month is initialized/opened

### Invoicing Rules
- invoices are created manually
- one stay corresponds to one invoice
- invoice numbering is per property and per year
- invoices are non-VAT in v1
- invoice PDF must state that payment has already been made via Booking.com

## Recommended API Design Principles
- REST-style JSON API
- stable resource names by module
- pagination for list endpoints
- filter by property on all module endpoints
- include audit-friendly metadata in write responses
- keep integration endpoints separate from UI convenience endpoints

## UI Application Structure

### Global Screens
- login page
- dashboard
- property switcher
- property settings
- user and permission administration
- integration status/errors panel

### Module Screens
- occupancy calendar
- occupancy list
- Nuki access overview
- cleaning log analytics page
- finance ledger
- invoice list and invoice form
- message templates editor and message generation table

## Recommended Build Order for a Single AI Coding Agent

### Phase 1: Foundations
1. project scaffolding
2. database migrations
3. authentication
4. role and permission model
5. property management
6. audit logging

### Phase 2: Occupancy as the core record
1. ICS source configuration
2. raw sync and normalized occupancies
3. occupancy calendar and list UI
4. authenticated occupancy JSON endpoint

### Phase 3: Operational automations
1. Nuki integration
2. access code generation
3. old code cleanup
4. cleaning log derived from Nuki events
5. monthly cleaning analytics

### Phase 4: Business modules
1. finance ledger
2. recurring expenses
3. cleaner expense integration
4. invoice generation and PDF storage
5. message templates and clipboard generation

### Phase 5: Hardening
1. dashboard summaries
2. retries and error visibility
3. attachment handling
4. backup/export hooks
5. migration-readiness cleanups for PostgreSQL

## Suggested Technical Strategy Notes for the Implementing AI Agent
- keep integrations behind interfaces
- keep time calculations property-timezone aware
- introduce migrations immediately
- snapshot mutable legal/billing data into invoices at creation time
- never trust frontend permission checks
- treat external sync operations as resumable and repeatable
- prefer explicit status fields over inferred states

## Deliverable Relationship
This architecture document is complemented by:
- `PMS_02_Module_Specifications.md`
- `PMS_03_Implementation_Checklists.md`
