<template>
  <div class="layout-diagram" :style="boxStyle">
    <div
      v-for="(cell, i) in cells"
      :key="i"
      class="layout-cell"
      :class="{
        'layout-cell--empty': empty[i],
        'layout-cell--selected': selectedIndex === i,
        'layout-cell--clickable': clickable,
      }"
      :style="cellStyle(cell)"
      @click="clickable && $emit('select', i)"
    >
      <slot name="cell" :index="i" :cell="cell">
        <span class="text-caption">{{ i + 1 }}</span>
      </slot>
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue'

const props = defineProps({
  rects: {
    type: Array,
    default: () => [],
  },
  aspect: {
    type: Number,
    default: 16 / 9,
  },
  selectedIndex: {
    type: Number,
    default: -1,
  },
  clickable: {
    type: Boolean,
    default: false,
  },
  empty: {
    type: Object,
    default: () => ({}),
  },
})

defineEmits(['select'])

const cells = computed(() => props.rects || [])
const boxStyle = computed(() => ({
  aspectRatio: String(props.aspect),
}))

function cellStyle(cell) {
  return {
    left: `${(cell.x || 0) * 100}%`,
    top: `${(cell.y || 0) * 100}%`,
    width: `${(cell.w || 0) * 100}%`,
    height: `${(cell.h || 0) * 100}%`,
  }
}
</script>

<style scoped>
.layout-diagram {
  position: relative;
  width: 100%;
  background: #111;
  overflow: hidden;
}
.layout-cell {
  position: absolute;
  box-sizing: border-box;
  border: 1px solid rgba(255, 255, 255, 0.2);
  display: flex;
  align-items: center;
  justify-content: center;
  color: rgba(255, 255, 255, 0.85);
  overflow: hidden;
  padding: 2px;
}
.layout-cell--empty {
  border-style: dashed;
  color: rgba(255, 255, 255, 0.4);
}
.layout-cell--selected {
  border-color: rgb(var(--v-theme-primary));
  border-width: 2px;
}
.layout-cell--clickable {
  cursor: pointer;
}
</style>
