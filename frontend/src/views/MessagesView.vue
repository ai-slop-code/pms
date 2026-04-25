<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Check, Plus, X } from 'lucide-vue-next'
import { api } from '@/api/http'
import { useCurrentProperty } from '@/composables/useCurrentProperty'
import { useCopyFeedback } from '@/composables/useCopyFeedback'
import { useConfirm } from '@/composables/useConfirm'
import { useTransientMessage } from '@/composables/useTransientMessage'
import UiPageHeader from '@/components/ui/UiPageHeader.vue'
import UiTabs from '@/components/ui/UiTabs.vue'
import UiCard from '@/components/ui/UiCard.vue'
import UiSection from '@/components/ui/UiSection.vue'
import UiTable from '@/components/ui/UiTable.vue'
import UiInput from '@/components/ui/UiInput.vue'
import UiSelect from '@/components/ui/UiSelect.vue'
import UiButton from '@/components/ui/UiButton.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import UiInlineBanner from '@/components/ui/UiInlineBanner.vue'
import UiEmptyState from '@/components/ui/UiEmptyState.vue'
import type {
  MessageTemplate,
  CleaningMessageResponse,
  MessagesOccupancy as Occupancy,
  RenderedMessage,
  GenerateMessagesResponse as GenerateResponse,
} from '@/api/types/messages'

const TEMPLATE_TYPE_LABELS: Record<string, string> = {
  check_in: 'Check-in',
  cleaning_staff: 'Cleaning staff',
}

const LANG_LABELS: Record<string, string> = {
  en: 'English',
  sk: 'Slovenčina',
  de: 'Deutsch',
  uk: 'Українська',
  hu: 'Magyar',
}

const { pid } = useCurrentProperty()
const { confirm } = useConfirm()

const tab = ref<'generate' | 'templates'>('generate')
const tabs = [
  { id: 'generate', label: 'Generate' },
  { id: 'templates', label: 'Templates' },
]

const loading = ref(false)
const error = ref('')
const { message: success, show: showSuccess } = useTransientMessage()
const templates = ref<MessageTemplate[]>([])
const supportedLanguages = ref<string[]>([])
const supportedPlaceholders = ref<string[]>([])

const occupancies = ref<Occupancy[]>([])
const selectedOccupancyId = ref<number | null>(null)
const generating = ref(false)
const generatedMessages = ref<RenderedMessage[]>([])
const nukiAvailable = ref(true)
// `copiedLang` doubles as the flash key (the rendered message's language code)
// so the per-row "Copied" button uses `copiedLang === msg.language_code`.
const { copied: copiedLang, flash: flashLang } = useCopyFeedback(2000)

const cleaningMessage = ref<CleaningMessageResponse | null>(null)
const cleaningLoading = ref(false)
// Truthy/null flag for the single cleaning-message copy button.
const { copied: cleaningCopied, flash: flashCleaning } = useCopyFeedback(2000)

const occSearch = ref('')
const occDropdownOpen = ref(false)
const filteredOccupancies = computed(() => {
  const q = occSearch.value.toLowerCase().trim()
  if (!q) return occupancies.value
  return occupancies.value.filter((o) => occLabel(o).toLowerCase().includes(q))
})
const selectedOccLabel = computed(() => {
  if (!selectedOccupancyId.value) return ''
  const occ = occupancies.value.find((o) => o.id === selectedOccupancyId.value)
  return occ ? occLabel(occ) : ''
})

function selectOccupancy(occ: Occupancy) {
  selectedOccupancyId.value = occ.id
  occSearch.value = ''
  occDropdownOpen.value = false
}

function onOccSearchFocus() {
  occDropdownOpen.value = true
  occSearch.value = ''
}

function onOccBlur() {
  // All clickable children in the dropdown use `@mousedown.prevent`, so the
  // input never loses focus to them — a blur here always means focus moved
  // outside the widget (tab key or outside click) and we can close cleanly.
  occDropdownOpen.value = false
  occSearch.value = ''
}

