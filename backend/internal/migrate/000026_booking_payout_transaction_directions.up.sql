-- Booking payout transactions must use the canonical finance direction values.
-- A previous statement-ingestion path wrote `in`/`out` (or failed to insert on
-- schemas with the CHECK constraint), which made payout money disappear from
-- finance summaries that sum only `incoming`/`outgoing`.

UPDATE finance_transactions
   SET direction = 'incoming',
       updated_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
 WHERE source_type = 'booking_payout'
   AND direction = 'in';

UPDATE finance_transactions
   SET direction = 'outgoing',
       updated_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
 WHERE source_type = 'booking_payout'
   AND direction = 'out';

UPDATE finance_bookings
   SET transaction_id = (
           SELECT ft.id
             FROM finance_transactions ft
            WHERE ft.property_id = finance_bookings.property_id
              AND ft.source_type = 'booking_payout'
              AND ft.source_reference_id = finance_bookings.reference_number
            ORDER BY ft.id DESC
            LIMIT 1
       ),
       updated_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
 WHERE transaction_id IS NULL
   AND EXISTS (
           SELECT 1
             FROM finance_transactions ft
            WHERE ft.property_id = finance_bookings.property_id
              AND ft.source_type = 'booking_payout'
              AND ft.source_reference_id = finance_bookings.reference_number
       );

INSERT INTO finance_transactions (
    property_id,
    transaction_date,
    direction,
    amount_cents,
    category_id,
    note,
    source_type,
    source_reference_id,
    is_auto_generated,
    created_at,
    updated_at
)
SELECT
    fb.property_id,
    fb.payout_date,
    CASE WHEN fb.net_cents < 0 THEN 'outgoing' ELSE 'incoming' END,
    ABS(fb.net_cents),
    (
        SELECT fc.id
          FROM finance_categories fc
         WHERE fc.active = 1
           AND fc.code = 'booking_income'
           AND (fc.property_id IS NULL OR fc.property_id = fb.property_id)
         ORDER BY CASE WHEN fc.property_id IS NULL THEN 1 ELSE 0 END
         LIMIT 1
    ),
    'Booking.com payout ' || fb.reference_number,
    'booking_payout',
    fb.reference_number,
    1,
    strftime('%Y-%m-%dT%H:%M:%SZ', 'now'),
    strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
  FROM finance_bookings fb
 WHERE fb.transaction_id IS NULL
   AND fb.net_cents <> 0
   AND fb.has_payout_data = 1
   AND EXISTS (
        SELECT 1
          FROM finance_categories fc
         WHERE fc.active = 1
           AND fc.code = 'booking_income'
           AND (fc.property_id IS NULL OR fc.property_id = fb.property_id)
   );

UPDATE finance_bookings
   SET transaction_id = (
           SELECT ft.id
             FROM finance_transactions ft
            WHERE ft.property_id = finance_bookings.property_id
              AND ft.source_type = 'booking_payout'
              AND ft.source_reference_id = finance_bookings.reference_number
            ORDER BY ft.id DESC
            LIMIT 1
       ),
       updated_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
 WHERE transaction_id IS NULL
   AND has_payout_data = 1
   AND EXISTS (
           SELECT 1
             FROM finance_transactions ft
            WHERE ft.property_id = finance_bookings.property_id
              AND ft.source_type = 'booking_payout'
              AND ft.source_reference_id = finance_bookings.reference_number
       );
