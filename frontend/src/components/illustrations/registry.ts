import type { Component } from 'vue'
import IllustrationEmptyInbox from './IllustrationEmptyInbox.vue'
import IllustrationNoResults from './IllustrationNoResults.vue'
import IllustrationError from './IllustrationError.vue'
import IllustrationKeys from './IllustrationKeys.vue'
import IllustrationDashboardWelcome from './IllustrationDashboardWelcome.vue'
import IllustrationInvoice from './IllustrationInvoice.vue'
import IllustrationSparkles from './IllustrationSparkles.vue'

/**
 * Named illustrations available to `UiEmptyState`'s `illustration` prop.
 * Keep this surface small — consumers that need a specific illustration
 * directly (e.g. the Login hero) should import the SFC.
 */
export type IllustrationName =
  | 'inbox'
  | 'no-results'
  | 'error'
  | 'keys'
  | 'dashboard'
  | 'invoice'
  | 'sparkles'

export const illustrations: Record<IllustrationName, Component> = {
  inbox: IllustrationEmptyInbox,
  'no-results': IllustrationNoResults,
  error: IllustrationError,
  keys: IllustrationKeys,
  dashboard: IllustrationDashboardWelcome,
  invoice: IllustrationInvoice,
  sparkles: IllustrationSparkles,
}
