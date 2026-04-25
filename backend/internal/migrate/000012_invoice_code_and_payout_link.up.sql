ALTER TABLE properties ADD COLUMN invoice_code TEXT;

ALTER TABLE invoices ADD COLUMN finance_booking_payout_id INTEGER REFERENCES finance_booking_payouts (id) ON DELETE SET NULL;

CREATE UNIQUE INDEX ux_invoices_property_booking_payout
ON invoices (property_id, finance_booking_payout_id)
WHERE finance_booking_payout_id IS NOT NULL;
