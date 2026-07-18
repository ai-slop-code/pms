# API types

This directory holds TypeScript types for the backend API consumed by the Vue app.

## Contract Boundary

- **Generated** (`generated.ts`) — produced from
  [`spec/openapi.yaml`](../../../../spec/openapi.yaml) by
  [`openapi-typescript`](https://openapi-ts.dev/) via
  `npm run types:openapi`. These are the backend API contract types.
- **Hand-authored** (`analytics.ts`, `bookingPayouts.ts`, `cleaning.ts`, etc.) —
  allowed only as UI/domain adapters or compatibility shims where they add
  frontend value. They must not contradict `spec/openapi.yaml` for a touched
  endpoint or DTO.

Generated and hand-authored types may coexist. When a surface is touched, update
OpenAPI first, regenerate `generated.ts` when the contract changes, then keep or
adjust hand-authored types only if they intentionally adapt the contract into a
UI/domain shape.

## Regenerate

```bash
cd frontend
npm run types:openapi
```

The generator reads `../spec/openapi.yaml` and writes `generated.ts`.
The file **must not** be edited by hand — commit the regenerated output.

## Migration plan

Each module will be migrated one at a time:

1. Extend `spec/openapi.yaml` with the module's routes and schemas.
2. Run `npm run types:openapi`.
3. Decide whether consumers should use generated types directly or keep a small
   hand-authored adapter type.
4. Delete or rewrite contradictory hand-authored DTOs for that touched surface.

Do not perform a repo-wide type rewrite just to remove hand-authored modules.
Prioritize PMS 21 touched surfaces and any DTO whose current shape cannot
represent named-stay-primary rows.

## Using generated types

```ts
import type { components } from "./generated"

type User = components["schemas"]["User"]
type LoginRequest = components["schemas"]["LoginRequest"]
```

Or path-level:

```ts
import type { paths } from "./generated"

type LoginResponse =
  paths["/auth/login"]["post"]["responses"][200]["content"]["application/json"]
```
