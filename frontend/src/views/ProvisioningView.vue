<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import QRCode from 'qrcode'
import { useAuthStore } from '@/stores/auth'
import UiButton from '@/components/ui/UiButton.vue'
import UiCard from '@/components/ui/UiCard.vue'
import UiInput from '@/components/ui/UiInput.vue'
import UiInlineBanner from '@/components/ui/UiInlineBanner.vue'

/**
 * First-run / forced-rotation provisioning flow.
 *
 * The backend's provisioning gate blocks every non-allowlisted call with a
 * 403 until the account has (a) rotated any temp/bootstrap password and
 * (b) — for super_admin — finished TOTP enrolment. This view walks the
 * user through both gates without forcing them to fish around the API by
 * hand. Stages are skipped if the corresponding gate is already cleared,
 * and the view bounces back to / when nothing is left to do.
 */
const auth = useAuthStore()
const router = useRouter()

type Stage = 'password' | 'two-factor' | 'recovery-codes' | 'done'

const stage = ref<Stage>('password')
const error = ref('')
const loading = ref(false)

// Password rotation form
const newPassword = ref('')
const confirmPassword = ref('')

// 2FA enrolment state
const enrolSecret = ref('')
const enrolURL = ref('')
const enrolQRDataURL = ref('')
const firstCode = ref('')
const recoveryCodes = ref<string[]>([])

const passwordMismatch = computed(
  () =>
    newPassword.value.length > 0 &&
    confirmPassword.value.length > 0 &&
    newPassword.value !== confirmPassword.value,
)

watch(enrolURL, async (url) => {
  if (!url) {
    enrolQRDataURL.value = ''
    return
  }
  try {
    enrolQRDataURL.value = await QRCode.toDataURL(url, {
      errorCorrectionLevel: 'M',
      margin: 1,
      width: 220,
    })
  } catch {
    // Best-effort QR; the secret + otpauth URL below remain usable so the
    // user can paste them into their authenticator manually.
    enrolQRDataURL.value = ''
  }
})

/**
 * Decide which stage to show. Called on mount and after each successful
 * step so we never linger on a screen whose gate is already cleared.
 */
async function pickStage() {
  if (!auth.user) {
    await router.replace('/login')
    return
  }
  // Stage 3 (recovery codes) is the only screen where provisioningRequired
  // has already flipped to false but we still need to keep the user here
  // long enough to copy their codes.
  if (stage.value === 'recovery-codes' && recoveryCodes.value.length > 0) {
    return
  }
  // Backend is the source of truth for whether we're done. This handles
  // the dev-bypass case (super_admin without TOTP, but the gate is waived)
  // where twoFactorEnrolled would still be false locally.
  if (!auth.provisioningRequired) {
    await router.replace('/')
    return
  }
  if (auth.user.must_change_password) {
    stage.value = 'password'
    return
  }
  if (auth.user.role === 'super_admin' && auth.twoFactorEnrolled === false) {
    if (!enrolSecret.value) {
      // Lazily start enrolment so revisits don't burn a fresh secret.
      await beginTwoFactor()
    } else {
      stage.value = 'two-factor'
    }
    return
  }
  // Defensive: gate says required but we don't know which one — kick to
  // home rather than render a broken screen.
  await router.replace('/')
}

async function submitPassword() {
  error.value = ''
  if (passwordMismatch.value) {
    error.value = 'Passwords do not match'
    return
  }
  loading.value = true
  try {
    await auth.rotatePassword(newPassword.value)
    newPassword.value = ''
    confirmPassword.value = ''
    await pickStage()
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to update password'
  } finally {
    loading.value = false
  }
}

async function beginTwoFactor() {
  error.value = ''
  loading.value = true
  try {
    const r = await auth.twoFactorEnrollStart()
    enrolSecret.value = r.secret
    enrolURL.value = r.otpauth_url
    stage.value = 'two-factor'
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to start 2FA enrolment'
  } finally {
    loading.value = false
  }
}

async function submitTwoFactor() {
  error.value = ''
  loading.value = true
  try {
    const r = await auth.twoFactorEnrollConfirm(enrolSecret.value, firstCode.value)
    recoveryCodes.value = r.recovery_codes
    stage.value = 'recovery-codes'
    firstCode.value = ''
    enrolSecret.value = ''
    enrolURL.value = ''
    enrolQRDataURL.value = ''
    await auth.fetchTwoFactorStatus()
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Could not verify the code'
  } finally {
    loading.value = false
  }
}

async function copyRecoveryCodes() {
  try {
    await navigator.clipboard.writeText(recoveryCodes.value.join('\n'))
  } catch {
    // Clipboard access can fail on insecure origins; the codes are still
    // visible on screen and the user can copy manually.
  }
}

async function finishProvisioning() {
  recoveryCodes.value = []
  stage.value = 'done'
  await auth.refreshMe()
  await router.replace('/')
}

async function logout() {
  try {
    await auth.logout()
  } finally {
    await router.replace('/login')
  }
}

onMounted(pickStage)
</script>

