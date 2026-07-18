# PMS 21 Stage 10 Local Verification - 2026-07-14

## Scope

- Deprecated legacy occupancy-as-stay API routes with explicit `Deprecation` and `Warning` response headers while retaining compatibility behavior.
- Deprecated public occupancy export/token endpoints in OpenAPI and backend responses.
- Added `PMS21_OCCUPANCY_EXPORT_DISABLED` backend kill switch for returning `410 Gone` from the public export route.
- Removed export-token/n8n/curl guidance from the Occupancy sync UI and stopped frontend calls to `occupancy-api-tokens`.
- Regenerated frontend OpenAPI types after documenting deprecated export/token endpoints.

## Verification

| Check | Result |
| --- | --- |
| `npm run types:openapi` from `frontend/` | Passed |
| `go test ./...` from `backend/` | Passed |
| `npm run type-check` from `frontend/` | Passed |
| `npm run test` from `frontend/` | Passed, 53 files / 278 tests |
| `npm run build` from `frontend/` | Passed |

## Notes

- No additional local Stage 10 implementation work remains.
- Public export storage/table remains in place for compatibility and rollback; normal UI creation/revocation is removed.
- Legacy occupancy-as-stay routes are not removed yet because compatibility callers may still exist during the production migration window.
- Production export disablement requires setting `PMS21_OCCUPANCY_EXPORT_DISABLED=1` during rollout.
