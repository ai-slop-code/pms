<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, RouterLink } from 'vue-router'
import { ChevronLeft } from 'lucide-vue-next'
import { api } from '@/api/http'
import UiPageHeader from '@/components/ui/UiPageHeader.vue'
import UiCard from '@/components/ui/UiCard.vue'
import UiSection from '@/components/ui/UiSection.vue'
import UiTabs from '@/components/ui/UiTabs.vue'
import UiInput from '@/components/ui/UiInput.vue'
import UiButton from '@/components/ui/UiButton.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import UiInlineBanner from '@/components/ui/UiInlineBanner.vue'

const route = useRoute()
const id = computed(() => Number(route.params.id))

const property = ref<Record<string, unknown> | null>(null)
const settings = ref<{ profile: Record<string, unknown>; integrations: Record<string, unknown> } | null>(null)
const tab = ref<'general' | 'settings'>('general')
const tabs = [
  { id: 'general', label: 'General' },
  { id: 'settings', label: 'Profile & integrations' },
]
const message = ref('')
const error = ref('')
const savingGeneral = ref(false)
const savingSettings = ref(false)

const form = ref({
  name: '',
  timezone: '',
  default_language: '',
  invoice_code: '',
  address_line1: '',
  city: '',
  postal_code: '',
  country: '',
  week_starts_on: 'monday' as 'monday' | 'sunday',
})

const profileForm = ref({
  legal_owner_name: '',
  billing_name: '',
  contact_phone: '',
  default_check_in_time: '',
  default_check_out_time: '',
  cleaner_nuki_auth_id: '',
  parking_instructions: '',
  wifi_ssid: '',
  wifi_password: '',
})

const secretsForm = ref({
  booking_ics_url: '',
  nuki_api_token: '',
  nuki_smartlock_id: '',
})

async function load() {
  error.value = ''
  property.value = null
  settings.value = null
  try {
    const p = await api<{ property: Record<string, unknown> }>(`/api/properties/${id.value}`)
    property.value = p.property
    form.value = {
      name: String(p.property.name ?? ''),
      timezone: String(p.property.timezone ?? ''),
      default_language: String(p.property.default_language ?? ''),
      invoice_code: String(p.property.invoice_code ?? ''),
      address_line1: String(p.property.address_line1 ?? ''),
      city: String(p.property.city ?? ''),
      postal_code: String(p.property.postal_code ?? ''),
      country: String(p.property.country ?? ''),
      week_starts_on: p.property.week_starts_on === 'sunday' ? 'sunday' : 'monday',
    }
    const s = await api<{ profile: Record<string, unknown>; integrations: Record<string, unknown> }>(
      `/api/properties/${id.value}/settings`
    )
    settings.value = s
    profileForm.value = {
      legal_owner_name: String(s.profile.legal_owner_name ?? ''),
      billing_name: String(s.profile.billing_name ?? ''),
      contact_phone: String(s.profile.contact_phone ?? ''),
      default_check_in_time: String(s.profile.default_check_in_time ?? ''),
      default_check_out_time: String(s.profile.default_check_out_time ?? ''),
      cleaner_nuki_auth_id: String(s.profile.cleaner_nuki_auth_id ?? ''),
      parking_instructions: String(s.profile.parking_instructions ?? ''),
      wifi_ssid: String(s.profile.wifi_ssid ?? ''),
      wifi_password: '',
    }
    secretsForm.value = { booking_ics_url: '', nuki_api_token: '', nuki_smartlock_id: '' }
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to load property details'
  }
}

async function saveGeneral() {
  message.value = ''
  error.value = ''
  savingGeneral.value = true
  try {
    await api(`/api/properties/${id.value}`, {
      method: 'PATCH',
      json: {
        name: form.value.name,
        timezone: form.value.timezone,
        default_language: form.value.default_language,
        invoice_code: form.value.invoice_code.trim() || null,
        address_line1: form.value.address_line1 || null,
        city: form.value.city || null,
        postal_code: form.value.postal_code || null,
        country: form.value.country || null,
        week_starts_on: form.value.week_starts_on,
      },
    })
    message.value = 'General details saved.'
    await load()
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to save general details'
  } finally {
    savingGeneral.value = false
  }
}

