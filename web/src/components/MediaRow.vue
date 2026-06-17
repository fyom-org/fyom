<template>
  <div v-if="items.length > 0" class="media-row">
    <h2 class="row-title">{{ title }}</h2>
    <div
      class="row-scroll-wrap"
      @scroll.passive="onScroll"
    >
      <div
        v-if="showLeftFade"
        class="scroll-fade scroll-fade-left"
        aria-hidden="true"
      ></div>
      <div ref="scrollRef" class="row-scroll">
        <MediaCard
          v-for="item in items"
          :key="item.id"
          :item="item"
          :library-name="getLibraryName ? getLibraryName(item) : ''"
          @status-changed="(id, status) => $emit('status-changed', id, status)"
        />
      </div>
      <div
        v-if="showRightFade"
        class="scroll-fade scroll-fade-right"
        aria-hidden="true"
      ></div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue';
import MediaCard from './MediaCard.vue';

defineProps<{
  title: string;
  items: unknown[];
  getLibraryName?: (item: unknown) => string;
}>();

defineEmits<{
  (e: 'status-changed', id: string, status: string): void;
}>();

const scrollRef = ref<HTMLElement | null>(null);
const showLeftFade = ref(false);
const showRightFade = ref(true);

function updateFades(): void {
  const el = scrollRef.value;
  if (!el) return;

  // 2px tolerance to avoid flicker at sub-pixel scroll boundaries.
  showLeftFade.value = el.scrollLeft > 2;
  showRightFade.value = el.scrollLeft + el.clientWidth < el.scrollWidth - 2;
}

function onScroll(): void {
  updateFades();
}

onMounted(() => {
  updateFades();
  window.addEventListener('resize', updateFades, { passive: true });
});

onBeforeUnmount(() => {
  window.removeEventListener('resize', updateFades);
});
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

/* Phase 11: the scroll container is wrapped so the fade gradients can be
   positioned absolutely over the row without scrolling with the content. */
.row-scroll-wrap {
  position: relative;
}

.row-scroll {
  display: flex;
  gap: 16px;
  overflow-x: auto;
  padding-bottom: 8px;
  scroll-snap-type: x mandatory;
  -webkit-overflow-scrolling: touch;
  scrollbar-width: thin;
  scrollbar-color: #2a2a3e transparent;
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

/* Phase 11: edge fade indicators. These signal to the user that more
   content is available beyond the visible viewport. They are purely
   decorative (aria-hidden) and pointer-events:none so they never
   block clicks on cards near the edge. */
.scroll-fade {
  position: absolute;
  top: 0;
  bottom: 8px;
  width: 32px;
  pointer-events: none;
  z-index: 1;
  transition: opacity 0.2s ease;
}

.scroll-fade-left {
  left: 0;
  background: linear-gradient(to right, rgba(20, 20, 32, 0.95), rgba(20, 20, 32, 0));
}

.scroll-fade-right {
  right: 0;
  background: linear-gradient(to left, rgba(20, 20, 32, 0.95), rgba(20, 20, 32, 0));
}

@media (prefers-reduced-motion: reduce) {
  .scroll-fade {
    transition: none;
  }
}
</style>