function clearSelection() {
  selectedOccupancyId.value = null
  occSearch.value = ''
  generatedMessages.value = []
}

const editingTemplate = ref<MessageTemplate | null>(null)
const editTitle = ref('')
const editBody = ref('')
const saving = ref(false)

const showNewForm = ref(false)
const newLang = ref('en')
const newTitle = ref('')
const creating = ref(false)

async function loadData() {
  if (!pid.value) return
  loading.value = true
  error.value = ''
  try {
    const [tplRes, occRes] = await Promise.all([
      api<{
        templates: MessageTemplate[]
        supported_languages: string[]
        supported_placeholders: string[]
      }>(`/api/properties/${pid.value}/message-templates`),
      api<{ occupancies: Occupancy[] }>(
        `/api/properties/${pid.value}/occupancies?limit=200&status=active`
      ).catch(() => ({ occupancies: [] as Occupancy[] })),
    ])
    templates.value = tplRes.templates
    supportedLanguages.value = tplRes.supported_languages
    supportedPlaceholders.value = tplRes.supported_placeholders
    const now = new Date()
    occupancies.value = (occRes.occupancies ?? [])
      .filter((o) => new Date(o.end_at) >= now)
      .sort((a, b) => new Date(a.start_at).getTime() - new Date(b.start_at).getTime())
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to load data'
  } finally {
    loading.value = false
  }
}

watch(pid, loadData, { immediate: true })

function occLabel(occ: Occupancy) {
  const name = occ.guest_display_name || occ.raw_summary || `#${occ.id}`
  const start = fmtDate(occ.start_at)
  const end = fmtDate(occ.end_at)
  return `${name} (${start} → ${end})`
}

function fmtDate(iso: string) {
  try {
    return new Date(iso).toLocaleDateString('en-GB', { day: '2-digit', month: '2-digit', year: 'numeric' })
  } catch { return iso }
}

function fmtDateTime(iso: string) {
  try {
    return new Date(iso).toLocaleString('en-GB', {
      day: '2-digit', month: '2-digit', year: 'numeric', hour: '2-digit', minute: '2-digit',
    })
  } catch { return iso }
}

async function generateMessages() {
  if (!pid.value || !selectedOccupancyId.value) return
  generating.value = true
  error.value = ''
  generatedMessages.value = []
  try {
    const res = await api<GenerateResponse>(
      `/api/properties/${pid.value}/messages/generate?occupancy_id=${selectedOccupancyId.value}`
    )
    generatedMessages.value = res.messages
    nukiAvailable.value = res.nuki_available
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Generation failed'
  } finally {
    generating.value = false
  }
}

async function copyToClipboard(msg: RenderedMessage) {
  try {
    await navigator.clipboard.writeText(msg.body)
    flashLang(msg.language_code)
  } catch {
    error.value = 'Failed to copy to clipboard'
  }
}

async function generateCleaningMessage() {
  if (!pid.value) return
  cleaningLoading.value = true
  error.value = ''
  cleaningMessage.value = null
  try {
    cleaningMessage.value = await api<CleaningMessageResponse>(
      `/api/properties/${pid.value}/messages/cleaning`
    )
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to generate cleaning message'
  } finally {
    cleaningLoading.value = false
  }
}

async function copyCleaningMessage() {
  if (!cleaningMessage.value) return
  try {
    await navigator.clipboard.writeText(cleaningMessage.value.body)
    flashCleaning(1)
  } catch {
    error.value = 'Failed to copy to clipboard'
  }
}

function startEdit(t: MessageTemplate) {
  editingTemplate.value = t
  editTitle.value = t.title
  editBody.value = t.body
}

function cancelEdit() { editingTemplate.value = null }

async function saveTemplate() {
  if (!pid.value || !editingTemplate.value) return
  saving.value = true
  error.value = ''
  try {
    await api(`/api/properties/${pid.value}/message-templates/${editingTemplate.value.id}`, {
      method: 'PATCH',
      json: { title: editTitle.value, body: editBody.value },
    })
    editingTemplate.value = null
    await loadData()
    showSuccess('Template saved')
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Save failed'
  } finally {
    saving.value = false
  }
}

