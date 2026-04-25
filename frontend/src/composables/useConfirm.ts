import { reactive } from 'vue'

export type ConfirmTone = 'default' | 'danger'

export interface ConfirmOptions {
  title?: string
  message: string
  confirmLabel?: string
  cancelLabel?: string
  tone?: ConfirmTone
}

interface InternalState {
  open: boolean
  options: ConfirmOptions
  resolve: ((value: boolean) => void) | null
}

const state = reactive<InternalState>({
  open: false,
  options: { message: '' },
  resolve: null,
})

function confirm(options: ConfirmOptions): Promise<boolean> {
  // If a previous confirm is still open, resolve it as cancelled first so we
  // never leak a dangling promise.
  if (state.resolve) {
    state.resolve(false)
    state.resolve = null
  }
  state.options = { ...options }
  state.open = true
  return new Promise<boolean>((resolve) => {
    state.resolve = resolve
  })
}

function respond(value: boolean) {
  const resolve = state.resolve
  state.resolve = null
  state.open = false
  if (resolve) resolve(value)
}

export function useConfirm() {
  return {
    state,
    confirm,
    respond,
  }
}
