<template>
  <div class="media-card">
    <div class="poster-wrap">
      <img :src="posterSrc" :alt="item.title" @error="onPosterError" />
      <div class="poster-fallback">{{ item.title?.[0] ?? '?' }}</div>
    </div>
    <div class="info">
      <span class="title">{{ item.title }}</span>
      <span class="year" v-if="item.year">{{ item.year }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'

const props = defineProps<{ item: { id: string; title: string; year?: number } }>()

const posterSrc = ref(`/api/v1/media/${props.item.id}/poster`)

function onPosterError() {
  posterSrc.value = ''
}
</script>

<style scoped>
.media-card {
  cursor: pointer;
}

.poster-wrap {
  aspect-ratio: 2 / 3;
  border-radius: 6px;
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

.poster-wrap img[src=""] {
  display: none;
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

.year {
  font-size: 12px;
  color: #777799;
  margin-top: 2px;
  display: block;
}

.media-card:hover .poster-wrap {
  transform: scale(1.05);
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.5);
  transition: transform 0.2s, box-shadow 0.2s;
}
</style>
