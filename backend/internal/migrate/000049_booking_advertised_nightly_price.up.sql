ALTER TABLE named_stays
    ADD COLUMN booking_advertised_nightly_price_cents INTEGER
        CHECK (booking_advertised_nightly_price_cents IS NULL
            OR (booking_advertised_nightly_price_cents > 0
                AND booking_advertised_nightly_price_cents <= 9007199254740991));

ALTER TABLE named_stays
    ADD COLUMN booking_advertised_nightly_price_currency TEXT
        CHECK ((booking_advertised_nightly_price_currency IS NULL
                AND booking_advertised_nightly_price_cents IS NULL)
            OR (booking_advertised_nightly_price_currency = 'EUR'
                AND booking_advertised_nightly_price_cents IS NOT NULL
                AND stay_type = 'booking_com'));
