<template>
  <router-link v-slot="{ navigate }" :to="`/library/${item.id}`" custom>
    <div
      class="media-card"
      role="link"
      tabindex="0"
      @click="navigate"
      @mouseenter="onHover"
      @mouseleave="onLeave"
    >
      <div class="poster-wrap">
        <img v-if="item.poster_url" :src="item.poster_url" :alt="item.title" loading="lazy" />
        <div v-else class="poster-fallback">{{ item.title?.[0] ?? '?' }}</div>
        <div v-if="hovered" class="hover-overlay">
          <button class="play-icon" @click.stop="play">▶</button>
        </div>
        <div v-if="item.position && item.duration && item.position > 0" class="progress-bar">
          <div class="progress-fill" :style="{ transform: `scaleX(${progressPercent / 100})` }"></div>
        </div>
      </div>
      <div class="info">
        <span class="title">{{ item.title }}</span>
        <div class="meta-row">
          <span v-if="item.year" class="year">{{ item.year }}</span>
          <span v-if="item.type" class="type-badge">{{ item.type }}</span>
        </div>
      </div>
    </div>
  </router-link>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue';
import { useRouter } from 'vue-router';

const props = defineProps<{
  item: {
    id: string;
    title: string;
    year?: number;
    type?: string;
    poster_url?: string;
    position?: number;
    duration?: number;
  };
}>();
const router = useRouter();
const hovered = ref(false);
const onHover = () => {
  hovered.value = true;
};
const onLeave = () => {
  hovered.value = false;
};
const play = () => {
  router.push(`/play/${props.item.id}`);
};
const progressPercent = computed(() => {
  if (!props.item.position || !props.item.duration) return 0;
  return Math.min((props.item.position / props.item.duration) * 100, 100);
});
</script>

<style scoped>
.media-card {
  cursor: pointer;
  position: relative;
  transition: transform 0.2s ease;
  transform-origin: center;
}
.poster-wrap {
  aspect-ratio: 2 / 3;
  border-radius: 8px;
  overflow: hidden;
  background: #2a2a3e;
  position: relative;
}
.poster-wrap img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  display: block;
}
.poster-fallback {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 48px;
  color: #555577;
}
.hover-overlay {
  position: absolute;
  inset: 0;
  background: rgba(0, 0, 0, 0.6);
  display: flex;
  align-items: center;
  justify-content: center;
  opacity: 0;
  transition: opacity 0.2s;
  border-radius: 8px;
}
.media-card:hover .hover-overlay {
  opacity: 1;
}
.media-card:hover {
  transform: scale(1.08);
  z-index: 10;
}
.play-icon {
  width: 48px;
  height: 48px;
  border-radius: 50%;
  background: rgba(108, 99, 255, 0.9);
  color: #fff;
  border: none;
  font-size: 20px;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
}
.progress-bar {
  position: absolute;
  bottom: 0;
  left: 0;
  right: 0;
  height: 3px;
  background: rgba(255, 255, 255, 0.2);
  border-radius: 0 0 8px 8px;
}
.progress-fill {
  height: 100%;
  background: #6c63ff;
  transition: transform 0.3s;
  transform-origin: left;
  width: 100%;
}
.info {
  margin-top: 8px;
}
.title {
  font-size: 13px;
  color: #d0d0d0;
  display: block;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.meta-row {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-top: 4px;
}
.year {
  font-size: 12px;
  color: #777799;
}
.type-badge {
  font-size: 11px;
  background: #2a2a3e;
  padding: 1px 6px;
  border-radius: 3px;
  text-transform: capitalize;
  color: #777799;
}
</style>
