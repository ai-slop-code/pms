<script setup lang="ts">
import { nextTick, ref, watch } from 'vue'
import { X } from 'lucide-vue-next'
import UiIconButton from './UiIconButton.vue'

interface Props {
  open: boolean
  title?: string
  size?: 'sm' | 'md' | 'lg'
  persistent?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  title: '',
  size: 'md',
  persistent: false,
})

const emit = defineEmits<{ (e: 'update:open', v: boolean): void }>()

const dialogRef = ref<HTMLDialogElement | null>(null)
const previousActive = ref<HTMLElement | null>(null)

function close() {
  if (props.persistent) return
  emit('update:open', false)
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape' && !props.persistent) {
    e.preventDefault()
    close()
  }
  if (e.key === 'Tab') {
    const root = dialogRef.value
    if (!root) return
    const focusable = root.querySelectorAll<HTMLElement>(
      'a[href], button:not([disabled]), textarea:not([disabled]), input:not([disabled]):not([type="hidden"]), select:not([disabled]), [tabindex]:not([tabindex="-1"])'
    )
    if (!focusable.length) return
    const first = focusable[0]
    const last = focusable[focusable.length - 1]
    if (!first || !last) return
    if (e.shiftKey && document.activeElement === first) {
      e.preventDefault()
      last.focus()
    } else if (!e.shiftKey && document.activeElement === last) {
      e.preventDefault()
      first.focus()
    }
  }
}

watch(
  () => props.open,
  async (isOpen) => {
    if (isOpen) {
      previousActive.value = document.activeElement as HTMLElement | null
      await nextTick()
      const root = dialogRef.value
      if (root) {
        const focusable = root.querySelector<HTMLElement>(
          'input:not([disabled]), textarea:not([disabled]), select:not([disabled]), button:not([disabled])'
        )
        ;(focusable ?? root).focus()
      }
    } else if (previousActive.value) {
      previousActive.value.focus()
    }
  }
)
</script>

<template>
  <Teleport to="body">
    <div v-if="props.open" class="ui-dialog__backdrop" role="presentation" aria-hidden="true" @click="close" />
    <div
      v-if="props.open"
      ref="dialogRef"
      class="ui-dialog"
      :class="`ui-dialog--${size}`"
      role="dialog"
      aria-modal="true"
      :aria-label="title || undefined"
      tabindex="-1"
      @keydown="onKeydown"
    >
      <header v-if="title || !persistent" class="ui-dialog__header">
        <h2 v-if="title" class="ui-dialog__title">{{ title }}</h2>
        <UiIconButton v-if="!persistent" label="Close" @click="close">
          <X :size="18" aria-hidden="true" />
        </UiIconButton>
      </header>
      <div class="ui-dialog__body">
        <slot />
      </div>
      <footer v-if="$slots.footer" class="ui-dialog__footer">
        <slot name="footer" />
      </footer>
    </div>
  </Teleport>
</template>

<style scoped>
.ui-dialog__backdrop {
  position: fixed;
  inset: 0;
  background: var(--color-scrim);
  z-index: 50;
}
.ui-dialog {
  position: fixed;
  inset: 0;
  margin: auto;
  background: var(--color-surface);
  border-radius: var(--radius-lg);
  box-shadow: var(--shadow-3);
  display: flex;
  flex-direction: column;
  z-index: 51;
  max-height: min(90vh, 800px);
  overflow: hidden;
}
.ui-dialog--sm {
  width: min(440px, 92vw);
  height: fit-content;
}
.ui-dialog--md {
  width: min(640px, 92vw);
  height: fit-content;
}
.ui-dialog--lg {
  width: min(960px, 94vw);
  height: fit-content;
}
.ui-dialog__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-3);
  padding: var(--space-4) var(--space-5);
  border-bottom: 1px solid var(--color-border);
}
.ui-dialog__title {
  font-size: var(--font-size-h3);
  font-weight: 600;
  margin: 0;
}
.ui-dialog__body {
  padding: var(--space-4) var(--space-5);
  overflow-y: auto;
  flex: 1;
}
.ui-dialog__footer {
  display: flex;
  justify-content: flex-end;
  gap: var(--space-2);
  padding: var(--space-3) var(--space-5);
  border-top: 1px solid var(--color-border);
  background: var(--color-sunken);
}
@media (max-width: 639.98px) {
  .ui-dialog {
    inset: 0;
    margin: 0;
    width: 100vw;
    max-width: 100vw;
    height: 100vh;
    max-height: 100vh;
    border-radius: 0;
  }
}
</style>
