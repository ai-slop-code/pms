# PMS 21 Stage 3 Local Dual-Write Verification

Date: 2026-07-13  
Database: `data/pms.db`  
Scope: local/test verification of Stage 3 raw Booking.com block dual-write.

## Preconditions

| Check | Result |
|---|---:|
| Starting schema migrations | 31 |
| Starting latest migration | `000031_ics_dtstamp` |
| Active properties | 1 |
| Active occupancy sources | 1 |
| Properties with Booking.com ICS URL | 1 |

## Migration Application

Applied additive migrations through the application migrator:

| Migration | Result |
|---|---|
| `000032_raw_booking_blocks_named_stays` | Applied locally |
| `000033_raw_block_sync_counters` | Applied locally |

Post-migration schema state:

| Metric | Value |
|---|---:|
| Applied schema migrations | 33 |
| Latest migration | `000033_raw_block_sync_counters` |
| Initial `raw_booking_blocks` rows | 0 |
| Initial `raw_booking_block_nights` rows | 0 |

## Gated Sync Run

Ran one local Booking.com ICS sync with `PMS21_RAW_BLOCKS_DUAL_WRITE=true` through a temporary guarded test harness. The temporary test file was removed after execution.

| Metric | Value |
|---|---:|
| Sync run ID | 6758 |
| Trigger | `pms21_stage3_local_verification` |
| Status | `success` |
| Events seen | 7 |
| Events parsed | 7 |
| Parse errors | 0 |
| Raw blocks inserted | 7 |
| Raw blocks updated | 0 |
| Raw blocks unchanged | 0 |
| Raw blocks deleted from source | 0 |
| Raw block conflicts | 0 |

## Resulting Raw Model Counts

| Metric | Value |
|---|---:|
| `raw_booking_blocks` total | 7 |
| Active `raw_booking_blocks` | 7 |
| Active `raw_booking_block_nights` | 23 |
| Raw blocks touched by verification sync | 7 |

## Parity Checks

| Check | Result |
|---|---:|
| Raw rows without matching legacy Booking.com aggregate UID/date range | 0 |
| Legacy/raw matching UID rows with date-range mismatch | 0 |
| Missing raw night expansion rows | 0 |
| Extra raw night expansion rows | 0 |
| `PRAGMA foreign_key_check` rows | 0 |

## Gate Conclusion

Stage 3 is locally verified for additive raw-block dual-write. The gate remains default-off and must not be enabled in production until the required production audit and backfill/cutover approvals are complete.

No Stage 2 backfill apply was implemented or run. No downstream read gates were enabled.