async function toggleActive(t: MessageTemplate) {
  if (!pid.value) return
  try {
    await api(`/api/properties/${pid.value}/message-templates/${t.id}`, {
      method: 'PATCH',
      json: { active: !t.active },
    })
    await loadData()
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Toggle failed'
  }
}

function insertPlaceholder(placeholder: string) {
  editBody.value += `{{${placeholder}}}`
}

async function createTemplate() {
  if (!pid.value || !newTitle.value.trim()) return
  creating.value = true
  error.value = ''
  try {
    await api(`/api/properties/${pid.value}/message-templates`, {
      method: 'POST',
      json: { language_code: newLang.value, title: newTitle.value.trim() },
    })
    showNewForm.value = false
    newTitle.value = ''
    await loadData()
    showSuccess('Template created')
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Create failed'
  } finally {
    creating.value = false
  }
}

async function deleteTemplate(t: MessageTemplate) {
  if (!pid.value) return
  const ok = await confirm({
    title: 'Delete template',
    message: `Delete template “${t.title}” (${t.language_code.toUpperCase()})?`,
    confirmLabel: 'Delete',
    tone: 'danger',
  })
  if (!ok) return
  try {
    await api(`/api/properties/${pid.value}/message-templates/${t.id}`, { method: 'DELETE' })
    await loadData()
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Delete failed'
  }
}
</script>

