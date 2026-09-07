<template>
  <v-text-field
    :model-value="modelValue"
    :label="label"
    :type="visible ? 'text' : 'password'"
    :variant="variant"
    :density="density"
    hide-details="auto"
    :autocomplete="autocomplete"
    :class="fieldClass"
    :style="fieldStyle"
    :hint="hint"
    :persistent-hint="persistentHint"
    :disabled="disabled"
    @update:model-value="$emit('update:modelValue', $event)"
  >
    <template #append-inner>
      <v-btn
        icon
        variant="text"
        size="small"
        :aria-label="visible ? 'Hide password' : 'Show password'"
        tabindex="-1"
        @click.stop="visible = !visible"
      >
        <v-icon size="small">{{ visible ? 'mdi-eye-off' : 'mdi-eye' }}</v-icon>
      </v-btn>
    </template>
  </v-text-field>
</template>

<script setup>
import { ref } from 'vue'

defineProps({
  modelValue: {
    type: String,
    default: '',
  },
  label: {
    type: String,
    default: 'Password',
  },
  /**
   * Login: current-password.
   * Create / rotate / non-session secrets: new-password (browsers ignore "off" on passwords).
   */
  autocomplete: {
    type: String,
    required: true,
  },
  variant: {
    type: String,
    default: 'outlined',
  },
  density: {
    type: String,
    default: 'compact',
  },
  fieldClass: {
    type: String,
    default: 'mb-4',
  },
  fieldStyle: {
    type: String,
    default: '',
  },
  hint: {
    type: String,
    default: undefined,
  },
  persistentHint: {
    type: Boolean,
    default: false,
  },
  disabled: {
    type: Boolean,
    default: false,
  },
})

defineEmits(['update:modelValue'])

const visible = ref(false)
</script>
