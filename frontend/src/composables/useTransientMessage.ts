import { ref } from 'vue'

/**
 * Tracks a short-lived status string (e.g. "Template saved") that should
 * clear itself after a delay. Replaces the scattered
 *
 *   success.value = 'Saved'
 *   setTimeout(() => { success.value = '' }, 3000)
 *
 * pattern with a single `show(msg)` call. The composable also clears any
 * pending timer when the message is replaced, so rapid successive updates
 * don't clip each other or fire a late reset.
 */
export function useTransientMessage(durationMs = 3000) {
  const message = ref('')
  let timer: ReturnType<typeof setTimeout> | null = null

  function clearTimer() {
    if (timer) {
      clearTimeout(timer)
      timer = null
    }
  }

  function show(value: string): void {
    message.value = value
    clearTimer()
    if (!value) return
    timer = setTimeout(() => {
      message.value = ''
      timer = null
    }, durationMs)
  }

  function clear(): void {
    clearTimer()
    message.value = ''
  }

  return { message, show, clear }
}
