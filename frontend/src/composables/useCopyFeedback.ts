import { ref } from 'vue'

/**
 * Tracks transient "Copied!" UI feedback state for copy-to-clipboard flows.
 * Returns a `copied` ref and a `flash(key)` helper that sets the ref to the
 * given key for `durationMs` and then clears it. The default 1.2 s matches
 * the existing behaviour in NukiView and MessagesView.
 */
export function useCopyFeedback(durationMs = 1200) {
  const copied = ref<string | number | null>(null)
  let timer: ReturnType<typeof setTimeout> | null = null

  function flash(key: string | number): void {
    copied.value = key
    if (timer) clearTimeout(timer)
    timer = setTimeout(() => {
      copied.value = null
      timer = null
    }, durationMs)
  }

  return { copied, flash }
}
