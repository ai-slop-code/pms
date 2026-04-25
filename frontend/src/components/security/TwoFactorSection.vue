<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import QRCode from 'qrcode'
import { useAuthStore } from '@/stores/auth'
import UiCard from '@/components/ui/UiCard.vue'
import UiSection from '@/components/ui/UiSection.vue'
import UiButton from '@/components/ui/UiButton.vue'
import UiInput from '@/components/ui/UiInput.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import UiInlineBanner from '@/components/ui/UiInlineBanner.vue'

const auth = useAuthStore()

type Stage =
  | 'idle' // show enrol / disable buttons
  | 'enroll' // showing the QR / secret and waiting for the first code
  | 'codes' // showing the freshly generated recovery codes
  | 'disable' // showing the disable password prompt

const stage = ref<Stage>('idle')
const enrolled = ref(false)
const remaining = ref(0)
const error = ref('')
const loading = ref(false)

const enrolSecret = ref('')
const enrolURL = ref('')
const enrolQRDataURL = ref('')
const firstCode = ref('')
const recoveryCodes = ref<string[]>([])
const disablePassword = ref('')

const otpauthDisplay = computed(() => enrolURL.value)

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
    // QR rendering is best-effort; the secret + otpauth link below remain usable.
    enrolQRDataURL.value = ''
  }
})

async function refresh() {
  try {
    const r = await auth.twoFactorStatus()
    enrolled.value = r.enrolled
    remaining.value = r.recovery_codes_remaining
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to load 2FA status'
  }
}

onMounted(refresh)

async function startEnrol() {
  error.value = ''
  loading.value = true
  try {
    const r = await auth.twoFactorEnrollStart()
    enrolSecret.value = r.secret
    enrolURL.value = r.otpauth_url
    stage.value = 'enroll'
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to start enrolment'
  } finally {
    loading.value = false
  }
}

async function confirmEnrol() {
  error.value = ''
  loading.value = true
  try {
    const r = await auth.twoFactorEnrollConfirm(enrolSecret.value, firstCode.value)
    recoveryCodes.value = r.recovery_codes
    stage.value = 'codes'
    firstCode.value = ''
    enrolSecret.value = ''
    enrolURL.value = ''
    enrolQRDataURL.value = ''
    await refresh()
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to enable 2FA'
  } finally {
    loading.value = false
  }
}

function cancelEnrol() {
  enrolSecret.value = ''
  enrolURL.value = ''
  enrolQRDataURL.value = ''
  firstCode.value = ''
  stage.value = 'idle'
  error.value = ''
}

function backToIdle() {
  stage.value = 'idle'
  recoveryCodes.value = []
}

async function confirmDisable() {
  error.value = ''
  loading.value = true
  try {
    await auth.twoFactorDisable(disablePassword.value)
    disablePassword.value = ''
    stage.value = 'idle'
    await refresh()
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to disable 2FA'
  } finally {
    loading.value = false
  }
}

async function copyRecoveryCodes() {
  try {
    await navigator.clipboard.writeText(recoveryCodes.value.join('\n'))
  } catch {
    // Clipboard may be unavailable (insecure origin / older browsers).
    // Users can still select and copy the codes manually.
  }
}
</script>

