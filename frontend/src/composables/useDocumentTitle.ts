import { onBeforeUnmount, watchEffect, type MaybeRefOrGetter, toValue } from 'vue'

const BASE_TITLE = 'PMS'

/**
 * Keeps `document.title` in sync with a reactive title string. Prepends the
 * active segment to the base "PMS" brand (e.g. `"Dashboard · PMS"`) and
 * restores the base title when the consumer unmounts.
 *
 * Pass `null` / `undefined` / `""` to show just the base title.
 */
export function useDocumentTitle(title: MaybeRefOrGetter<string | null | undefined>): void {
  watchEffect(() => {
    const value = toValue(title)
    document.title = value ? `${value} · ${BASE_TITLE}` : BASE_TITLE
  })
  onBeforeUnmount(() => {
    document.title = BASE_TITLE
  })
}