<template>
  <div class="provisioning-page">
    <UiCard class="provisioning-card">
      <template #header>
        <div class="provisioning-brand">
          <div class="provisioning-brand__mark" aria-hidden="true">PMS</div>
          <h1 class="provisioning-brand__title">
            {{
              stage === 'password'
                ? 'Set a new password'
                : stage === 'two-factor'
                  ? 'Enable two-factor authentication'
                  : stage === 'recovery-codes'
                    ? 'Save your recovery codes'
                    : 'Finishing up'
            }}
          </h1>
        </div>
      </template>

      <p v-if="auth.user" class="provisioning-hint">
        Signed in as <strong>{{ auth.user.email }}</strong
        >. You'll be redirected to the app once setup is complete.
      </p>

      <!-- Stage 1: rotate the bootstrap / admin-reset password. -->
      <form
        v-if="stage === 'password'"
        class="provisioning-form"
        @submit.prevent="submitPassword"
      >
        <p class="provisioning-hint">
          Your account is using a temporary password. Choose a new one with at
          least 12 characters before continuing.
        </p>
        <UiInput
          v-model="newPassword"
          label="New password"
          type="password"
          autocomplete="new-password"
          required
          minlength="12"
        />
        <UiInput
          v-model="confirmPassword"
          label="Confirm new password"
          type="password"
          autocomplete="new-password"
          required
          minlength="12"
        />
        <UiInlineBanner
          v-if="passwordMismatch"
          tone="warning"
          title="Passwords do not match"
        />
        <UiInlineBanner v-if="error" tone="danger" :title="error" />
        <UiButton
          type="submit"
          variant="primary"
          :loading="loading"
          :disabled="passwordMismatch || newPassword.length < 12"
          block
        >
          Save password
        </UiButton>
      </form>

      <!-- Stage 2: scan QR / enter authenticator code. -->
      <form
        v-else-if="stage === 'two-factor'"
        class="provisioning-form"
        @submit.prevent="submitTwoFactor"
      >
        <p class="provisioning-hint">
          Super-admin accounts must use a second factor. Scan this QR with
          Google Authenticator, 1Password, Authy, or any TOTP app, then enter
          the 6-digit code it displays.
        </p>
        <div v-if="enrolQRDataURL" class="provisioning-qr">
          <img :src="enrolQRDataURL" alt="TOTP enrolment QR code" />
        </div>
        <details v-if="enrolSecret" class="provisioning-secret">
          <summary>Can't scan? Show the secret</summary>
          <code class="provisioning-secret__value">{{ enrolSecret }}</code>
          <p class="provisioning-secret__hint">
            Paste this into your authenticator if scanning the QR isn't an
            option.
          </p>
        </details>
        <UiInput
          v-model="firstCode"
          label="Authentication code"
          type="text"
          inputmode="numeric"
          autocomplete="one-time-code"
          maxlength="6"
          pattern="[0-9]{6}"
          required
        />
        <UiInlineBanner v-if="error" tone="danger" :title="error" />
        <UiButton type="submit" variant="primary" :loading="loading" block>
          Confirm and enable
        </UiButton>
      </form>

      <!-- Stage 3: one-time display of recovery codes. -->
      <div
        v-else-if="stage === 'recovery-codes'"
        class="provisioning-form"
      >
        <p class="provisioning-hint">
          Store these codes somewhere safe — a password manager works well.
          Each one can be used once if you lose access to your authenticator.
          They will not be shown again.
        </p>
        <ul class="provisioning-codes" aria-label="Recovery codes">
          <li v-for="code in recoveryCodes" :key="code">{{ code }}</li>
        </ul>
        <div class="provisioning-actions">
          <UiButton variant="ghost" type="button" @click="copyRecoveryCodes">
            Copy to clipboard
          </UiButton>
          <UiButton
            variant="primary"
            type="button"
            :loading="loading"
            @click="finishProvisioning"
          >
            I've saved them — continue
          </UiButton>
        </div>
      </div>

      <button
        v-if="stage !== 'done'"
        type="button"
        class="provisioning-link"
        @click="logout"
      >
        Sign out and start over
      </button>
    </UiCard>
  </div>
</template>

<style scoped>
.provisioning-page {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: var(--space-4);
  background: var(--color-sunken);
}
.provisioning-card {
  width: 100%;
  max-width: 480px;
}
.provisioning-brand {
  display: flex;
  align-items: center;
  gap: var(--space-3);
}
.provisioning-brand__mark {
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
.provisioning-brand__title {
  margin: 0;
  font-size: var(--font-size-h3);
  font-weight: 600;
}
.provisioning-form {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}
.provisioning-hint {
  margin: 0;
  color: var(--color-text-subtle);
  font-size: var(--font-size-sm);
}
.provisioning-qr {
  display: flex;
  justify-content: center;
  background: white;
  padding: var(--space-3);
  border-radius: var(--radius-md);
  border: 1px solid var(--color-border);
}
.provisioning-qr img {
  width: 200px;
  height: 200px;
}
.provisioning-secret {
  font-size: var(--font-size-sm);
  color: var(--color-text-subtle);
}
.provisioning-secret__value {
  display: block;
  margin: var(--space-2) 0;
  padding: var(--space-2);
  background: var(--color-sunken);
  border-radius: var(--radius-sm);
  word-break: break-all;
  font-family: var(--font-family-mono, monospace);
}
.provisioning-codes {
  list-style: none;
  margin: 0;
  padding: var(--space-3);
  background: var(--color-sunken);
  border-radius: var(--radius-md);
  border: 1px solid var(--color-border);
  font-family: var(--font-family-mono, monospace);
  font-size: var(--font-size-sm);
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: var(--space-2);
}
.provisioning-actions {
  display: flex;
  gap: var(--space-3);
  justify-content: flex-end;
}
.provisioning-link {
  margin-top: var(--space-4);
  background: none;
  border: 0;
  padding: 0;
  color: var(--color-text-subtle);
  font: inherit;
  text-decoration: underline;
  cursor: pointer;
  align-self: flex-start;
}
</style>