async function saveSettings() {
  message.value = ''
  error.value = ''
  savingSettings.value = true
  try {
    const profile: Record<string, unknown> = {
      legal_owner_name: profileForm.value.legal_owner_name || null,
      billing_name: profileForm.value.billing_name || null,
      contact_phone: profileForm.value.contact_phone || null,
      default_check_in_time: profileForm.value.default_check_in_time,
      default_check_out_time: profileForm.value.default_check_out_time,
      cleaner_nuki_auth_id: profileForm.value.cleaner_nuki_auth_id || null,
      parking_instructions: profileForm.value.parking_instructions || null,
      wifi_ssid: profileForm.value.wifi_ssid || null,
    }
    if (profileForm.value.wifi_password) {
      profile.wifi_password = profileForm.value.wifi_password
    }
    const secrets: Record<string, unknown> = {}
    if (secretsForm.value.booking_ics_url !== '') secrets.booking_ics_url = secretsForm.value.booking_ics_url
    if (secretsForm.value.nuki_api_token !== '') secrets.nuki_api_token = secretsForm.value.nuki_api_token
    if (secretsForm.value.nuki_smartlock_id !== '') secrets.nuki_smartlock_id = secretsForm.value.nuki_smartlock_id
    await api(`/api/properties/${id.value}/settings`, {
      method: 'PATCH',
      json: {
        profile,
        secrets: Object.keys(secrets).length ? secrets : undefined,
      },
    })
    message.value = 'Profile & integrations saved.'
    profileForm.value.wifi_password = ''
    secretsForm.value = { booking_ics_url: '', nuki_api_token: '', nuki_smartlock_id: '' }
    await load()
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to save property settings'
  } finally {
    savingSettings.value = false
  }
}

onMounted(load)
watch(id, load)
</script>

<template>
  <div>
    <UiPageHeader
      :title="String(property?.name || 'Property')"
      :lede="property ? 'Manage details, profile, and integration secrets for this property.' : ''"
    >
      <template #meta>
        <RouterLink to="/properties" class="back-link">
          <ChevronLeft :size="14" aria-hidden="true" />
          <span>All properties</span>
        </RouterLink>
      </template>
    </UiPageHeader>

    <UiInlineBanner v-if="error" tone="danger" :title="error" />
    <UiInlineBanner v-if="message" tone="success" :title="message" />

    <UiTabs v-model="tab" :tabs="tabs" aria-label="Property sections">
      <template #default="{ active }">
        <div v-if="active === 'general' && property">
          <UiCard>
            <form class="form-grid" @submit.prevent="saveGeneral">
              <UiInput v-model="form.name" label="Name" required />
              <UiInput v-model="form.timezone" label="Timezone" help="IANA name, e.g. Europe/Bratislava" />
              <UiInput v-model="form.default_language" label="Default language" help="Two-letter code (sk, en, de, …)" />
              <UiInput
                v-model="form.invoice_code"
                label="Invoice number prefix (optional)"
                placeholder="e.g. APT01 — used as APT01/2026/0001"
              />
              <UiInput v-model="form.address_line1" label="Address line" />
              <UiInput v-model="form.city" label="City" />
              <UiInput v-model="form.postal_code" label="Postal code" />
              <UiInput v-model="form.country" label="Country" />

              <fieldset class="form-grid__full radio-group">
                <legend class="radio-group__label">Week starts on</legend>
                <label class="radio-row">
                  <input v-model="form.week_starts_on" type="radio" value="monday" />
                  <span>Monday</span>
                </label>
                <label class="radio-row">
                  <input v-model="form.week_starts_on" type="radio" value="sunday" />
                  <span>Sunday</span>
                </label>
              </fieldset>

              <div class="form-grid__full form-actions">
                <UiButton type="submit" variant="primary" :loading="savingGeneral">Save general details</UiButton>
              </div>
            </form>
          </UiCard>
        </div>

        <div v-else-if="active === 'settings' && settings">
          <UiCard>
            <div class="integrations-summary">
              <span class="muted">Integrations:</span>
              <UiBadge :tone="settings.integrations.booking_ics_configured ? 'success' : 'neutral'" dot>
                ICS {{ settings.integrations.booking_ics_configured ? 'configured' : 'off' }}
              </UiBadge>
              <UiBadge :tone="settings.integrations.nuki_configured ? 'success' : 'neutral'" dot>
                Nuki {{ settings.integrations.nuki_configured ? 'configured' : 'off' }}
              </UiBadge>
            </div>

            <form @submit.prevent="saveSettings">
              <UiSection title="Profile" description="Owner / contact details and stay defaults shown to guests.">
                <div class="form-grid">
                  <UiInput v-model="profileForm.legal_owner_name" label="Legal owner name" />
                  <UiInput v-model="profileForm.billing_name" label="Billing name" />
                  <UiInput v-model="profileForm.contact_phone" label="Contact phone" />
                  <UiInput
                    v-model="profileForm.cleaner_nuki_auth_id"
                    label="Cleaner Nuki Auth ID"
                    help="Used for cleaning log reconciliation"
                  />
                  <UiInput
                    v-model="profileForm.default_check_in_time"
                    label="Default check-in time"
                    type="time"
                  />
                  <UiInput
                    v-model="profileForm.default_check_out_time"
                    label="Default check-out time"
                    type="time"
                  />
                  <UiInput v-model="profileForm.wifi_ssid" label="Wi-Fi SSID" />
                  <UiInput
                    v-model="profileForm.wifi_password"
                    label="Wi-Fi password"
                    type="password"
                    autocomplete="new-password"
                    help="Leave blank to keep the current value."
                  />
                  <label class="textarea-field form-grid__full">
                    <span class="textarea-field__label">Parking instructions</span>
                    <textarea v-model="profileForm.parking_instructions" rows="3" />
                  </label>
                </div>
              </UiSection>

              <UiSection title="Secrets" description="Values are write-only — leave a field blank to keep the existing value.">
                <div class="form-grid">
                  <UiInput
                    v-model="secretsForm.booking_ics_url"
                    label="Booking.com ICS URL"
                    type="url"
                    placeholder="https://..."
                    autocomplete="off"
                  />
                  <UiInput
                    v-model="secretsForm.nuki_api_token"
                    label="Nuki API token"
                    type="password"
                    autocomplete="new-password"
                  />
                  <UiInput
                    v-model="secretsForm.nuki_smartlock_id"
                    label="Nuki smart lock ID"
                    placeholder="Numeric smartlockId (not authId)"
                  />
                </div>
              </UiSection>

              <div class="form-actions">
                <UiButton type="submit" variant="primary" :loading="savingSettings">
                  Save profile & integrations
                </UiButton>
              </div>
            </form>
          </UiCard>
        </div>
      </template>
    </UiTabs>
  </div>
