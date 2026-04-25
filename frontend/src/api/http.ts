/**
 * Base URL the frontend uses to reach the backend.
 *
 * - Empty (default): same-origin, e.g. the reverse proxy routes `/api/*` to
 *   the backend on the same host the SPA is served from.
 * - Absolute URL (e.g. `https://api.pms.airport.sk`): cross-origin deployment
 *   where the backend lives on its own domain. In that mode the backend must
 *   set `CORS_ORIGINS` to include the SPA's origin and issue the session
 *   cookie with `SameSite=None; Secure` (both sides must be on HTTPS).
 *
 * Resolution order (first match wins):
 *   1. Runtime config: `window.__PMS_CONFIG__.apiBaseUrl` from `public/config.js`.
 *      Operators can edit this file after deployment without rebuilding.
 *   2. Build-time env: `VITE_API_BASE_URL` baked in via `npm run build`.
 *
 * Trailing slashes are stripped so callers can continue passing paths that
 * start with `/api/...`.
 */
function resolveBase(): string {
  const runtime =
    typeof window !== 'undefined' ? window.__PMS_CONFIG__?.apiBaseUrl : undefined
  const buildTime = import.meta.env?.VITE_API_BASE_URL
  const raw = (runtime ?? buildTime ?? '').toString().trim()
  if (!raw) return ''
  return raw.replace(/\/+$/, '')
}

const base = resolveBase()

/**
 * Normalises any `HeadersInit` — plain record, `Headers`, or an iterable of
 * `[name, value]` tuples — into a plain `Record<string, string>`. The old
 * `...(init.headers as Record<string, string>)` cast silently dropped the
 * last two forms, so callers that passed a `Headers` instance ended up
 * sending no headers at all.
 */
function toHeaderRecord(source: HeadersInit | undefined): Record<string, string> {
  if (!source) return {}
  if (source instanceof Headers) {
    const out: Record<string, string> = {}
    source.forEach((value, key) => {
      out[key] = value
    })
    return out
  }
  if (Array.isArray(source)) {
    return Object.fromEntries(source)
  }
  return { ...source }
}

export async function api<T>(
  path: string,
  init?: RequestInit & { json?: unknown }
): Promise<T> {
  const headers: Record<string, string> = {
    Accept: 'application/json',
    'X-PMS-Client': 'web',
    ...toHeaderRecord(init?.headers),
  }
  let body = init?.body as BodyInit | undefined
  if (init?.json !== undefined) {
    headers['Content-Type'] = 'application/json'
    body = JSON.stringify(init.json)
  }
  const res = await fetch(base + path, {
    ...init,
    headers,
    body,
    credentials: 'include',
    // API responses are never cache-safe: always hit the network so user actions
    // like "Refresh Data" or "Run Reconciliation" reflect the latest server state.
    cache: init?.cache ?? 'no-store',
  })
  const text = await res.text()
  let data: unknown = null
  if (text) {
    try {
      data = JSON.parse(text)
    } catch {
      data = { error: text }
    }
  }
  if (!res.ok) {
    const err = (data as { error?: string })?.error || res.statusText
    throw new Error(err)
  }
  return data as T
}
