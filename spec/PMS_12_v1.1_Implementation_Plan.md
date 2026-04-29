# PMS v1.1 — Implementation Plan

## Scope
Incremental release on top of v1.0. Tracks all work items targeted for the
v1.1 milestone. Each task lists the user-facing outcome, the surfaces that
must change (backend, frontend, schema, spec), and acceptance criteria.

---

## Tasks

### 1. Invoice generation — language, supplier address, property VAT ID  ✅

> **Implementation note (2026-04-29):** Most of the backend, schema and PDF
> plumbing was already in place from an earlier iteration:
>
> - The `invoices.language` column, the `language` field on the request /
>   response DTOs, the SK/EN label dictionary inside `invoicepdf`, and
>   locale-aware money formatting all existed.
> - `vat_id`, `ico`, `dic` already live on `property_profiles` (not on
>   `properties`), and the property settings API already exposes them in
>   `propertySettingsProfileDTO`.
> - `defaultInvoiceSupplier` already snapshots the property address as a
>   fallback when the profile billing address is empty.
>
> The only real gap was the **frontend property settings form**, which did
> not expose the tax/billing fields, so a user could not actually configure
> a VAT ID. That gap is now closed.

**User-facing outcome**

- Operator can choose the invoice language (Slovak or English) when
  generating or regenerating an invoice PDF. The chosen language is
  persisted on the invoice so re-rendered versions stay consistent.
- The "Dodávateľ" / "Supplier" block on the rendered PDF shows the property
  address configured on the property, in addition to the existing supplier
  identity fields.
- A property can have its own VAT ID configured. The VAT ID is shown in the
  supplier block of the invoice PDF when present.

**Backend** (already in place — no changes required)

- `vat_id`, `ico`, `dic`, `billing_address`, `city`, `postal_code`,
  `country` exist on `property_profiles` and round-trip through the
  `/api/properties/{id}/settings` endpoints (see
  `propertySettingsProfileDTO` and `Store.UpdatePropertyProfile`).
- The supplier snapshot stored on each invoice (`supplier_snapshot_json`)
  already captures all of the above. `defaultInvoiceSupplier` falls back
  to the property's own `address_line1` / `city` / `postal_code` /
  `country` when the profile billing address is blank — so the invoice
  shows the property address whenever the user hasn't entered a separate
  billing address.
- `invoices.language` is persisted, validated against `{sk, en}` and
  defaults to `sk` for legacy rows. `language` is accepted on POST /
  PATCH and re-used on regeneration.

**Frontend** (this release)

- `PropertyDetailView` profile tab now has a **Billing & tax** section
  with inputs for `legal_owner_name`, `billing_name`, `billing_address`,
  `billing` city / postal code / country, `IČO`, `DIČ`, `VAT ID (IČ DPH)`.
  The form posts the new keys via the existing `/settings` PATCH.
- The invoice list shows the document language as a small badge on each
  row so it is obvious whether a SK or EN PDF was generated.
- Invoice editor already had a language selector defaulting to the
  property's `default_language`; no changes needed.

**`invoicepdf` package** (already in place — no changes required)

- Language-aware label dictionary covers headings, party titles, detail
  rows, info card titles, payment-note copy and the footer.
- Money formatting switches the decimal separator (`,` for SK, `.` for
  EN). The supplier card prints `ICO`, `DIC` and `VAT ID` lines whenever
  set on the snapshot; the address is rendered from the snapshot fields.

**Spec updates**

- `spec/openapi.yaml` does not currently enumerate the property profile
  fields or the invoice schema in detail, so no diff there. The
  description above is the canonical reference for v1.1.

**Acceptance criteria**

- Generating a new invoice with `language=en` produces a PDF with English
  labels and English-formatted dates and amounts.
- Regenerating the same invoice without specifying a language re-uses the
  language stored at creation time.
- The supplier block on every newly generated invoice shows the property's
  address lines and (when set) VAT ID.
- Invoices issued before the v1.1 deploy continue to render in Slovak with
  the previously snapshotted supplier data; no historical PDF is altered.
- Backend unit tests cover: language default resolution, snapshot of
  property address + VAT ID, label localisation in `invoicepdf`.

---