<template>
  <div>
    <UiPageHeader
      title="Customer messages"
      lede="Generate guest-ready messages for upcoming stays and edit reusable templates."
    />

    <UiEmptyState
      v-if="!pid"
      illustration="dashboard"
      title="Pick a property"
      description="Use the property switcher in the topbar to load message templates."
    />

    <template v-else>
      <UiInlineBanner v-if="error" tone="danger" :title="error" />
      <UiInlineBanner v-if="success" tone="success" :title="success" />

      <UiTabs v-model="tab" :tabs="tabs" aria-label="Message views">
        <template #default="{ active }">
          <!-- Generate -->
          <div v-if="active === 'generate'" class="stack">
            <UiSection title="Stay message" description="Pick an upcoming stay and generate localised guest messages.">
              <UiCard>
                <div class="field">
                  <label class="field__label" for="occ-picker">Select a stay</label>
                  <div class="occ-picker" :class="{ open: occDropdownOpen }">
                    <div class="occ-input-wrap">
                      <input
                        id="occ-picker"
                        class="occ-search"
                        :placeholder="selectedOccLabel || (occupancies.length ? 'Search stays…' : 'No upcoming stays')"
                        :value="occDropdownOpen ? occSearch : selectedOccLabel"
                        :disabled="generating || !occupancies.length"
                        @focus="onOccSearchFocus"
                        @blur="onOccBlur"
                        @input="occSearch = ($event.target as HTMLInputElement).value"
                      />
                      <button
                        v-if="selectedOccupancyId && !occDropdownOpen"
                        type="button"
                        class="occ-clear"
                        aria-label="Clear selection"
                        @mousedown.prevent="clearSelection"
                      ><X :size="14" aria-hidden="true" /></button>
                    </div>
                    <ul v-if="occDropdownOpen && filteredOccupancies.length" class="occ-list">
                      <li
                        v-for="occ in filteredOccupancies"
                        :key="occ.id"
                        class="occ-option"
                        :class="{ selected: occ.id === selectedOccupancyId }"
                        @mousedown.prevent="selectOccupancy(occ)"
                      >
                        <span class="occ-name">{{ occ.guest_display_name || occ.raw_summary || `#${occ.id}` }}</span>
                        <span class="occ-dates">{{ fmtDate(occ.start_at) }} → {{ fmtDate(occ.end_at) }}</span>
                      </li>
                    </ul>
                    <div v-if="occDropdownOpen && occSearch && !filteredOccupancies.length" class="occ-list occ-empty">
                      No stays matching "{{ occSearch }}"
                    </div>
                  </div>
                </div>
                <div class="actions">
                  <UiButton
                    variant="primary"
                    :disabled="!selectedOccupancyId"
                    :loading="generating"
                    @click="generateMessages"
                  >Generate messages</UiButton>
                </div>
              </UiCard>

              <UiInlineBanner
                v-if="generatedMessages.length && !nukiAvailable"
                tone="warning"
                title="Nuki access code missing"
              >
                No Nuki access code available for this stay. The code placeholder shows "—" in the messages below.
              </UiInlineBanner>

              <UiCard
                v-for="msg in generatedMessages"
                :key="msg.language_code"
                class="msg-card"
              >
                <div class="msg-header">
                  <div class="msg-lang">
                    <UiBadge tone="info">{{ msg.language_code.toUpperCase() }}</UiBadge>
                    <span class="lang-name">{{ LANG_LABELS[msg.language_code] || msg.language_code }}</span>
                  </div>
                  <UiButton
                    :variant="copiedLang === msg.language_code ? 'primary' : 'secondary'"
                    size="sm"
                    @click="copyToClipboard(msg)"
                  >
                    <template v-if="copiedLang === msg.language_code" #iconLeft>
                      <Check :size="14" aria-hidden="true" />
                    </template>
                    {{ copiedLang === msg.language_code ? 'Copied' : 'Copy' }}
                  </UiButton>
                </div>
                <h4 class="msg-title">{{ msg.title }}</h4>
                <pre class="msg-body">{{ msg.body }}</pre>
              </UiCard>
            </UiSection>

            <UiSection
              title="Cleaning schedule"
              description="WhatsApp-ready message for the cleaning staff with all upcoming stays from today onwards."
            >
              <UiCard>
                <div class="actions">
                  <UiButton variant="primary" :loading="cleaningLoading" @click="generateCleaningMessage">
                    Generate cleaning message
                  </UiButton>
                </div>
              </UiCard>

              <UiCard v-if="cleaningMessage" class="msg-card">
                <div class="msg-header">
                  <div class="msg-lang">
                    <UiBadge tone="info">{{ cleaningMessage.language_code.toUpperCase() }}</UiBadge>
                    <span class="lang-name">
                      {{ LANG_LABELS[cleaningMessage.language_code] || cleaningMessage.language_code }}
                      · {{ cleaningMessage.stays_count }} stay{{ cleaningMessage.stays_count === 1 ? '' : 's' }}
                    </span>
                  </div>
                  <UiButton
                    :variant="cleaningCopied ? 'primary' : 'secondary'"
                    size="sm"
                    @click="copyCleaningMessage"
                  >
                    <template v-if="cleaningCopied" #iconLeft>
                      <Check :size="14" aria-hidden="true" />
                    </template>
                    {{ cleaningCopied ? 'Copied' : 'Copy' }}
                  </UiButton>
                </div>
                <h4 class="msg-title">{{ cleaningMessage.title }}</h4>
                <pre class="msg-body">{{ cleaningMessage.body }}</pre>
              </UiCard>
            </UiSection>
          </div>

          <!-- Templates -->
          <div v-else>
            <UiSection
              v-if="editingTemplate"
              :title="`Edit template — ${LANG_LABELS[editingTemplate.language_code] || editingTemplate.language_code}`"
            >
              <UiCard>
                <UiInput v-model="editTitle" label="Title" />
                <label class="field" style="margin-top: var(--space-3)">
                  <span class="field__label">Body</span>
                  <textarea v-model="editBody" rows="14" class="template-textarea" />
                </label>
                <div class="placeholders-bar">
                  <span class="ph-label">Insert placeholder:</span>
                  <button
                    v-for="ph in supportedPlaceholders"
                    :key="ph"
                    type="button"
                    class="ph-btn"
                    @click="insertPlaceholder(ph)"
                  >{{ ph }}</button>
                </div>
                <div class="actions actions--right">
                  <UiButton variant="ghost" @click="cancelEdit">Cancel</UiButton>
                  <UiButton variant="primary" :loading="saving" @click="saveTemplate">Save template</UiButton>
                </div>
              </UiCard>
            </UiSection>

            <UiSection v-else-if="showNewForm" title="New template">
              <UiCard>
                <div class="new-tpl-form">
                  <UiSelect v-model="newLang" label="Language">
                    <option v-for="lang in supportedLanguages" :key="lang" :value="lang">
                      {{ lang.toUpperCase() }} — {{ LANG_LABELS[lang] || lang }}
                    </option>
                  </UiSelect>
                  <UiInput v-model="newTitle" label="Title" placeholder="e.g. House rules, late check-in…" />
                  <div class="actions actions--right">
                    <UiButton variant="ghost" @click="showNewForm = false">Cancel</UiButton>
                    <UiButton
                      variant="primary"
                      :disabled="!newTitle.trim()"
                      :loading="creating"
                      @click="createTemplate"
                    >Create template</UiButton>
                  </div>
                </div>
              </UiCard>
            </UiSection>

            <UiSection v-else title="Message templates">
              <template #actions>
                <UiButton variant="primary" size="sm" @click="showNewForm = true">
                  <template #iconLeft><Plus :size="14" aria-hidden="true" /></template>
                  New template
                </UiButton>
              </template>

              <UiTable
                :empty="!loading && !templates.length"
                empty-text="No templates yet — they will be initialized automatically."
              >
                <template #head>
                  <tr>
                    <th>Type</th>
                    <th>Language</th>
                    <th>Title</th>
                    <th>Status</th>
                    <th>Updated</th>
                    <th aria-label="Actions" />
                  </tr>
                </template>
                <tr v-for="t in templates" :key="t.id">
                  <td>
                    <UiBadge :tone="t.template_type === 'cleaning_staff' ? 'success' : 'info'">
                      {{ TEMPLATE_TYPE_LABELS[t.template_type] || t.template_type }}
                    </UiBadge>
                  </td>
                  <td>
                    <UiBadge tone="neutral">{{ t.language_code.toUpperCase() }}</UiBadge>
                    <span class="muted lang-inline">{{ LANG_LABELS[t.language_code] || t.language_code }}</span>
                  </td>
                  <td>{{ t.title }}</td>
                  <td>
                    <button
                      type="button"
                      class="toggle-btn"
                      :class="{ on: t.active }"
                      @click="toggleActive(t)"
                    >
                      {{ t.active ? 'Active' : 'Inactive' }}
                    </button>
                  </td>
                  <td class="muted">{{ fmtDateTime(t.updated_at) }}</td>
                  <td class="row-actions">
                    <UiButton variant="ghost" size="sm" @click="startEdit(t)">Edit</UiButton>
                    <UiButton variant="danger" size="sm" @click="deleteTemplate(t)">Delete</UiButton>
                  </td>
                </tr>
              </UiTable>
            </UiSection>
          </div>
        </template>
      </UiTabs>
    </template>
  </div>
