<template>
  <div class="source-list">
    <v-data-table
      :headers="headers"
      :items="sources"
      density="comfortable"
      :items-per-page="50"
    >
      <template #item.kind="{ item }">
        {{ kindLabel(item.kind) }}
      </template>
      <template #item.probe="{ item }">
        {{ formatSourceIssue(item) }}
      </template>
      <template #item.status="{ item }">
        <SourceProbeChip :source="item" />
      </template>
      <template #item.actions="{ item }">
        <div
          v-if="isAdmin"
          class="d-flex justify-end align-center flex-nowrap"
          style="gap: 4px"
        >
          <v-btn
            variant="text"
            size="small"
            class="flex-shrink-0"
            :loading="probingId === item.id"
            @click="$emit('probe', item)"
          >
            Probe
          </v-btn>
          <v-btn
            icon
            variant="text"
            size="small"
            aria-label="Edit source"
            @click="$emit('edit', item)"
          >
            <v-icon size="small">mdi-pencil</v-icon>
          </v-btn>
          <v-btn
            icon
            variant="text"
            size="small"
            color="error"
            aria-label="Delete source"
            @click="$emit('remove', item)"
          >
            <v-icon size="small">mdi-delete</v-icon>
          </v-btn>
        </div>
      </template>
      <template #bottom />
    </v-data-table>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { useDisplay } from 'vuetify'
import SourceProbeChip from '@/components/sources/SourceProbeChip.vue'
import { formatSourceIssue, kindLabel } from '@/utils/formatters'

defineProps({
  sources: {
    type: Array,
    default: () => [],
  },
  isAdmin: {
    type: Boolean,
    default: false,
  },
  probingId: {
    type: Number,
    default: null,
  },
})

defineEmits(['probe', 'edit', 'remove'])

const { mobile } = useDisplay()

const headers = computed(() => {
  const cols = [
    { title: 'Name', key: 'name' },
    { title: 'Kind', key: 'kind' },
  ]
  if (!mobile.value) {
    cols.push({ title: 'Probe', key: 'probe' })
  }
  cols.push(
    { title: 'Status', key: 'status', sortable: false },
    {
      title: '',
      key: 'actions',
      sortable: false,
      align: 'end',
      width: mobile.value ? 148 : 168,
    },
  )
  return cols
})
</script>

<style scoped>
.source-list {
  width: 100%;
  overflow-x: auto;
  -webkit-overflow-scrolling: touch;
}
</style>
