CREATE TEMP TABLE invoice_line_items_down_guard (id INTEGER);
CREATE TEMP TRIGGER invoice_line_items_down_guard_check
BEFORE INSERT ON invoice_line_items_down_guard
WHEN EXISTS (SELECT 1 FROM invoices WHERE line_items_json IS NOT NULL)
BEGIN
    SELECT RAISE(ABORT, 'cannot remove invoice line items while itemized documents exist');
END;
INSERT INTO invoice_line_items_down_guard VALUES (1);
DROP TABLE invoice_line_items_down_guard;
ALTER TABLE invoices DROP COLUMN line_items_json;