</template>

<style scoped>
.stack {
  display: flex;
  flex-direction: column;
  gap: var(--space-5);
}
.muted {
  color: var(--color-text-muted);
  font-size: var(--font-size-sm);
}
.lang-inline {
  margin-left: var(--space-2);
}
.actions {
  display: flex;
  gap: var(--space-2);
  margin-top: var(--space-3);
}
.actions--right {
  justify-content: flex-end;
}
.row-actions {
  display: flex;
  gap: var(--space-2);
  justify-content: flex-end;
  white-space: nowrap;
}
.field {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
}
.field__label {
  font-size: var(--font-size-sm);
  font-weight: 500;
  color: var(--color-text-muted);
}
.template-textarea {
  width: 100%;
  font-family: var(--font-family-mono, 'SF Mono', 'Fira Code', monospace);
  font-size: 0.88rem;
  line-height: 1.5;
  padding: var(--space-3);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  background: var(--color-surface);
  color: var(--color-text);
  resize: vertical;
}
.template-textarea:focus {
  outline: none;
  border-color: var(--color-primary);
  box-shadow: var(--focus-ring);
}
.placeholders-bar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-1);
  margin-top: var(--space-3);
}
.ph-label {
  font-size: var(--font-size-xs);
  color: var(--color-text-muted);
  margin-right: var(--space-1);
}
.ph-btn {
  background: var(--color-sunken);
  color: var(--color-text);
  font-size: var(--font-size-xs);
  padding: 2px 8px;
  border-radius: var(--radius-sm);
  border: 1px solid var(--color-border);
  font-family: var(--font-family-mono, monospace);
  cursor: pointer;
  min-height: 0;
}
.ph-btn:hover {
  background: color-mix(in srgb, var(--color-primary) 10%, transparent);
  color: var(--color-primary);
}
.toggle-btn {
  font-size: var(--font-size-xs);
  padding: 2px 10px;
  border-radius: 999px;
  background: var(--color-sunken);
  color: var(--color-text-muted);
  border: 1px solid var(--color-border);
  cursor: pointer;
  min-height: 0;
}
.toggle-btn.on {
  background: color-mix(in srgb, var(--success-bg) 15%, transparent);
  color: var(--success-fg);
  border-color: color-mix(in srgb, var(--success-bg) 30%, transparent);
}
.msg-card {
  margin-top: var(--space-3);
}
.msg-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: var(--space-2);
  gap: var(--space-3);
  flex-wrap: wrap;
}
.msg-lang {
  display: flex;
  align-items: center;
  gap: var(--space-2);
}
.lang-name {
  font-weight: 500;
  font-size: var(--font-size-sm);
  color: var(--color-text);
}
.msg-title {
  margin: var(--space-2) 0;
  font-size: var(--font-size-md);
  color: var(--color-text);
  font-weight: 600;
}
.msg-body {
  background: var(--color-sunken);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  padding: var(--space-4);
  font-family: inherit;
  font-size: var(--font-size-sm);
  white-space: pre-wrap;
  word-break: break-word;
  line-height: 1.6;
  margin: 0;
  color: var(--color-text);
}
.new-tpl-form {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  max-width: 480px;
}

