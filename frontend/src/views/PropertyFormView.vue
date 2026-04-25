<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '@/api/http'
import { usePropertyStore } from '@/stores/property'
import UiPageHeader from '@/components/ui/UiPageHeader.vue'
import UiCard from '@/components/ui/UiCard.vue'
import UiInput from '@/components/ui/UiInput.vue'
import UiButton from '@/components/ui/UiButton.vue'
import UiInlineBanner from '@/components/ui/UiInlineBanner.vue'

const name = ref('')
const timezone = ref('Europe/Bratislava')
const defaultLanguage = ref('sk')
const error = ref('')
const loading = ref(false)
const router = useRouter()
const props = usePropertyStore()

async function submit() {
  error.value = ''
  loading.value = true
  try {
    await api('/api/properties', {
      method: 'POST',
      json: { name: name.value, timezone: timezone.value, default_language: defaultLanguage.value },
    })
    await props.fetchList()
    await router.push('/properties')
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to create property'
  } finally {
    loading.value = false
  }
}

function cancel() {
  router.push('/properties')
}
</script>

<template>
  <div class="property-form-page">
    <UiPageHeader
      title="New property"
      lede="Create a property to start importing occupancies and finance data."
    />

    <UiInlineBanner v-if="error" tone="danger" :title="error" />

    <UiCard>
      <form class="property-form" @submit.prevent="submit">
        <UiInput
          v-model="name"
          label="Name"
          required
          autocomplete="off"
          placeholder="e.g. Apartment Bratislava Old Town"
        />
        <UiInput
          v-model="timezone"
          label="Timezone"
          help="IANA name, e.g. Europe/Bratislava"
        />
        <UiInput
          v-model="defaultLanguage"
          label="Default language"
          help="Two-letter code (sk, en, de, …)"
        />
        <div class="property-form__actions">
          <UiButton type="button" variant="secondary" :disabled="loading" @click="cancel">Cancel</UiButton>
          <UiButton type="submit" variant="primary" :loading="loading">Create property</UiButton>
        </div>
      </form>
    </UiCard>
  </div>
</template>

<style scoped>
.property-form-page {
  max-width: 640px;
}
.property-form {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}
.property-form__actions {
  display: flex;
  justify-content: flex-end;
  gap: var(--space-2);
  margin-top: var(--space-2);
}
</style>
