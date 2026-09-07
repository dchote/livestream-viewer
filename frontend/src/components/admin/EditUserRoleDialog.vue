<template>
  <StandardDialog
    :model-value="modelValue"
    title="Edit user role"
    max-width="400"
    :fullscreen="mobile"
    @update:model-value="$emit('update:modelValue', $event)"
    @close="onClose"
  >
    <v-alert v-if="error" type="error" density="compact" class="mb-4">
      {{ error }}
    </v-alert>
    <v-select
      v-model="role"
      :items="ROLE_OPTIONS"
      label="Role"
      variant="outlined"
      density="compact"
      hide-details="auto"
      autocomplete="off"
      class="mb-4"
      style="max-width: 320px;"
    />
    <template #actions>
      <v-spacer />
      <v-btn variant="text" class="mr-2" @click="$emit('update:modelValue', false)">Cancel</v-btn>
      <v-btn color="primary" variant="elevated" :loading="loading" @click="submit">
        Save
      </v-btn>
    </template>
  </StandardDialog>
</template>

<script setup>
import { ref, watch } from 'vue'
import { useDisplay } from 'vuetify'
import StandardDialog from '@/components/common/StandardDialog.vue'
import { ROLE_OPTIONS, ROLE_USER } from '@/utils/roles'

const props = defineProps({
  modelValue: {
    type: Boolean,
    default: false,
  },
  user: {
    type: Object,
    default: null,
  },
  loading: {
    type: Boolean,
    default: false,
  },
})

const emit = defineEmits(['update:modelValue', 'close', 'save'])

const { mobile } = useDisplay()
const role = ref(ROLE_USER)
const error = ref('')

watch(
  () => props.modelValue,
  (open) => {
    if (open) {
      role.value = props.user?.role || ROLE_USER
      error.value = ''
    }
  },
)

function onClose() {
  error.value = ''
  emit('close')
}

function submit() {
  error.value = ''
  if (!props.user?.id) {
    error.value = 'No user selected'
    return
  }
  emit('save', {
    id: props.user.id,
    role: role.value,
  })
}

defineExpose({
  setError(message) {
    error.value = message || ''
  },
})
</script>
