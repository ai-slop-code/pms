DROP INDEX IF EXISTS ux_finance_transactions_late_checkout_stay;
DELETE FROM finance_categories WHERE property_id IS NULL AND code = 'late_checkout';
ALTER TABLE cleaning_monthly_summaries DROP COLUMN late_checkout_bonus_cents;
ALTER TABLE property_profiles DROP COLUMN late_check_out_time;
ALTER TABLE named_stays DROP COLUMN late_checkout;