<template>
  <UiSection
    title="Two-factor authentication"
    description="Protect your account with a TOTP authenticator app (Google Authenticator, 1Password, Authy…)."
  >
    <UiCard>
      <UiInlineBanner v-if="error" tone="danger" :title="error" />

      <div v-if="stage === 'idle'" class="tfa-body">
        <div class="tfa-status">
          <UiBadge :tone="enrolled ? 'success' : 'neutral'">
            {{ enrolled ? 'Enabled' : 'Disabled' }}
          </UiBadge>
          <span v-if="enrolled" class="tfa-meta">
            {{ remaining }} recovery code{{ remaining === 1 ? '' : 's' }} remaining
          </span>
        </div>
        <div class="tfa-actions">
          <UiButton v-if="!enrolled" variant="primary" :loading="loading" @click="startEnrol">
            Enable 2FA
          </UiButton>
          <template v-else>
            <UiButton variant="secondary" @click="stage = 'disable'">Disable 2FA</UiButton>
          </template>
        </div>
      </div>

      <div v-else-if="stage === 'enroll'" class="tfa-body">
        <p class="tfa-hint">
          Scan the QR code with your authenticator app, or paste this secret manually:
        </p>
        <img
          v-if="enrolQRDataURL"
          :src="enrolQRDataURL"
          alt="TOTP QR code"
          class="tfa-qr"
          width="220"
          height="220"
        />
        <code class="tfa-secret">{{ enrolSecret }}</code>
        <p class="tfa-hint">
          <a :href="otpauthDisplay" class="tfa-link">Open in authenticator app</a>
        </p>
        <p class="tfa-hint">Enter the 6-digit code the app shows to finish enrolment.</p>
        <UiInput
          v-model="firstCode"
          label="Authentication code"
          inputmode="numeric"
          autocomplete="one-time-code"
          maxlength="6"
          pattern="[0-9]{6}"
        />
        <div class="tfa-actions">
          <UiButton variant="secondary" @click="cancelEnrol">Cancel</UiButton>
          <UiButton variant="primary" :loading="loading" @click="confirmEnrol">
            Verify and enable
          </UiButton>
        </div>
      </div>

      <div v-else-if="stage === 'codes'" class="tfa-body">
        <UiInlineBanner
          tone="warning"
          title="Save your recovery codes"
          :description="'These codes are shown only once. Each code works a single time and lets you sign in without your authenticator app.'"
        />
        <ul class="tfa-codes">
          <li v-for="c in recoveryCodes" :key="c">
            <code>{{ c }}</code>
          </li>
        </ul>
        <div class="tfa-actions">
          <UiButton variant="secondary" @click="copyRecoveryCodes">Copy to clipboard</UiButton>
          <UiButton variant="primary" @click="backToIdle">I saved them</UiButton>
        </div>
      </div>

      <div v-else-if="stage === 'disable'" class="tfa-body">
        <p class="tfa-hint">
          Confirm your password to disable 2FA. Your authenticator enrolment and all
          recovery codes will be removed.
        </p>
        <UiInput
          v-model="disablePassword"
          label="Password"
          type="password"
          autocomplete="current-password"
        />
        <div class="tfa-actions">
          <UiButton variant="secondary" @click="stage = 'idle'">Cancel</UiButton>
          <UiButton variant="primary" :loading="loading" @click="confirmDisable">
            Disable 2FA
          </UiButton>
        </div>
      </div>
    </UiCard>
  </UiSection>
</template>

<style scoped>
.tfa-body {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}
.tfa-status {
  display: flex;
  align-items: center;
  gap: var(--space-3);
}
.tfa-meta {
  color: var(--color-text-muted);
  font-size: var(--font-size-sm);
}
.tfa-hint {
  margin: 0;
  color: var(--color-text-muted);
  font-size: var(--font-size-sm);
}
.tfa-qr {
  display: block;
  width: 220px;
  height: 220px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-sm);
  background: #fff;
  padding: var(--space-2);
}
.tfa-secret {
  display: block;
  padding: var(--space-2) var(--space-3);
  background: var(--color-sunken);
  border-radius: var(--radius-sm);
  font-family: var(--font-mono);
  word-break: break-all;
}
.tfa-link {
  color: var(--color-primary);
  text-decoration: underline;
}
.tfa-codes {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
  gap: var(--space-2);
  list-style: none;
  margin: 0;
  padding: 0;
}
.tfa-codes code {
  display: block;
  padding: var(--space-2);
  background: var(--color-sunken);
  border-radius: var(--radius-sm);
  font-family: var(--font-mono);
  text-align: center;
}
.tfa-actions {
  display: flex;
  justify-content: flex-end;
  gap: var(--space-3);
}
</style>
