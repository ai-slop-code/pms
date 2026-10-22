package migrate

import (
	"database/sql"
	"strings"
	"testing"

	"pms/backend/internal/dbconn"
)

func TestInvoiceLineItemsMigrationPreservesLegacyAndGuardsRollback(t *testing.T) {
	db, err := dbconn.Open("sqlite://" + t.TempDir() + "/items.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	applyMigrationsThrough(t, db, "000045_invoice_proforma.up.sql")
	now := "2026-10-22T00:00:00Z"
	mustExec(t, db, `INSERT INTO users (id, email, password_hash, role, created_at, updated_at) VALUES (1, 'items@test.local', 'hash', 'owner', ?, ?)`, now, now)
	mustExec(t, db, `INSERT INTO properties (id, name, timezone, owner_user_id, created_at, updated_at) VALUES (1, 'Items', 'UTC', 1, ?, ?)`, now, now)
	mustExec(t, db, `INSERT INTO named_stays (id, property_id, display_name, stay_type, check_in_date, check_out_date, status, cleaning_required, review_status, created_at, updated_at) VALUES (1, 1, 'Guest', 'booking_com', '2026-11-01', '2026-11-03', 'active', 1, 'confirmed', ?, ?)`, now, now)
	mustExec(t, db, `INSERT INTO invoices (id, property_id, named_stay_id, invoice_number, sequence_year, sequence_value, language, issue_date, taxable_supply_date, due_date, stay_start_date, stay_end_date, supplier_snapshot_json, customer_snapshot_json, amount_total_cents, created_at, updated_at) VALUES (1, 1, 1, 'TEST/2026/0001', 2026, 1, 'en', ?, ?, ?, ?, '2026-11-03T00:00:00Z', '{"name":"Owner"}', '{"name":"Guest"}', 123456, ?, ?)`, now, now, now, now, now, now)
	mustExec(t, db, `INSERT INTO invoice_files (id, invoice_id, version, file_path, file_size_bytes, created_at) VALUES (1, 1, 1, 'invoices/original.pdf', 1234, ?)`, now)
	mustExec(t, db, `INSERT INTO invoice_sequences (property_id, sequence_year, current_value, created_at, updated_at) VALUES (1, 2026, 1, ?, ?)`, now, now)
	up, err := embeddedMigrations.ReadFile("000046_invoice_line_items.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	down, err := embeddedMigrations.ReadFile("000046_invoice_line_items.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, migration := range [][]byte{up, down, up} {
		mustExec(t, db, string(migration))
	}
	var snapshot sql.NullString
	var amount int
	var number, path, supplier string
	if err := db.QueryRow(`SELECT i.line_items_json, i.amount_total_cents, i.invoice_number, f.file_path, i.supplier_snapshot_json FROM invoices i JOIN invoice_files f ON f.invoice_id=i.id WHERE i.id=1`).Scan(&snapshot, &amount, &number, &path, &supplier); err != nil {
		t.Fatal(err)
	}
	if snapshot.Valid || amount != 123456 || number != "TEST/2026/0001" || path != "invoices/original.pdf" || supplier != `{"name":"Owner"}` {
		t.Fatalf("legacy row changed: %v %d %s %s %s", snapshot, amount, number, path, supplier)
	}
	assertIntQuery(t, db, `SELECT current_value FROM invoice_sequences`, 1)
	for _, invalid := range []string{`[]`, `{}`, `null`, `not json`} {
		if _, err := db.Exec(`UPDATE invoices SET line_items_json = ? WHERE id=1`, invalid); err == nil {
			t.Fatalf("accepted invalid JSON %q", invalid)
		}
	}
	items := `[{"description":"Accommodation","quantity":"1","unit_price_cents":123456,"line_total_cents":123456}]`
	mustExec(t, db, `UPDATE invoices SET line_items_json=? WHERE id=1`, items)
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(string(down))
	_ = tx.Rollback()
	if err == nil || !strings.Contains(err.Error(), "itemized documents exist") {
		t.Fatalf("rollback error=%v", err)
	}
	if err := db.QueryRow(`SELECT line_items_json FROM invoices WHERE id=1`).Scan(&snapshot); err != nil || snapshot.String != items {
		t.Fatalf("failed rollback lost items: %v %v", snapshot, err)
	}
	var integrity string
	if err := db.QueryRow(`PRAGMA integrity_check`).Scan(&integrity); err != nil || integrity != "ok" {
		t.Fatalf("integrity=%s err=%v", integrity, err)
	}
	rows, err := db.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatal("foreign key violation after migration")
	}
}
