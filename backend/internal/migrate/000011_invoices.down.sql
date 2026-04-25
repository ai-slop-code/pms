DROP INDEX IF EXISTS idx_invoice_files_invoice_version;
DROP TABLE IF EXISTS invoice_files;

DROP INDEX IF EXISTS idx_invoices_property_issue_date;
DROP INDEX IF EXISTS ux_invoices_property_occupancy;
DROP TABLE IF EXISTS invoices;

DROP INDEX IF EXISTS idx_invoice_sequences_property_year;
DROP TABLE IF EXISTS invoice_sequences;
