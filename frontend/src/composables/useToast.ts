import { reactive, ref } from 'vue'

export type ToastTone = 'info' | 'success' | 'warning' | 'danger'

export interface ToastOptions {
  tone?: ToastTone
  title?: string
  message?: string
  timeout?: number
}

export interface Toast {
  id: number
  tone: ToastTone
  title: string
  message: string
  timeout: number
}

const toasts = reactive<Toast[]>([])
const seq = ref(0)

function dismiss(id: number) {
  const idx = toasts.findIndex((t) => t.id === id)
  if (idx >= 0) toasts.splice(idx, 1)
}

function push(options: ToastOptions) {
  seq.value += 1
  const timeout = options.timeout ?? (options.tone === 'danger' ? 8000 : 4500)
  const toast: Toast = {
    id: seq.value,
    tone: options.tone ?? 'info',
    title: options.title ?? '',
    message: options.message ?? '',
    timeout,
  }
  toasts.push(toast)
  if (timeout > 0) {
    setTimeout(() => dismiss(toast.id), timeout)
  }
  return toast.id
}

export function useToast() {
  return {
    toasts,
    push,
    dismiss,
    success(msg: string, title = 'Success') {
      return push({ tone: 'success', title, message: msg })
    },
    error(msg: string, title = 'Error') {
      return push({ tone: 'danger', title, message: msg })
    },
    warning(msg: string, title = 'Warning') {
      return push({ tone: 'warning', title, message: msg })
    },
    info(msg: string, title = '') {
      return push({ tone: 'info', title, message: msg })
    },
  }
}
