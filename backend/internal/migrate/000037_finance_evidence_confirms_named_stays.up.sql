-- PMS 21: payout/statement evidence confirms a Booking.com stay even when the
-- historical ICS event is no longer available.
UPDATE named_stays
SET review_status = 'confirmed',
    review_reason = NULL,
    nuki_generation_status = CASE
        WHEN status = 'active'
         AND stay_type IN ('booking_com', 'external')
         AND (stay_outcome IS NULL OR stay_outcome NOT IN ('cancelled_non_refundable', 'no_show'))
         AND check_out_date >= strftime('%Y-%m-%d', 'now')
         AND COALESCE(nuki_generation_status, 'not_applicable') = 'not_applicable'
        THEN 'pending'
        ELSE nuki_generation_status
    END,
    updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
WHERE review_status = 'needs_review'
  AND review_reason = 'legacy_non_reservation_stay'
  AND EXISTS (
      SELECT 1
      FROM finance_bookings fb
      WHERE fb.named_stay_id = named_stays.id
        AND fb.property_id = named_stays.property_id
        AND lower(trim(COALESCE(fb.source_channel, ''))) = 'booking_com'
        AND (fb.has_payout_data = 1 OR fb.has_statement_data = 1)
        AND upper(trim(COALESCE(fb.status, fb.reservation_status, ''))) NOT IN
            ('CANCELLED', 'CANCELLED_BY_GUEST', 'CANCELLED_BY_PARTNER')
  );
