<script setup lang="ts">
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import UiButton from '@/components/ui/UiButton.vue'
import UiCard from '@/components/ui/UiCard.vue'
import UiInput from '@/components/ui/UiInput.vue'
import UiInlineBanner from '@/components/ui/UiInlineBanner.vue'
import IllustrationKeys from '@/components/illustrations/IllustrationKeys.vue'

const email = ref('')
const password = ref('')
const code = ref('')
const recoveryCode = ref('')
const useRecovery = ref(false)
const error = ref('')
const loading = ref(false)
const auth = useAuthStore()
const router = useRouter()
const route = useRoute()

async function submit() {
  error.value = ''
  loading.value = true
  try {
    await auth.login(email.value, password.value)
    if (auth.mfaPending) {
      // Stay on the page; the template swaps in the code input.
      return
    }
    const redirect = (route.query.redirect as string) || '/'
    await router.replace(redirect)
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Login failed'
  } finally {
    loading.value = false
  }
}

async function submitCode() {
  error.value = ''
  loading.value = true
  try {
    if (useRecovery.value) {
      await auth.verifyTwoFactor({ recovery_code: recoveryCode.value })
    } else {
      await auth.verifyTwoFactor({ code: code.value })
    }
    const redirect = (route.query.redirect as string) || '/'
    await router.replace(redirect)
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Verification failed'
  } finally {
    loading.value = false
  }
}

async function cancelChallenge() {
  error.value = ''
  try {
    await auth.logout()
  } finally {
    code.value = ''
    recoveryCode.value = ''
    useRecovery.value = false
  }
}
</script>

<template>
  <div class="login-page">
    <UiCard class="login-card">
      <template #header>
        <div class="login-brand">
          <div class="login-brand__mark" aria-hidden="true">PMS</div>
          <h1 class="login-brand__title">
            {{ auth.mfaPending ? 'Two-factor authentication' : 'Sign in' }}
          </h1>
        </div>
      </template>
      <div class="login-hero" aria-hidden="true">
        <IllustrationKeys />
      </div>

      <!-- Password step -->
      <form v-if="!auth.mfaPending" class="login-form" @submit.prevent="submit">
        <UiInput
          v-model="email"
          label="Email"
          type="email"
          autocomplete="username"
          required
        />
        <UiInput
          v-model="password"
          label="Password"
          type="password"
          autocomplete="current-password"
          required
        />
        <UiInlineBanner v-if="error" tone="danger" :title="error" />
        <UiButton type="submit" variant="primary" :loading="loading" block>
          Sign in
        </UiButton>
      </form>

      <!-- TOTP challenge step -->
      <form v-else class="login-form" @submit.prevent="submitCode">
        <p class="login-hint">
          Enter the 6-digit code from your authenticator app
          {{ useRecovery ? '— or switch back to use a recovery code.' : '.' }}
        </p>
        <UiInput
          v-if="!useRecovery"
          v-model="code"
          label="Authentication code"
          type="text"
          inputmode="numeric"
          autocomplete="one-time-code"
          maxlength="6"
          pattern="[0-9]{6}"
          required
        />
        <UiInput
          v-else
          v-model="recoveryCode"
          label="Recovery code"
          type="text"
          autocomplete="off"
          required
        />
        <UiInlineBanner v-if="error" tone="danger" :title="error" />
        <UiButton type="submit" variant="primary" :loading="loading" block>
          Verify
        </UiButton>
        <div class="login-actions">
          <button
            type="button"
            class="login-link"
            @click="useRecovery = !useRecovery"
          >
            {{ useRecovery ? 'Use authenticator code' : 'Use a recovery code instead' }}
          </button>
          <button type="button" class="login-link" @click="cancelChallenge">
            Cancel
          </button>
        </div>
      </form>
    </UiCard>
  </div>
</template>

<style scoped>
.login-page {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: var(--space-4);
  background: var(--color-sunken);
}
.login-card {
  width: 100%;
  max-width: 400px;
}
.login-brand {
  display: flex;
  align-items: center;
  gap: var(--space-3);
}
.login-brand__mark {
  width: 36px;
  height: 36px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  background: var(--color-primary);
  color: var(--color-on-primary);
  font-size: var(--font-size-xs);
  font-weight: 700;
  border-radius: var(--radius-md);
  letter-spacing: 0.04em;
}
.login-brand__title {
  margin: 0;
  font-size: var(--font-size-h3);
  font-weight: 600;
}
.login-form {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}
.login-hero {
  display: flex;
  justify-content: center;
  color: var(--color-text-subtle);
  margin-bottom: var(--space-3);
}
.login-hero :deep(.illustration) {
  width: 160px;
  max-width: 100%;
}
.login-hint {
  margin: 0;
  color: var(--color-text-subtle);
  font-size: var(--font-size-sm);
}
.login-actions {
  display: flex;
  justify-content: space-between;
  gap: var(--space-3);
}
.login-link {
  background: none;
  border: 0;
  padding: 0;
  color: var(--color-primary);
  font: inherit;
  text-decoration: underline;
  cursor: pointer;
}
/* Tight login card heights (≤ 400px) hide the hero to keep the form above the fold. */
@media (max-height: 400px) {
  .login-hero {
    display: none;
  }
}
</style>
