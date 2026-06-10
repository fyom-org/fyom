<template>
  <div v-if="items.length > 0" class="media-row">
    <h2 class="row-title">{{ title }}</h2>
    <div class="row-scroll">
      <MediaCard v-for="item in items" :key="item.id" :item="item" />
    </div>
  </div>
</template>

<script setup lang="ts">
import MediaCard from './MediaCard.vue';
defineProps<{ title: string; items: unknown[] }>();
</script>

<style scoped>
.media-row {
  margin-bottom: 32px;
}
.row-title {
  font-size: 18px;
  color: #e0e0e0;
  margin: 0 0 12px;
  padding-left: 4px;
  font-weight: 600;
}
.row-scroll {
  display: flex;
  gap: 16px;
  overflow-x: auto;
  padding-bottom: 8px;
  scroll-snap-type: x mandatory;
  -webkit-overflow-scrolling: touch;
}
.row-scroll::-webkit-scrollbar {
  height: 4px;
}
.row-scroll::-webkit-scrollbar-track {
  background: transparent;
}
.row-scroll::-webkit-scrollbar-thumb {
  background: #2a2a3e;
  border-radius: 2px;
}
.row-scroll > * {
  scroll-snap-align: start;
  flex-shrink: 0;
  width: 160px;
}
</style>