</template>

<style scoped>
.back-link {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  color: var(--color-text-muted);
  font-size: var(--font-size-sm);
}
.back-link:hover {
  color: var(--color-primary);
  text-decoration: none;
}

.form-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: var(--space-4);
}
.form-grid__full {
  grid-column: 1 / -1;
}

.form-actions {
  margin-top: var(--space-4);
  display: flex;
  justify-content: flex-end;
  gap: var(--space-2);
}

.radio-group {
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  padding: var(--space-3) var(--space-4);
  margin: 0;
  display: flex;
  align-items: center;
  gap: var(--space-4);
  flex-wrap: wrap;
}
.radio-group__label {
  font-size: var(--font-size-sm);
  color: var(--color-text-muted);
  font-weight: 500;
  padding: 0;
}
.radio-row {
  display: inline-flex;
  align-items: center;
  gap: var(--space-2);
  font-size: var(--font-size-md);
  color: var(--color-text);
  margin: 0;
  font-weight: 500;
}
.radio-row input[type='radio'] {
  width: auto;
  min-height: 0;
  margin: 0;
}

.textarea-field {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  margin: 0;
}
.textarea-field__label {
  font-size: var(--font-size-sm);
  color: var(--color-text-muted);
  font-weight: 500;
}
.textarea-field textarea {
  width: 100%;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  padding: var(--space-3);
  font: var(--font-size-md) / 1.5 var(--font-family-sans);
  color: var(--color-text);
  background: var(--color-surface);
  resize: vertical;
}
.textarea-field textarea:focus {
  border-color: var(--color-primary);
  box-shadow: var(--focus-ring);
  outline: none;
}

.integrations-summary {
  display: inline-flex;
  align-items: center;
  gap: var(--space-2);
  margin-bottom: var(--space-4);
  font-size: var(--font-size-sm);
  flex-wrap: wrap;
}
.muted {
  color: var(--color-text-muted);
}
</style>