.occ-picker { position: relative; }
.occ-input-wrap { position: relative; }
.occ-search {
  width: 100%;
  min-height: 36px;
  padding: 0 32px 0 var(--space-3);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  font: var(--font-size-md) / 1.4 var(--font-family-sans);
  background: var(--color-surface);
  color: var(--color-text);
}
.occ-search:focus {
  outline: none;
  border-color: var(--color-primary);
  box-shadow: var(--focus-ring);
}
.occ-clear {
  position: absolute;
  right: 6px;
  top: 50%;
  transform: translateY(-50%);
  background: none;
  border: none;
  color: var(--color-text-muted);
  padding: 4px;
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  min-height: 0;
}
.occ-clear:hover { color: var(--color-text); }
.occ-list {
  position: absolute;
  z-index: 50;
  top: calc(100% + 4px);
  left: 0;
  right: 0;
  max-height: 240px;
  overflow-y: auto;
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  box-shadow: var(--shadow-2);
  list-style: none;
  margin: 0;
  padding: 4px 0;
}
.occ-option {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: var(--space-2) var(--space-3);
  cursor: pointer;
  gap: var(--space-3);
}
.occ-option:hover { background: var(--color-sunken); }
.occ-option.selected {
  background: color-mix(in srgb, var(--color-primary) 8%, transparent);
  font-weight: 500;
}
.occ-name {
  font-size: var(--font-size-sm);
  color: var(--color-text);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  min-width: 0;
}
.occ-dates {
  font-size: var(--font-size-xs);
  color: var(--color-text-muted);
  white-space: nowrap;
  flex-shrink: 0;
}
.occ-empty {
  padding: var(--space-3);
  font-size: var(--font-size-sm);
  color: var(--color-text-muted);
  text-align: center;
}
</style>
