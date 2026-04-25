/**
 * Async helpers. Keep this file tiny — most async logic belongs in
 * composables or API modules, not here.
 */

/** Promise-based `setTimeout`. Used by retry loops in NukiView/AnalyticsView. */
export function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms))
}
