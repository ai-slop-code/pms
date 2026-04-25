# API types

This directory holds TypeScript types for the backend API consumed by the Vue app.

## Two sources of truth (temporarily)

- **Hand-authored** (`analytics.ts`, `bookingPayouts.ts`, `cleaning.ts`, etc.) —
  the current source of truth for module DTOs. Kept in place to avoid churn
  across 250+ tests.
- **Generated** (`generated.ts`) — produced from
  [`spec/openapi.yaml`](../../../../spec/openapi.yaml) by
  [`openapi-typescript`](https://openapi-ts.dev/) via
  `npm run types:openapi`. Currently covers the system / auth / users /
  properties surface.

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
3. Point consumers (stores, views) at types imported from
   `./generated` (aliased via `index.ts` so call sites need one
   import swap, not dozens).
4. Delete the hand-authored `*.ts` for that module.

Order of migration (lowest risk first): `users`, `finance`, `invoice`,
`occupancy`, `cleaning`, `messages`, `nuki`, `dashboard`, `analytics`,
`bookingPayouts`.

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
