ALTER TABLE named_stays ADD COLUMN late_checkout INTEGER NOT NULL DEFAULT 0 CHECK (late_checkout IN (0, 1));
ALTER TABLE property_profiles ADD COLUMN late_check_out_time TEXT NOT NULL DEFAULT '13:00';
ALTER TABLE cleaning_monthly_summaries ADD COLUMN late_checkout_bonus_cents INTEGER NOT NULL DEFAULT 0;

INSERT INTO finance_categories (property_id, code, title, direction, counts_toward_property_income, active, created_at, updated_at)
SELECT NULL, 'late_checkout', 'Late checkout', 'incoming', 0, 1,
       strftime('%Y-%m-%dT%H:%M:%SZ', 'now'), strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
WHERE NOT EXISTS (SELECT 1 FROM finance_categories WHERE property_id IS NULL AND code = 'late_checkout');

CREATE UNIQUE INDEX ux_finance_transactions_late_checkout_stay
ON finance_transactions (property_id, source_type, source_reference_id)
WHERE source_type IN ('late_checkout', 'late_checkout_cleaner_bonus') AND source_reference_id IS NOT NULL;
