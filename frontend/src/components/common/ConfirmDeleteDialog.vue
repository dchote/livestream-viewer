<template>
  <StandardDialog
    :model-value="modelValue"
    :title="title"
    max-width="400"
    :fullscreen="mobile"
    @update:model-value="$emit('update:modelValue', $event)"
    @close="$emit('close')"
  >
    <v-alert v-if="error" type="error" density="compact" class="mb-4">{{ error }}</v-alert>
    <!-- Gap goes on the question when the caller adds detail below it. -->
    <p :class="$slots.default ? 'mb-2' : 'mb-0'">
      Are you sure you want to delete {{ name }}?
    </p>
    <slot />
    <template #actions>
      <v-spacer />
      <v-btn variant="text" class="mr-2" @click="$emit('update:modelValue', false)">Cancel</v-btn>
      <v-btn color="error" variant="elevated" :loading="loading" @click="$emit('confirm')">
        Delete
      </v-btn>
    </template>
  </StandardDialog>
</template>

<script setup>
import { useDisplay } from 'vuetify'
import StandardDialog from '@/components/common/StandardDialog.vue'

defineProps({
  modelValue: {
    type: Boolean,
    default: false,
  },
  /** Dialog title, e.g. "Delete screen?". */
  title: {
    type: String,
    required: true,
  },
  /** Name of the entity being deleted, interpolated into the question. */
  name: {
    type: String,
    default: '',
  },
  error: {
    type: String,
    default: '',
  },
  loading: {
    type: Boolean,
    default: false,
  },
})

defineEmits(['update:modelValue', 'close', 'confirm'])

const { mobile } = useDisplay()
</script>
