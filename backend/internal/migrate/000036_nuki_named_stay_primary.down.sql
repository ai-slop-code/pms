-- Irreversible compatibility expansion: named-stay-primary rows can validly
-- have no legacy occupancy. Rolling back the binary keeps this additive schema.
SELECT 1;
